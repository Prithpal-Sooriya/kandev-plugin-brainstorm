package brainstorm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// JobStatus is the lifecycle of one agent turn.
type JobStatus string

const (
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobError   JobStatus = "error"
)

// Job is a snapshot of one turn as seen by a poller.
type Job struct {
	ID     string      `json:"id"`
	Status JobStatus   `json:"status"`
	Result *TurnResult `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`

	finishedAt time.Time
}

// Runner executes one turn. It must honour ctx cancellation.
type Runner func(ctx context.Context) (TurnResult, error)

// Jobs runs agent turns in the background and keeps finished results for a
// short retention window so the browser can collect them by polling. State
// is in-memory on purpose: a turn is cheap to retry, and a plugin restart
// simply surfaces as "job not found" to the poller.
type Jobs struct {
	mu        sync.Mutex
	jobs      map[string]*Job
	timeout   time.Duration
	retention time.Duration
	maxActive int
	now       func() time.Time
}

// NewJobs returns a store whose turns are cancelled after timeout and whose
// finished results are forgotten retention after completion.
func NewJobs(timeout, retention time.Duration, maxActive int) *Jobs {
	return &Jobs{
		jobs:      map[string]*Job{},
		timeout:   timeout,
		retention: retention,
		maxActive: maxActive,
		now:       time.Now,
	}
}

// ErrBusy is returned by Start when too many turns are already running.
type ErrBusy struct{}

func (ErrBusy) Error() string {
	return "too many brainstorm turns are already running; try again shortly"
}

// Start launches run in the background and returns its job id.
func (j *Jobs) Start(run Runner) (string, error) {
	j.mu.Lock()
	j.gcLocked()
	active := 0
	for _, job := range j.jobs {
		if job.Status == JobRunning {
			active++
		}
	}
	if j.maxActive > 0 && active >= j.maxActive {
		j.mu.Unlock()
		return "", ErrBusy{}
	}
	id := newID()
	j.jobs[id] = &Job{ID: id, Status: JobRunning}
	j.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), j.timeout)
		defer cancel()
		result, err := run(ctx)
		j.mu.Lock()
		defer j.mu.Unlock()
		job, ok := j.jobs[id]
		if !ok {
			return
		}
		job.finishedAt = j.now()
		if err != nil {
			job.Status = JobError
			job.Error = err.Error()
			return
		}
		job.Status = JobDone
		job.Result = &result
	}()
	return id, nil
}

// Get returns a copy of the job, or false when it is unknown or expired.
func (j *Jobs) Get(id string) (Job, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.gcLocked()
	job, ok := j.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *job, true
}

func (j *Jobs) gcLocked() {
	cutoff := j.now().Add(-j.retention)
	for id, job := range j.jobs {
		if job.Status != JobRunning && job.finishedAt.Before(cutoff) {
			delete(j.jobs, id)
		}
	}
}

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
