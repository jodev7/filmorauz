package pipeline

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/filmorauz/worker/storage"
)

const (
	SegmentUploadConcurrency = 10    // Concurrent segment upload workers per rendition
	SegmentRetryMax          = 5     // Max retry attempts per segment
	SegmentRetryBaseDelayMs  = 500   // Base delay for exponential backoff (500ms)
	SegmentRetryMaxDelayMs   = 30000 // Max backoff cap (30s)
	FileStabilityCheckMs     = 500
	FileStabilityAttempts    = 3
)

type segmentUploader struct {
	storage    storage.Storage
	folderName string
	rendition  string

	uploaded    sync.Map
	pending     chan string
	errors      []error
	firstErr    error // first real upload failure for this rendition
	errorsMu    sync.Mutex
	uploadCount int32
	failCount   int32
	// totalRetryAttempts tracks how many times we retried a failed upload (excluding first attempt)
	totalRetryAttempts int32

	workers int
	retries int
	wg      sync.WaitGroup
}

func newSegmentUploader(storage storage.Storage, folderName, rendition string, workers int, retries int) *segmentUploader {
	if retries <= 0 {
		retries = SegmentRetryMax
	}
	return &segmentUploader{
		storage:    storage,
		folderName: folderName,
		rendition:  rendition,
		pending:    make(chan string, 100),
		workers:    workers,
		retries:    retries,
	}
}

func (u *segmentUploader) start() {
	log.Printf("[STREAM_UPLOAD] Starting uploader for %s (workers: %d)", u.rendition, u.workers)

	for i := 0; i < u.workers; i++ {
		u.wg.Add(1)
		go u.worker(i)
	}
}

func (u *segmentUploader) worker(id int) {
	defer u.wg.Done()

	for path := range u.pending {
		u.uploadSegment(path)
	}
}

func (u *segmentUploader) uploadSegment(path string) {
	remotePath := fmt.Sprintf("videos/movies/%s/%s/%s", u.folderName, u.rendition, filepath.Base(path))

	if _, ok := u.uploaded.Load(remotePath); ok {
		log.Printf("[STREAM_UPLOAD] Skipping already uploaded: %s", remotePath)
		return
	}

	log.Printf("[STREAM_UPLOAD] Segment detected: rendition=%s, local=%s, remote=%s", u.rendition, path, remotePath)

	if !u.waitForStableFile(path) {
		err := fmt.Errorf("file not stable: %s", path)
		log.Printf("[STREAM_UPLOAD] FAILED: rendition=%s, key=%s, reason=%v", u.rendition, remotePath, err)
		atomic.AddInt32(&u.failCount, 1)
		u.recordError(err)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		wrapped := fmt.Errorf("read file %s: %w", path, err)
		log.Printf("[STREAM_UPLOAD] FAILED: rendition=%s, key=%s, reason=%v", u.rendition, remotePath, wrapped)
		atomic.AddInt32(&u.failCount, 1)
		u.recordError(wrapped)
		return
	}

	// Retry loop with exponential backoff
	for attempt := 0; attempt < u.retries; attempt++ {
		_, err = u.storage.UploadData(remotePath, data, "video/mp2t")
		if err == nil {
			break
		}
		// This attempt failed
		log.Printf("[STREAM_UPLOAD] Upload attempt failed: rendition=%s, key=%s, attempt=%d/%d, bytes=%d, error=%v",
			u.rendition, remotePath, attempt+1, u.retries, len(data), err)
		if attempt < u.retries-1 {
			// Will retry: count this retry and wait with exponential backoff
			atomic.AddInt32(&u.totalRetryAttempts, 1)
			// Exponential backoff: base 500ms, doubling each time
			delayMs := SegmentRetryBaseDelayMs * (1 << attempt) // 500, 1000, 2000, 4000, 8000
			if delayMs > SegmentRetryMaxDelayMs {
				delayMs = SegmentRetryMaxDelayMs
			}
			// Add jitter +/- 10% to avoid thundering herd
			jitter := (rand.Intn(delayMs/10) - delayMs/20) // +/- 5%
			delayMs += jitter
			if delayMs < 50 {
				delayMs = 50 // minimum 50ms
			}
			log.Printf("[STREAM_UPLOAD] Retry %d/%d for %s after %dms: %v", attempt+1, u.retries, remotePath, delayMs, err)
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}
	}

	if err != nil {
		log.Printf("[STREAM_UPLOAD] FAILED_FINAL: rendition=%s, key=%s, after=%d attempts, error=%v",
			u.rendition, remotePath, u.retries, err)
		atomic.AddInt32(&u.failCount, 1)
		u.recordError(fmt.Errorf("%s/%s: %w", u.rendition, filepath.Base(path), err))
		return
	}

	u.uploaded.Store(remotePath, true)
	atomic.AddInt32(&u.uploadCount, 1)
	log.Printf("[STREAM_UPLOAD] Segment uploaded: %s", remotePath)
	log.Printf("[HLS_KEY] segment_local=%s segment_b2_key=%s", path, remotePath)
}

func (u *segmentUploader) waitForStableFile(path string) bool {
	var lastSize int64

	for i := 0; i < FileStabilityAttempts; i++ {
		info, err := os.Stat(path)
		if err != nil {
			return false
		}

		currentSize := info.Size()
		if currentSize > 0 && currentSize == lastSize {
			return true
		}

		lastSize = currentSize
		time.Sleep(time.Duration(FileStabilityCheckMs) * time.Millisecond)
	}

	info, err := os.Stat(path)
	return err == nil && info.Size() > 0 && info.Size() == lastSize
}

func (u *segmentUploader) recordError(err error) {
	u.errorsMu.Lock()
	u.errors = append(u.errors, err)
	if u.firstErr == nil {
		u.firstErr = err
	}
	u.errorsMu.Unlock()
}

func (u *segmentUploader) stop() error {
	close(u.pending)
	u.wg.Wait()

	uploadCount := atomic.LoadInt32(&u.uploadCount)
	failCount := atomic.LoadInt32(&u.failCount)
	totalRetries := atomic.LoadInt32(&u.totalRetryAttempts)
	totalProcessed := uploadCount + failCount

	log.Printf("[STREAM_UPLOAD] %s done: processed=%d, uploaded=%d, failed=%d, retries=%d",
		u.rendition, totalProcessed, uploadCount, failCount, totalRetries)

	if failCount > 0 {
		u.errorsMu.Lock()
		firstErr := u.firstErr
		u.errorsMu.Unlock()
		// Log detailed error summary
		log.Printf("[STREAM_UPLOAD] %s: %d failures out of %d segments (%.1f%% failure rate), first_error=%v",
			u.rendition, failCount, totalProcessed, float64(failCount)*100.0/float64(totalProcessed), firstErr)
		if firstErr != nil {
			return fmt.Errorf("%s: %d/%d segment uploads failed — first error: %w",
				u.rendition, failCount, totalProcessed, firstErr)
		}
		return fmt.Errorf("%s: %d/%d segment uploads failed", u.rendition, failCount, totalProcessed)
	}
	return nil
}

func (u *segmentUploader) addFile(path string) {
	// Blocking send ensures no segment loss even when channel is full.
	// This provides natural backpressure: if upload pipeline is saturated,
	// the file watcher will pause until workers consume pending segments.
	u.pending <- path
}

func (u *segmentUploader) getErrors() []error {
	u.errorsMu.Lock()
	defer u.errorsMu.Unlock()
	errors := make([]error, len(u.errors))
	copy(errors, u.errors)
	return errors
}

type fileWatcher struct {
	watchDirs []string
	uploaders map[string]*segmentUploader
	interval  time.Duration

	stopChan chan struct{}
	polling  bool
	polled   map[string]int64
	mu       sync.Mutex
	wg       sync.WaitGroup // ensures goroutine has exited after stop
}

func newFileWatcher(dirs []string, uploaders map[string]*segmentUploader) *fileWatcher {
	return &fileWatcher{
		watchDirs: dirs,
		uploaders: uploaders,
		interval:  500 * time.Millisecond,
		stopChan:  make(chan struct{}),
		polled:    make(map[string]int64),
	}
}

func (f *fileWatcher) start() {
	log.Printf("[FILE_WATCHER] Starting to watch %d directories", len(f.watchDirs))
	f.polling = true
	f.wg.Add(1)

	go func() {
		defer f.wg.Done()
		for f.polling {
			select {
			case <-f.stopChan:
				f.polling = false
				return
			default:
				f.scanAndNotify()
			}
			time.Sleep(f.interval)
		}
	}()
}

func (f *fileWatcher) scanAndNotify() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, dir := range f.watchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		renditionDir := filepath.Base(dir)

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()
			if !hasTsExtension(name) {
				continue
			}

			fullPath := filepath.Join(dir, name)
			stat, err := entry.Info()
			if err != nil {
				continue
			}

			key := fullPath
			if mtime, exists := f.polled[key]; !exists || mtime != stat.ModTime().Unix() {
				f.polled[key] = stat.ModTime().Unix()

				if uploader, ok := f.uploaders[renditionDir]; ok {
					uploader.addFile(fullPath)
				}
			}
		}
	}
}

func (f *fileWatcher) stop() {
	close(f.stopChan)
	f.wg.Wait() // ensure goroutine exits before returning
	log.Printf("[FILE_WATCHER] Stopped")
}

func hasTsExtension(name string) bool {
	return len(name) > 3 && name[len(name)-3:] == ".ts"
}

type streamingUploader struct {
	storage    storage.Storage
	folderName string
	renditions []RenditionConfig
	workers    int
	outputDir  string

	uploaders map[string]*segmentUploader
	watcher   *fileWatcher
}

func newStreamingUploader(storage storage.Storage, folderName string, renditions []RenditionConfig, workers int, retries int, outputDir string) *streamingUploader {
	uploaders := make(map[string]*segmentUploader)
	for _, r := range renditions {
		uploaders[r.Name] = newSegmentUploader(storage, folderName, r.Name, workers, retries)
	}

	watchDirs := make([]string, 0, len(renditions))
	for _, r := range renditions {
		watchDirs = append(watchDirs, filepath.Join(outputDir, r.Name))
	}

	return &streamingUploader{
		storage:    storage,
		folderName: folderName,
		renditions: renditions,
		workers:    workers,
		outputDir:  outputDir,
		uploaders:  uploaders,
		watcher:    newFileWatcher(watchDirs, uploaders),
	}
}

func (s *streamingUploader) start() {
	for _, u := range s.uploaders {
		u.start()
	}
	s.watcher.start()
	log.Printf("[STREAM_UPLOAD] Started streaming upload for %d renditions", len(s.renditions))
}

func (s *streamingUploader) stopAndWait() error {
	// Grace period: allow file watcher to pick up any final segments created
	// by the just-finished ffmpeg processes. Watcher polls every 500ms;
	// waiting 2 seconds ensures at least one full scan after last segment written.
	time.Sleep(2 * time.Second)

	// Now stop the watcher and wait for its goroutine to exit
	s.watcher.stop()

	var allErrors []error
	var firstErr error
	totalFailed := int32(0)
	for _, r := range s.renditions {
		u := s.uploaders[r.Name]
		if u == nil {
			continue
		}
		err := u.stop()
		totalFailed += atomic.LoadInt32(&u.failCount)
		if err != nil {
			allErrors = append(allErrors, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if len(allErrors) > 0 {
		log.Printf("[STREAM_UPLOAD] Aggregate failure: renditions_failed=%d, total_segments_failed=%d, first_error=%v",
			len(allErrors), totalFailed, firstErr)
		if firstErr != nil {
			return fmt.Errorf("segment upload failed across %d renditions (%d segments): %w",
				len(allErrors), totalFailed, firstErr)
		}
		return fmt.Errorf("segment upload failed across %d renditions (%d segments)",
			len(allErrors), totalFailed)
	}
	log.Printf("[STREAM_UPLOAD] All uploads complete")
	return nil
}
