package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
)

// PendingCountGetter is an interface for getting pending job count
type PendingCountGetter interface {
	CountPendingJobs(ctx context.Context) (int64, error)
}

// ScalingConfig defines the scaling thresholds and limits
type ScalingConfig struct {
	ThresholdLow    int
	ThresholdMedium int
	ThresholdHigh   int
	MinWorkers      int
	MaxWorkers      int
	PollingInterval time.Duration
}

// DefaultScalingConfig returns the default scaling configuration
func DefaultScalingConfig() ScalingConfig {
	return ScalingConfig{
		ThresholdLow:    10,
		ThresholdMedium: 50,
		ThresholdHigh:   100,
		MinWorkers:      1,
		MaxWorkers:      10,
		PollingInterval: 10 * time.Second,
	}
}

// WorkerProcess represents a running worker
type WorkerProcess struct {
	ID       string
	StopChan chan struct{}
	DoneChan chan struct{}
}

// WorkerPoolManager manages a pool of workers with auto-scaling
type WorkerPoolManager struct {
	config        ScalingConfig
	workers       map[string]*WorkerProcess
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	pendingGetter PendingCountGetter
	started       bool
}

// NewWorkerPoolManager creates a new worker pool manager
func NewWorkerPoolManager(getter PendingCountGetter, config ScalingConfig) *WorkerPoolManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPoolManager{
		config:        config,
		workers:       make(map[string]*WorkerProcess),
		ctx:           ctx,
		cancel:        cancel,
		pendingGetter: getter,
	}
}

// Start begins the auto-scaling worker pool
func (m *WorkerPoolManager) Start() {
	if m.started {
		log.Println("[SCALER] Worker pool already started")
		return
	}
	m.started = true

	log.Printf("[SCALER] Starting worker pool manager")
	log.Printf("[SCALER] Scaling config: low=%d, medium=%d, high=%d, min=%d, max=%d",
		m.config.ThresholdLow, m.config.ThresholdMedium, m.config.ThresholdHigh,
		m.config.MinWorkers, m.config.MaxWorkers)

	// Start initial workers (minimum)
	m.scaleToWorkers(m.config.MinWorkers)

	// Start scaling monitor loop
	m.wg.Add(1)
	go m.scalingLoop()
}

// Stop gracefully stops all workers
func (m *WorkerPoolManager) Stop() {
	log.Printf("[SCALER] Stopping worker pool manager...")
	m.cancel()

	// Wait for scaling loop to finish
	m.wg.Wait()

	// Stop all workers
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, w := range m.workers {
		log.Printf("[SCALER] Stopping worker %s", id)
		close(w.StopChan)
		<-w.DoneChan // Wait for worker to finish
	}
	m.workers = nil
	log.Printf("[SCALER] All workers stopped")
}

// scalingLoop periodically checks queue size and scales workers
func (m *WorkerPoolManager) scalingLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.PollingInterval)
	defer ticker.Stop()

	log.Printf("[SCALER] Scaling loop started (interval: %v)", m.config.PollingInterval)

	for {
		select {
		case <-m.ctx.Done():
			log.Printf("[SCALER] Scaling loop stopped")
			return
		case <-ticker.C:
			m.checkAndScale()
		}
	}
}

// checkAndScale evaluates queue size and adjusts worker count
func (m *WorkerPoolManager) checkAndScale() {
	pendingCount, err := m.getPendingCount()
	if err != nil {
		log.Printf("[SCALER] Error getting pending count: %v", err)
		return
	}

	currentWorkers := m.getWorkerCount()
	targetWorkers := m.calculateTargetWorkers(pendingCount)

	log.Printf("[SCALER] Queue: %d, Workers: %d -> %d",
		pendingCount, currentWorkers, targetWorkers)

	if targetWorkers != currentWorkers {
		log.Printf("[SCALER] ACTION: Scaling %d -> %d workers (queue: %d)",
			currentWorkers, targetWorkers, pendingCount)
		m.scaleToWorkers(targetWorkers)
	}
}

// getPendingCount retrieves the current pending job count
func (m *WorkerPoolManager) getPendingCount() (int64, error) {
	if m.pendingGetter != nil {
		return m.pendingGetter.CountPendingJobs(m.ctx)
	}
	return 0, fmt.Errorf("pending getter not configured")
}

// calculateTargetWorkers determines the ideal worker count based on queue
func (m *WorkerPoolManager) calculateTargetWorkers(queueSize int64) int {
	switch {
	case queueSize > int64(m.config.ThresholdHigh):
		return m.config.MaxWorkers
	case queueSize > int64(m.config.ThresholdMedium):
		return 5
	case queueSize > int64(m.config.ThresholdLow):
		return 2
	default:
		return m.config.MinWorkers
	}
}

// scaleToWorkers adjusts the worker pool to the target count
func (m *WorkerPoolManager) scaleToWorkers(targetCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clamp to min/max bounds
	if targetCount < m.config.MinWorkers {
		targetCount = m.config.MinWorkers
	}
	if targetCount > m.config.MaxWorkers {
		targetCount = m.config.MaxWorkers
	}

	currentCount := len(m.workers)

	if targetCount > currentCount {
		// Scale UP: spawn additional workers
		spawnCount := targetCount - currentCount
		log.Printf("[SCALER] Scaling UP: +%d workers", spawnCount)
		for i := 0; i < spawnCount; i++ {
			m.spawnWorkerLocked()
		}
	} else if targetCount < currentCount {
		// Scale DOWN: remove excess workers
		stopCount := currentCount - targetCount
		log.Printf("[SCALER] Scaling DOWN: -%d workers", stopCount)
		m.stopWorkersLocked(stopCount)
	}

	log.Printf("[SCALER] Worker pool size: %d (target: %d)", len(m.workers), targetCount)
}

// spawnWorkerLocked creates a new worker (must hold mutex)
func (m *WorkerPoolManager) spawnWorkerLocked() {
	workerID := fmt.Sprintf("worker-%d", len(m.workers)+1)

	stopChan := make(chan struct{})
	doneChan := make(chan struct{})

	m.workers[workerID] = &WorkerProcess{
		ID:       workerID,
		StopChan: stopChan,
		DoneChan: doneChan,
	}

	// Start the worker goroutine
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer close(doneChan)

		log.Printf("[SCALER] Worker %s started", workerID)

		// Worker main loop - processes jobs
		// In production, this would call jobRepo.ClaimNextJob()
		for {
			select {
			case <-m.ctx.Done():
				log.Printf("[SCALER] Worker %s stopping (context cancelled)", workerID)
				return
			case <-stopChan:
				log.Printf("[SCALER] Worker %s stopped (requested)", workerID)
				return
			default:
				// Poll for work - time.Sleep would be used in production
			}
		}
	}()

	log.Printf("[SCALER] Spawned worker: %s (total: %d)", workerID, len(m.workers))
}

// stopWorkersLocked stops the specified number of workers (must hold mutex)
func (m *WorkerPoolManager) stopWorkersLocked(count int) {
	stopped := 0
	for id, w := range m.workers {
		if stopped >= count {
			break
		}
		select {
		case w.StopChan <- struct{}{}:
			log.Printf("[SCALER] Sent stop signal to worker: %s", id)
			delete(m.workers, id)
			stopped++
		default:
			// Worker already stopping
		}
	}
}

// getWorkerCount returns the current number of active workers
func (m *WorkerPoolManager) getWorkerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.workers)
}

// GetStatus returns the current status of the worker pool
func (m *WorkerPoolManager) GetStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pending, _ := m.getPendingCount()

	return map[string]interface{}{
		"worker_count":   len(m.workers),
		"pending_jobs":   pending,
		"min_workers":    m.config.MinWorkers,
		"max_workers":    m.config.MaxWorkers,
		"threshold_low":  m.config.ThresholdLow,
		"threshold_high": m.config.ThresholdHigh,
	}
}

// GetScalingConfig loads scaling configuration from environment
func GetScalingConfig() ScalingConfig {
	config := DefaultScalingConfig()

	if v := os.Getenv("WORKER_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			config.MinWorkers = n
		}
	}
	if v := os.Getenv("WORKER_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			config.MaxWorkers = n
		}
	}
	if v := os.Getenv("SCALE_THRESHOLD_LOW"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			config.ThresholdLow = n
		}
	}
	if v := os.Getenv("SCALE_THRESHOLD_HIGH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			config.ThresholdHigh = n
		}
	}

	return config
}
