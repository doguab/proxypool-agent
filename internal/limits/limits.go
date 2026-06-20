package limits

import "sync"

type Semaphore struct {
	ch chan struct{}
}

func New(max int) *Semaphore {
	if max <= 0 {
		max = 1
	}
	return &Semaphore{ch: make(chan struct{}, max)}
}

func (s *Semaphore) Acquire() {
	s.ch <- struct{}{}
}

func (s *Semaphore) Release() {
	select {
	case <-s.ch:
	default:
	}
}

type Counter struct {
	mu    sync.Mutex
	count int
	max   int
}

func NewCounter(max int) *Counter {
	return &Counter{max: max}
}

func (c *Counter) TryInc() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.count >= c.max {
		return false
	}
	c.count++
	return true
}

func (c *Counter) Dec() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.count > 0 {
		c.count--
	}
}

func (c *Counter) Current() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}
