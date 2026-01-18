package perf

import (
	"sync/atomic"
)

type OpCounter struct {
	name  string
	value int64
}

func NewOpCounter(name string) *OpCounter {
	return &OpCounter{name: name}
}

func (c *OpCounter) Inc() {
	atomic.AddInt64(&c.value, 1)
}

func (c *OpCounter) Value() int64 {
	return atomic.LoadInt64(&c.value)
}

func (c *OpCounter) Reset() {
	atomic.StoreInt64(&c.value, 0)
}
