package structures

import (
	"sync"

	"github.com/kev-cao/log-console/utils/mathutils"
)

// CircularBuffer is a thread-safe circular buffer implementation.
type CircularBuffer[T any] struct {
	buf      []T
	mu       sync.Mutex
	capacity int
	size     int
	ptr      int
}

// NewCircularBuffer creates a new CircularBuffer with the given capacity.
func NewCircularBuffer[T any](capacity int) *CircularBuffer[T] {
	return &CircularBuffer[T]{
		buf:      make([]T, capacity),
		capacity: capacity,
	}
}

// Add adds a value to the buffer. If the buffer is full, the oldest value will be overwritten.
func (c *CircularBuffer[T]) Add(val T) (overwrote bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.size < c.capacity {
		c.size++
	} else {
		overwrote = true
	}

	c.buf[c.ptr] = val
	c.ptr = (c.ptr + 1) % c.capacity
	return
}

// Append adds multiple values to the buffer. If the buffer is full, the oldest values will be overwritten.
func (c *CircularBuffer[T]) Append(vals ...T) (overwrote bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.size+len(vals) <= c.capacity {
		c.size += len(vals)
	} else {
		overwrote = true
		c.size = c.capacity
	}

	for _, val := range vals {
		c.buf[c.ptr] = val
		c.ptr = (c.ptr + 1) % c.capacity
	}
	return
}

// Get returns a slice of the values in the buffer in the order they were added.
func (c *CircularBuffer[T]) Get() []T {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.size == 0 {
		return nil
	}

	res := make([]T, 0, c.size)
	start := mathutils.FloorMod(c.ptr-c.size, c.capacity)
	for i := 0; i < c.size; i++ {
		res = append(res, c.buf[(start+i)%c.capacity])
	}
	return res
}

// Clear removes all values from the buffer.
func (c *CircularBuffer[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.size = 0
	c.ptr = 0
}

// Size returns the number of values in the buffer.
func (c *CircularBuffer[T]) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.size
}

// Capacity returns the maximum number of values the buffer can hold.
func (c *CircularBuffer[T]) Capacity() int {
	return c.capacity
}

// IsFull returns true if the buffer is full.
func (c *CircularBuffer[T]) IsFull() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Size() == c.Capacity()
}
