package pipeline

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/filmorauz/worker/models"
)

// CleanupAllowlist holds the absolute, cleaned root directories that
// safeRemove is permitted to delete inside. Any path outside every entry is
// refused — this is the guardrail against the "rm -rf /" foot-gun if a
// jobID/local_path ever leaks an empty/junk value into a delete call.
type CleanupAllowlist struct {
	ParserDownloads string // /opt/filmorauz/parser/downloads
	ReadyVideo      string // /opt/filmorauz/worker/readyvideo
	WorkerUploads   string // /opt/filmorauz/worker/uploads (only safe to clean in prod)
}

func cleanAbs(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(abs)
}

// withinAny returns true when target sits under one of the allowed roots.
// Root and target are compared after Abs+Clean so "../" tricks can't escape.
func withinAny(target string, roots ...string) (string, bool) {
	t := cleanAbs(target)
	if t == "" || t == "/" || t == "." {
		return "", false
	}
	for _, r := range roots {
		if r == "" {
			continue
		}
		root := cleanAbs(r)
		if root == "" || root == "/" {
			continue
		}
		// Require target == root, or target starts with root + separator.
		if t == root {
			return root, true
		}
		if strings.HasPrefix(t, root+string(os.PathSeparator)) {
			return root, true
		}
	}
	return "", false
}

// safeRemove deletes a file or directory, but only if it sits under one of
// the explicitly allowed roots. All decisions are logged so an operator can
// follow the cleanup trail in `journalctl -u worker`.
func safeRemove(jobID, what, target string, allowedRoots ...string) {
	if target == "" {
		log.Printf("[CLEANUP] SKIP job=%s reason=empty_path what=%s", jobID, what)
		return
	}
	root, ok := withinAny(target, allowedRoots...)
	if !ok {
		log.Printf("[CLEANUP] SKIP job=%s reason=outside_allowlist what=%s path=%s allowed=%v",
			jobID, what, target, allowedRoots)
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[CLEANUP] SKIP job=%s reason=already_gone what=%s path=%s", jobID, what, target)
		} else {
			log.Printf("[CLEANUP] SKIP job=%s reason=stat_failed what=%s path=%s err=%v", jobID, what, target, err)
		}
		return
	}
	// Refuse to nuke a root itself ("readyvideo/" with no subfolder) — only
	// individual job folders/files inside it.
	if cleanAbs(target) == root {
		log.Printf("[CLEANUP] SKIP job=%s reason=is_allowlist_root what=%s path=%s", jobID, what, target)
		return
	}
	var rmErr error
	if info.IsDir() {
		rmErr = os.RemoveAll(target)
	} else {
		rmErr = os.Remove(target)
	}
	if rmErr != nil {
		log.Printf("[CLEANUP] FAILED job=%s what=%s path=%s err=%v", jobID, what, target, rmErr)
		return
	}
	log.Printf("[CLEANUP] DELETED job=%s what=%s path=%s", jobID, what, target)

	// Best-effort: prune the empty parent dir (e.g. parser/downloads/<job>/).
	parent := filepath.Dir(target)
	if _, ok := withinAny(parent, allowedRoots...); !ok {
		return
	}
	if cleanAbs(parent) == cleanAbs(root) {
		return // never delete a root itself
	}
	if entries, rerr := os.ReadDir(parent); rerr == nil && len(entries) == 0 {
		if rmErr := os.Remove(parent); rmErr == nil {
			log.Printf("[CLEANUP] DELETED job=%s what=%s_parent path=%s", jobID, what, parent)
		}
	}
}

// CleanupAfterSuccess is the single entry-point for post-success cleanup.
// It is mode-aware: in production we delete both the parser source MP4 and
// the readyvideo working dir; in development we keep the parser MP4 because
// the local streaming layer may still reference it, and we leave readyvideo
// alone for the same reason. Callers must only invoke this after:
//   - HLS uploaded (B2 in prod / worker/uploads in dev)
//   - poster/backdrop uploaded if applicable
//   - Mongo write succeeded
//   - job.Status set to completed
func (p *Pipeline) CleanupAfterSuccess(job *models.IngestionJob, parserLocalPath, readyVideoDir string) {
	if job == nil {
		return
	}
	jobID := job.ID.Hex()
	if job.Status != models.IngestionStatusCompleted {
		log.Printf("[CLEANUP] SKIP job=%s reason=not_completed status=%s", jobID, job.Status)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(p.config.StorageConfig.Mode))
	allow := p.cleanupAllowlist()

	if mode == "prod" || mode == "production" {
		// Parser-downloaded source MP4 (or whatever extension the parser left).
		safeRemove(jobID, "parser_source", parserLocalPath, allow.ParserDownloads)
		// Worker readyvideo working folder (HLS staging area).
		safeRemove(jobID, "ready_video", readyVideoDir, allow.ReadyVideo)
	} else {
		log.Printf("[CLEANUP] SKIP job=%s reason=dev_mode_keep_local_files mode=%s parser=%s ready=%s",
			jobID, mode, parserLocalPath, readyVideoDir)
	}
}

// cleanupAllowlist resolves the three permitted roots from the storage
// config + DOWNLOAD_DIR env, and falls back to relative defaults when those
// are blank (matches main.go's behavior).
func (p *Pipeline) cleanupAllowlist() CleanupAllowlist {
	downloads := strings.TrimSpace(os.Getenv("DOWNLOAD_DIR"))
	if downloads == "" {
		downloads = "../parser/downloads"
	}
	cwd, _ := os.Getwd()
	readyVideo := filepath.Join(cwd, "readyvideo")
	uploads := p.config.StorageConfig.LocalPath
	if uploads == "" {
		uploads = filepath.Join(cwd, "uploads")
	}
	return CleanupAllowlist{
		ParserDownloads: cleanAbs(downloads),
		ReadyVideo:      cleanAbs(readyVideo),
		WorkerUploads:   cleanAbs(uploads),
	}
}

// CleanupCompletedJobArtifacts is the manual-action entry point: scan a
// completed job and remove its on-disk artifacts using the same prefix
// guards. Called from the worker HTTP cleanup endpoint and the periodic
// janitor. Returns a list of human-readable actions taken / skipped.
func (p *Pipeline) CleanupCompletedJobArtifacts(job *models.IngestionJob) ([]string, error) {
	if job == nil {
		return nil, fmt.Errorf("nil job")
	}
	jobID := job.ID.Hex()
	if job.Status != models.IngestionStatusCompleted {
		return nil, fmt.Errorf("job %s status=%s, refusing to clean (only completed jobs are cleanable)", jobID, job.Status)
	}
	mode := strings.ToLower(strings.TrimSpace(p.config.StorageConfig.Mode))
	if !(mode == "prod" || mode == "production") {
		return []string{fmt.Sprintf("skipped: dev mode keeps local files (mode=%s)", mode)}, nil
	}

	allow := p.cleanupAllowlist()
	var actions []string

	// Parser download lives at job.LocalPath when the worker drove the
	// download, or at job.Metadata.VideoURL local fallback. Try both.
	candidates := []string{job.LocalPath}
	if job.Metadata != nil {
		candidates = append(candidates, job.Metadata.VideoURL)
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, ok := withinAny(c, allow.ParserDownloads); !ok {
			continue
		}
		safeRemove(jobID, "parser_source", c, allow.ParserDownloads)
		actions = append(actions, "parser_source: "+c)
	}

	// Readyvideo working folder: the canonical layout is
	// readyvideo/<folder>/, where <folder> is recorded in job.OutputPath
	// during processing. Treat OutputPath as authoritative; fall back to
	// scanning by job ID prefix.
	if job.OutputPath != "" {
		if _, ok := withinAny(job.OutputPath, allow.ReadyVideo); ok {
			safeRemove(jobID, "ready_video", job.OutputPath, allow.ReadyVideo)
			actions = append(actions, "ready_video: "+job.OutputPath)
		}
	}

	if len(actions) == 0 {
		actions = append(actions, "nothing to clean (paths empty or already gone)")
	}
	return actions, nil
}
