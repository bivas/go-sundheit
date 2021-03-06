package gosundheit

import (
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// Health is the API for registering / deregistering health checks, and for fetching the health checks results.
type Health interface {
	// RegisterCheck registers a health check according to the given configuration.
	// Once RegisterCheck() is called, the check is scheduled to run in it's own goroutine.
	// Callers must make sure the checks complete at a reasonable time frame, or the next execution will delay.
	RegisterCheck(cfg *Config) error
	// Deregister removes a health check from this instance, and stops it's next executions.
	// If the check is running while Deregister() is called, the check may complete it's current execution.
	// Once a check is removed, it's results are no longer returned.
	Deregister(name string)
	// Results returns a snapshot of the health checks execution results at the time of calling, and the current health.
	// A system is considered healthy iff all checks are passing
	Results() (results map[string]Result, healthy bool)
	// IsHealthy returns the current health of the system.
	// A system is considered healthy iff all checks are passing.
	IsHealthy() bool
	// DeregisterAll Deregister removes all health checks from this instance, and stops their next executions.
	// It is equivalent of calling Deregister() for each currently registered check.
	DeregisterAll()
}

// New returns a new Health instance.
func New(opts ...Option) Health {
	h := &health{
		results:    make(map[string]Result, maxExpectedChecks),
		checkTasks: make(map[string]checkTask, maxExpectedChecks),
		lock:       sync.RWMutex{},
	}
	for _, opt := range append(opts, WithDefaults()) {
		opt(h)
	}
	return h
}

type health struct {
	results        map[string]Result
	checkTasks     map[string]checkTask
	checksListener CheckListeners
	healthListener HealthListeners
	lock           sync.RWMutex
}

func (h *health) RegisterCheck(cfg *Config) error {
	if cfg.Check == nil || cfg.Check.Name() == "" {
		return errors.Errorf("misconfigured check %v", cfg.Check)
	}

	// checks are initially failing by default, but we allow overrides...
	var initialErr error
	if !cfg.InitiallyPassing {
		initialErr = fmt.Errorf(initialResultMsg)
	}

	result := h.updateResult(cfg.Check.Name(), initialResultMsg, 0, initialErr, time.Now())
	h.checksListener.OnCheckRegistered(cfg.Check.Name(), result)
	h.scheduleCheck(h.createCheckTask(cfg), cfg)
	return nil
}

func (h *health) createCheckTask(cfg *Config) *checkTask {
	h.lock.Lock()
	defer h.lock.Unlock()

	task := checkTask{
		stopChan: make(chan bool, 1),
		check:    cfg.Check,
	}
	h.checkTasks[cfg.Check.Name()] = task

	return &task
}

func (h *health) stopCheckTask(name string) {
	h.lock.Lock()
	defer h.lock.Unlock()

	task := h.checkTasks[name]

	task.stop()

	delete(h.results, name)
	delete(h.checkTasks, name)
}

func (h *health) scheduleCheck(task *checkTask, cfg *Config) {
	go func() {
		// initial execution
		if !h.runCheckOrStop(task, time.After(cfg.InitialDelay)) {
			return
		}
		h.reportResults()
		// scheduled recurring execution
		task.ticker = time.NewTicker(cfg.ExecutionPeriod)
		for {
			if !h.runCheckOrStop(task, task.ticker.C) {
				return
			}
			h.reportResults()
		}
	}()
}

func (h *health) reportResults() {
	h.lock.RLock()
	resultsCopy := copyResultsMap(h.results)
	h.lock.RUnlock()
	h.healthListener.OnResultsUpdated(resultsCopy)
}

func (h *health) runCheckOrStop(task *checkTask, timerChan <-chan time.Time) bool {
	select {
	case <-task.stopChan:
		h.stopCheckTask(task.check.Name())
		return false
	case t := <-timerChan:
		h.checkAndUpdateResult(task, t)
		return true
	}
}

func (h *health) checkAndUpdateResult(task *checkTask, checkTime time.Time) {
	h.checksListener.OnCheckStarted(task.check.Name())
	details, duration, err := task.execute()
	result := h.updateResult(task.check.Name(), details, duration, err, checkTime)
	h.checksListener.OnCheckCompleted(task.check.Name(), result)
}

func (h *health) Deregister(name string) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	task, ok := h.checkTasks[name]
	if ok {
		// actual cleanup happens in the task go routine
		task.stopChan <- true
	}
}

func (h *health) DeregisterAll() {
	h.lock.RLock()
	defer h.lock.RUnlock()

	for _, task := range h.checkTasks {
		task.stopChan <- true
	}
}

func (h *health) Results() (results map[string]Result, healthy bool) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	results = make(map[string]Result, len(h.results))

	healthy = true
	for k, v := range h.results {
		results[k] = v
		healthy = healthy && v.IsHealthy()
	}

	return
}

func (h *health) IsHealthy() (healthy bool) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	return allHealthy(h.results)
}

func (h *health) updateResult(
	name string, details interface{}, checkDuration time.Duration, err error, t time.Time) (result Result) {

	h.lock.Lock()
	defer h.lock.Unlock()

	prevResult, ok := h.results[name]
	result = Result{
		Details:            details,
		Error:              newMarshalableError(err),
		Timestamp:          t,
		Duration:           checkDuration,
		TimeOfFirstFailure: nil,
	}

	if !result.IsHealthy() {
		if ok {
			result.ContiguousFailures = prevResult.ContiguousFailures + 1
			if prevResult.IsHealthy() {
				result.TimeOfFirstFailure = &t
			} else {
				result.TimeOfFirstFailure = prevResult.TimeOfFirstFailure
			}
		} else {
			result.ContiguousFailures = 1
			result.TimeOfFirstFailure = &t
		}
	}

	h.results[name] = result
	return result
}
