package structures

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	// Manually construct buffer to avoid test dependencies
	t.Run("Get without wrapping", func(t *testing.T) {
		t.Run("Empty buffer", func(t *testing.T) {
			buf := CircularBuffer[int]{
				buf:      []int{},
				capacity: 5,
				size:     0,
				ptr:      0,
			}
			require.Empty(t, buf.Get())
		})

		t.Run("Non-full non-empty buffer start at 0", func(t *testing.T) {
			buf := CircularBuffer[int]{
				buf:      []int{1, 2, 3, 4, 5},
				capacity: 5,
				size:     3,
				ptr:      3,
			}
			require.Equal(t, []int{1, 2, 3}, buf.Get())
		})

		t.Run("Non-full non-empty buffer start in middle", func(t *testing.T) {
			buf := CircularBuffer[int]{
				buf:      []int{1, 2, 3, 4, 5},
				capacity: 5,
				size:     3,
				ptr:      4,
			}
			require.Equal(t, []int{2, 3, 4}, buf.Get())
		})

		t.Run("Full buffer start at 0", func(t *testing.T) {
			buf := CircularBuffer[int]{
				buf:      []int{1, 2, 3, 4, 5},
				capacity: 5,
				size:     5,
				ptr:      0,
			}
			require.Equal(t, []int{1, 2, 3, 4, 5}, buf.Get())
		})
	})

	t.Run("Get with wrapping", func(t *testing.T) {
		t.Run("Non-full buffer", func(t *testing.T) {
			buf := CircularBuffer[int]{
				buf:      []int{1, 2, 3, 4, 5},
				capacity: 5,
				size:     4,
				ptr:      2,
			}
			require.Equal(t, []int{4, 5, 1, 2}, buf.Get())
		})

		t.Run("Full buffer start at 0", func(t *testing.T) {
			buf := CircularBuffer[int]{
				buf:      []int{1, 2, 3, 4, 5},
				capacity: 5,
				size:     5,
				ptr:      0,
			}
			require.Equal(t, []int{1, 2, 3, 4, 5}, buf.Get())
		})

		t.Run("Full buffer start in middle", func(t *testing.T) {
			buf := CircularBuffer[int]{
				buf:      []int{1, 2, 3, 4, 5},
				capacity: 5,
				size:     5,
				ptr:      2,
			}
			require.Equal(t, []int{4, 5, 1, 2, 3}, buf.Get())
		})
	})
}

func TestAdd(t *testing.T) {
	t.Run("Add without overfilling", func(t *testing.T) {
		buf := NewCircularBuffer[int](5)
		for i := 1; i <= 5; i++ {
			require.False(t, buf.Add(i))
		}
		require.Equal(t, []int{1, 2, 3, 4, 5}, buf.Get())
	})

	t.Run("Add with overfilling", func(t *testing.T) {
		buf := NewCircularBuffer[int](5)
		for i := 1; i <= 5; i++ {
			require.False(t, buf.Add(i))
		}
		require.True(t, buf.Add(6))
		require.True(t, buf.Add(7))
		require.Equal(t, []int{3, 4, 5, 6, 7}, buf.Get())
	})

	t.Run("Add with overfill twice", func(t *testing.T) {
		buf := NewCircularBuffer[int](5)
		for i := 1; i <= 5; i++ {
			require.False(t, buf.Add(i))
		}
		for i := 6; i <= 10; i++ {
			require.True(t, buf.Add(i))
		}
		require.Equal(t, []int{6, 7, 8, 9, 10}, buf.Get())
	})
}

func TestAppend(t *testing.T) {
	t.Run("Append without overfilling", func(t *testing.T) {
		buf := NewCircularBuffer[int](5)
		require.False(t, buf.Append(1, 2, 3))
		require.Equal(t, []int{1, 2, 3}, buf.Get())
	})

	t.Run("Append with overfilling", func(t *testing.T) {
		buf := NewCircularBuffer[int](5)
		require.True(t, buf.Append(1, 2, 3, 4, 5, 6, 7))
		require.Equal(t, []int{3, 4, 5, 6, 7}, buf.Get())
	})

	t.Run("Append to non-empty buffer without overfilling", func(t *testing.T) {
		buf := NewCircularBuffer[int](5)
		for i := 1; i <= 3; i++ {
			require.False(t, buf.Add(i))
		}
		require.False(t, buf.Append(4, 5))
		require.Equal(t, []int{1, 2, 3, 4, 5}, buf.Get())
	})

	t.Run("Append to non-empty buffer with overfilling", func(t *testing.T) {
		buf := NewCircularBuffer[int](5)
		for i := 1; i <= 3; i++ {
			require.False(t, buf.Add(i))
		}
		require.True(t, buf.Append(4, 5, 6, 7))
		require.Equal(t, []int{3, 4, 5, 6, 7}, buf.Get())
	})
}

func TestClear(t *testing.T) {
	t.Run("Clear empty buffer", func(t *testing.T) {
		buf := NewCircularBuffer[int](5)
		buf.Clear()
		require.Empty(t, buf.Get())
	})

	t.Run("Clear non-empty buffer", func(t *testing.T) {
		buf := NewCircularBuffer[int](5)
		for i := 1; i <= 5; i++ {
			require.False(t, buf.Add(i))
		}
		buf.Clear()
		require.Empty(t, buf.Get())
	})
}
