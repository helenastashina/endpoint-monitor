package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"endpoint-monitor/internal/monitor"
)

type Job struct {
	Monitor monitor.Monitor
}

type DueLister func(ctx context.Context) ([]monitor.Monitor, error)

type Scheduler struct {
	listDue        DueLister
	jobs           chan<- Job
	done           <-chan int64
	tick           time.Duration
	inflight       map[int64]struct{}
	mu             sync.Mutex
	stopped        bool
	enqueueStopped chan struct{}
	stopOnce       sync.Once
}

func New(listDue DueLister, jobs chan<- Job, done <-chan int64, tick time.Duration) *Scheduler {
	return &Scheduler{
		listDue:        listDue,
		jobs:           jobs,
		done:           done,
		tick:           tick,
		inflight:       make(map[int64]struct{}),
		enqueueStopped: make(chan struct{}),
	}
}

func (s *Scheduler) EnqueueStopped() <-chan struct{} {
	return s.enqueueStopped
}

func (s *Scheduler) Run(ctx context.Context, shutdownComplete <-chan struct{}) {
	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()

	polling := true
	for {
		select {
		case <-shutdownComplete:
			return
		case <-ctx.Done():
			if polling {
				polling = false
				ticker.Stop()
				s.stopEnqueue()
			}
		case id := <-s.done:
			s.releaseInflight(id)
		case <-ticker.C:
			if polling {
				s.pollDue(ctx)
			}
		}
	}
}

func (s *Scheduler) stopEnqueue() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.stopOnce.Do(func() {
		close(s.enqueueStopped)
	})
}

func (s *Scheduler) pollDue(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	monitors, err := s.listDue(ctx)
	if err != nil {
		log.Printf("scheduler: list due monitors: %v", err)
		return
	}

	for _, m := range monitors {
		s.tryEnqueue(m)
	}
}

func (s *Scheduler) tryEnqueue(m monitor.Monitor) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return
	}
	if _, ok := s.inflight[m.ID]; ok {
		return
	}
	s.inflight[m.ID] = struct{}{}

	select {
	case s.jobs <- Job{Monitor: m}:
	default:
		delete(s.inflight, m.ID)
		log.Printf("scheduler: job queue full, defer monitor %d", m.ID)
	}
}

func (s *Scheduler) releaseInflight(id int64) {
	s.mu.Lock()
	delete(s.inflight, id)
	s.mu.Unlock()
}

func (s *Scheduler) InFlight(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.inflight[id]
	return ok
}
