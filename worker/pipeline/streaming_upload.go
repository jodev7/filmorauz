package pipeline

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/filmorauz/worker/storage"
)

const (
	SegmentUploadConcurrency = 10
	SegmentRetryMax          = 3
	SegmentRetryDelayMs      = 500
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
	errorsMu    sync.Mutex
	uploadCount int32
	failCount   int32

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
	remotePath := fmt.Sprintf("videos/%s/%s/%s", u.folderName, u.rendition, filepath.Base(path))

	if _, ok := u.uploaded.Load(remotePath); ok {
		log.Printf("[STREAM_UPLOAD] Skipping already uploaded: %s", remotePath)
		return
	}

	log.Printf("[STREAM_UPLOAD] Segment detected: %s", path)

	if !u.waitForStableFile(path) {
		u.recordError(fmt.Errorf("file not stable: %s", path))
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		u.recordError(fmt.Errorf("read file: %w", err))
		return
	}

	for attempt := 0; attempt < u.retries; attempt++ {
		_, err = u.storage.UploadData(remotePath, data, "video/mp2t")
		if err == nil {
			break
		}
		log.Printf("[STREAM_UPLOAD] Retry %d/%d for %s: %v", attempt+1, u.retries, remotePath, err)
		time.Sleep(time.Duration(SegmentRetryDelayMs) * time.Millisecond)
	}

	if err != nil {
		log.Printf("[STREAM_UPLOAD] FAILED: %s - %v", remotePath, err)
		atomic.AddInt32(&u.failCount, 1)
		u.recordError(err)
		return
	}

	u.uploaded.Store(remotePath, true)
	atomic.AddInt32(&u.uploadCount, 1)
	log.Printf("[STREAM_UPLOAD] Segment uploaded: %s", remotePath)
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
	u.errorsMu.Unlock()
}

func (u *segmentUploader) stop() error {
	close(u.pending)
	u.wg.Wait()

	uploadCount := atomic.LoadInt32(&u.uploadCount)
	failCount := atomic.LoadInt32(&u.failCount)
	log.Printf("[STREAM_UPLOAD] %s done: %d uploaded, %d failed", u.rendition, uploadCount, failCount)

	if failCount > 0 {
		u.errorsMu.Lock()
		defer u.errorsMu.Unlock()
		return fmt.Errorf("%d segment uploads failed", failCount)
	}
	return nil
}

func (u *segmentUploader) addFile(path string) {
	select {
	case u.pending <- path:
	default:
	}
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

	go func() {
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
	s.watcher.stop()

	var allErrors []error
	for name, u := range s.uploaders {
		if err := u.stop(); err != nil {
			allErrors = append(allErrors, fmt.Errorf("%s: %w", name, err))
		}
	}

	if len(allErrors) > 0 {
		return fmt.Errorf("streaming upload errors: %v", allErrors)
	}
	log.Printf("[STREAM_UPLOAD] All uploads complete")
	return nil
}
