// Copyright 2025 TimeWtr
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package core

import (
	"container/heap"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/TimeWtr/Chanjet/config"
	"github.com/TimeWtr/Chanjet/errorx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmartBuffer_BasicOperations(t *testing.T) {
	t.Run("Write and safe read small data", func(t *testing.T) {
		sb := newSmartBuffer(10)
		defer sb.Close()

		data := []byte("hello")
		ok := sb.write(data)
		require.True(t, ok)

		readData, ok := sb.safeRead()
		require.True(t, ok)
		assert.Equal(t, data, readData)
		assert.Equal(t, 0, sb.len())
	})

	t.Run("Write and zero copy read small data", func(t *testing.T) {
		sb := newSmartBuffer(10)
		defer sb.Close()

		data := []byte("hello")
		ok := sb.write(data)
		require.True(t, ok)
		time.Sleep(time.Millisecond * 10)
		readData, ok := sb.zeroCopyRead()
		require.True(t, ok)
		assert.Equal(t, data, readData)
		assert.Equal(t, 0, sb.len())
	})

	t.Run("Write and zero-copy read large data", func(t *testing.T) {
		sb := newSmartBuffer(10)
		defer sb.Close()

		data := make([]byte, 64*1024) // 64KB
		rand.Read(data)
		ok := sb.write(data)
		require.True(t, ok)

		readData, ok := sb.zeroCopyRead()
		require.True(t, ok)
		assert.Equal(t, data, readData)
		assert.Equal(t, 0, sb.len())
	})

	t.Run("Buffer full", func(t *testing.T) {
		sb := newSmartBuffer(2)
		defer sb.Close()

		require.True(t, sb.write([]byte("a")))
		require.True(t, sb.write([]byte("b")))
		require.False(t, sb.write([]byte("c")))

		// Read one item
		_, ok := sb.safeRead()
		require.True(t, ok)

		// Now should be able to write again
		require.True(t, sb.write([]byte("c")))
	})

	t.Run("Close behavior", func(t *testing.T) {
		sb := newSmartBuffer(5)
		sb.write([]byte("test"))

		sb.Close()

		// Writes after close should fail
		ok := sb.write([]byte("new"))
		require.False(t, ok)

		// Can read existing data
		data, ok := sb.safeRead()
		require.True(t, ok)
		assert.Equal(t, []byte("test"), data)

		// Subsequent reads should fail
		_, ok = sb.safeRead()
		require.False(t, ok)
	})

	t.Run("Recycle worker", func(t *testing.T) {
		sb := newSmartBuffer(10)

		// Write medium data that should be cached
		mediumData := make([]byte, 16*1024) // 16KB
		sb.write(mediumData)

		// Immediately read should work without copy
		data, ok := sb.zeroCopyRead()
		require.True(t, ok)
		assert.Equal(t, mediumData, data)

		// Wait for cache to expire
		time.Sleep(MediumDataCacheDuration + 100*time.Millisecond)

		// Now should return copy
		newData := make([]byte, len(mediumData))
		copy(newData, mediumData)
		sb.write(newData)
		readData, ok := sb.safeRead()
		require.True(t, ok)
		assert.Equal(t, mediumData, readData)

		sb.Close()
	})
}

func TestDoubleBuffer_BasicWriteRead(t *testing.T) {
	sc, err := config.NewSwitchCondition(config.SwitchConfig{
		SizeThreshold:    100,
		PercentThreshold: 80,
		TimeThreshold:    time.Second * 5,
	})
	require.NoError(t, err)

	db, err := NewDoubleBuffer(20, sc)
	require.NoError(t, err)
	defer db.Close()

	data := []byte("test data")
	for i := 0; i < 10; i++ {
		err = db.Write(data)
		require.NoError(t, err)
	}

	for i := 0; i < 10; i++ {
		res, ok := db.active.zeroCopyRead()
		assert.True(t, ok)
		assert.Equal(t, data, res)
	}
}

func TestDoubleBuffer_SwitchConditions(t *testing.T) {
	t.Run("Switch by capacity", func(t *testing.T) {
		sc, err := config.NewSwitchCondition(config.SwitchConfig{
			SizeThreshold:    50,
			PercentThreshold: 80,
			TimeThreshold:    10 * time.Second,
		})
		require.NoError(t, err)

		db, err := NewDoubleBuffer(20, sc)
		require.NoError(t, err)
		defer db.Close()

		go func() {
			for {
				select {
				case data, ok := <-db.readq:
					if !ok {
						return
					}
					t.Log("read data: ", data)
				}
			}
		}()

		for i := 0; i < 50; i++ {
			err = db.Write([]byte{byte(i)})
			require.NoError(t, err)
		}

		time.Sleep(time.Millisecond * 10)
	})

	t.Run("Switch by time", func(t *testing.T) {
		sc, err := config.NewSwitchCondition(config.SwitchConfig{
			SizeThreshold:    100,
			PercentThreshold: 80,
			TimeThreshold:    time.Second,
		})
		require.NoError(t, err)

		db, err := NewDoubleBuffer(100, sc)
		require.NoError(t, err)
		defer db.Close()
		db.swapSignal = make(chan struct{}, 1)
		//db.interval = 10 * time.Millisecond // Very short interval

		db.Write([]byte("test"))

		// Wait for time-based switch
		time.Sleep(50 * time.Millisecond)

		assert.Equal(t, 1, db.pendingHeap.Len(), "expected one item in heap")
	})

	t.Run("Switch by combined factors", func(t *testing.T) {
		sc, err := config.NewSwitchCondition(config.SwitchConfig{
			SizeThreshold:    100,
			PercentThreshold: 80,
			TimeThreshold:    time.Second,
		})
		require.NoError(t, err)

		db, err := NewDoubleBuffer(10, sc)
		require.NoError(t, err)
		defer db.Close()
		db.swapSignal = make(chan struct{}, 1)
		//db.interval = 20 * time.Millisecond

		// Write enough data to be above threshold but below capacity
		for i := 0; i < 8; i++ {
			db.Write([]byte{byte(i)})
		}

		// Should trigger switch after time passes
		time.Sleep(50 * time.Millisecond)

		assert.Equal(t, 1, db.pendingHeap.Len(), "expected one item in heap")
	})
}

func TestDoubleBuffer_ConcurrentWrites(t *testing.T) {
	sc, err := config.NewSwitchCondition(config.SwitchConfig{
		SizeThreshold:    100,
		PercentThreshold: 80,
		TimeThreshold:    time.Second,
	})
	require.NoError(t, err)

	db, err := NewDoubleBuffer(1000, sc)
	assert.NoError(t, err)
	defer db.Close()
	db.swapSignal = make(chan struct{}, 10)
	db.readq = make(chan [][]byte, 1000)

	const (
		numWriters = 10
		numWrites  = 100
	)

	var wg sync.WaitGroup
	wg.Add(numWriters)

	for i := 0; i < numWriters; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numWrites; j++ {
				data := []byte{byte(id), byte(j)}
				err := db.Write(data)
				require.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// Manually trigger any pending switches
	for i := 0; i < 10; i++ {
		select {
		case db.swapSignal <- struct{}{}:
			db.switchChannel()
		default:
		}
	}

	// Let processing complete
	time.Sleep(500 * time.Millisecond)

	// Collect all results
	results := make(map[[2]byte]bool)
	for len(results) < numWriters*numWrites {
		select {
		case batch := <-db.readq:
			for _, data := range batch {
				if len(data) == 2 {
					key := [2]byte{data[0], data[1]}
					results[key] = true
				}
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out, received %d/%d items", len(results), numWriters*numWrites)
		}
	}

	assert.Equal(t, numWriters*numWrites, len(results))
}

func TestDoubleBuffer_Ordering(t *testing.T) {
	sc, err := config.NewSwitchCondition(config.SwitchConfig{
		SizeThreshold:    100,
		PercentThreshold: 80,
		TimeThreshold:    time.Second,
	})
	require.NoError(t, err)

	db, err := NewDoubleBuffer(5, sc)
	require.NoError(t, err)
	defer db.Close()
	db.swapSignal = make(chan struct{}, 10)
	db.readq = make(chan [][]byte, 100)

	const numItems = 100
	sent := make([][]byte, numItems)

	for i := 0; i < numItems; i++ {
		data := []byte{byte(i)}
		sent[i] = data
		db.Write(data)
	}

	// Manually trigger any pending switches
	for i := 0; i < 20; i++ {
		select {
		case db.swapSignal <- struct{}{}:
			db.switchChannel()
		default:
		}
	}

	// Collect results
	var received [][]byte
	for len(received) < numItems {
		select {
		case batch := <-db.readq:
			received = append(received, batch...)
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out, received %d/%d items", len(received), numItems)
		}
	}

	// Verify ordering
	for i := 0; i < numItems; i++ {
		if i >= len(received) {
			break
		}
		assert.Equal(t, []byte{byte(i)}, received[i], "mismatch at position %d", i)
	}
}

func TestDoubleBuffer_Close(t *testing.T) {
	t.Run("Close with pending data", func(t *testing.T) {
		sc, err := config.NewSwitchCondition(config.SwitchConfig{
			SizeThreshold:    100,
			PercentThreshold: 80,
			TimeThreshold:    time.Second,
		})
		require.NoError(t, err)

		db, err := NewDoubleBuffer(10, sc)
		require.NoError(t, err)
		db.readq = make(chan [][]byte, 10)

		// Write some data
		for i := 0; i < 5; i++ {
			db.Write([]byte{byte(i)})
		}

		// Close before processing
		db.Close()

		// Should have processed all data
		total := 0
	loop:
		for {
			select {
			case batch := <-db.readq:
				total += len(batch)
			default:
				break loop
			}
		}
		assert.Equal(t, 5, total)
	})

	t.Run("Write after close", func(t *testing.T) {
		sc, err := config.NewSwitchCondition(config.SwitchConfig{
			SizeThreshold:    100,
			PercentThreshold: 80,
			TimeThreshold:    time.Second,
		})
		require.NoError(t, err)
		db, err := NewDoubleBuffer(10, sc)
		require.NoError(t, err)
		db.Close()

		err = db.Write([]byte("test"))
		assert.Equal(t, errorx.ErrBufferClose, err)
	})

	t.Run("Double close", func(t *testing.T) {
		sc, err := config.NewSwitchCondition(config.SwitchConfig{
			SizeThreshold:    100,
			PercentThreshold: 80,
			TimeThreshold:    time.Second,
		})
		require.NoError(t, err)

		db, err := NewDoubleBuffer(10, sc)
		require.NoError(t, err)
		db.Close()
		assert.NotPanics(t, func() { db.Close() })
	})
}

func TestMinHeap(t *testing.T) {
	h := &MinHeap{}
	heap.Init(h)

	// Push out-of-order items
	items := []MinHeapItem{
		{sequence: 5, buf: &SmartBuffer{}},
		{sequence: 3, buf: &SmartBuffer{}},
		{sequence: 7, buf: &SmartBuffer{}},
		{sequence: 1, buf: &SmartBuffer{}},
	}

	for _, item := range items {
		heap.Push(h, item)
	}

	// Should pop in order
	expected := []int64{1, 3, 5, 7}
	for _, exp := range expected {
		item := heap.Pop(h).(MinHeapItem)
		assert.Equal(t, exp, item.sequence)
	}
}

func TestDoubleBuffer_HeapProcessing(t *testing.T) {
	sc, err := config.NewSwitchCondition(config.SwitchConfig{
		SizeThreshold:    100,
		PercentThreshold: 80,
		TimeThreshold:    time.Second,
	})
	require.NoError(t, err)

	db, err := NewDoubleBuffer(3, sc)
	require.NoError(t, err)
	defer db.Close()
	db.swapSignal = make(chan struct{}, 10)
	db.readq = make(chan [][]byte, 100)

	// Write enough data to cause multiple switches
	for i := 0; i < 10; i++ {
		db.Write([]byte{byte(i)})
	}

	// Manually trigger any pending switches
	for i := 0; i < 10; i++ {
		select {
		case db.swapSignal <- struct{}{}:
			db.switchChannel()
		default:
		}
	}

	// Collect results
	var received []byte
	for len(received) < 10 {
		select {
		case batch := <-db.readq:
			for _, data := range batch {
				if len(data) == 1 {
					received = append(received, data[0])
				}
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out, received %d/10 items", len(received))
		}
	}

	// Verify all items received in order
	for i := 0; i < 10; i++ {
		assert.Equal(t, byte(i), received[i], "mismatch at position %d", i)
	}
}

func TestDoubleBuffer_MemoryReclamation(t *testing.T) {
	runTest := func() int {
		sc, err := config.NewSwitchCondition(config.SwitchConfig{
			SizeThreshold:    100,
			PercentThreshold: 80,
			TimeThreshold:    time.Second,
		})
		require.NoError(t, err)

		db, err := NewDoubleBuffer(10, sc)
		require.NoError(t, err)
		db.readq = make(chan [][]byte, 100)

		for i := 0; i < 100; i++ {
			db.Write(make([]byte, 1024))
		}

		// Manually trigger any pending switches
		for i := 0; i < 10; i++ {
			select {
			case db.swapSignal <- struct{}{}:
				db.switchChannel()
			default:
			}
		}

		// Drain the read queue
		go func() {
			for range db.readq {
			}
		}()

		db.Close()
		return 0 // We're actually interested in allocs, not return value
	}

	// First run to initialize pools
	runTest()

	// Measure allocations on subsequent runs
	allocs := testing.AllocsPerRun(10, func() {
		runTest()
	})

	assert.Less(t, allocs, 100.0, "allocations should be low due to object reuse")
}

func TestDoubleBuffer_LargeDataHandling(t *testing.T) {
	sc, err := config.NewSwitchCondition(config.SwitchConfig{
		SizeThreshold:    100,
		PercentThreshold: 80,
		TimeThreshold:    time.Second,
	})
	require.NoError(t, err)

	db, err := NewDoubleBuffer(10, sc)
	require.NoError(t, err)
	defer db.Close()
	db.swapSignal = make(chan struct{}, 1)
	db.readq = make(chan [][]byte, 10)

	// Create large data (64KB)
	largeData := make([]byte, 64*1024)
	rand.Read(largeData)

	err = db.Write(largeData)
	require.NoError(t, err)

	// Trigger switch
	db.swapSignal <- struct{}{}
	db.switchChannel()

	// Verify data
	select {
	case batch := <-db.readq:
		require.Len(t, batch, 1)
		assert.Equal(t, largeData, batch[0])
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for data")
	}
}

func TestDoubleBuffer_ZeroCopyRelease(t *testing.T) {
	sb := newSmartBuffer(10)
	defer sb.Close()

	// Create large data (64KB)
	largeData := make([]byte, 64*1024)
	rand.Read(largeData)

	ok := sb.write(largeData)
	require.True(t, ok)

	// Zero-copy read
	data, ok := sb.zeroCopyRead()
	require.True(t, ok)

	// Release the data
	sb.Release(data)

	// Verify the release (this would require access to internal pools to fully verify)
}

func TestDoubleBuffer_ProcessorTimeout(t *testing.T) {
	sc, err := config.NewSwitchCondition(config.SwitchConfig{
		SizeThreshold:    100,
		PercentThreshold: 80,
		TimeThreshold:    time.Second,
	})
	require.NoError(t, err)

	db, err := NewDoubleBuffer(10, sc)
	require.NoError(t, err)
	defer db.Close()
	db.swapSignal = make(chan struct{}, 1)
	db.readq = make(chan [][]byte, 10)

	//// Override condWithTimeout to always timeout for this test
	//db.condWithTimeout = func(timeout time.Duration) bool {
	//	return false
	//}

	// Write some data
	db.Write([]byte("test"))

	// Trigger switch
	db.swapSignal <- struct{}{}
	db.switchChannel()

	// Processor should not process due to timeout
	select {
	case <-db.readq:
		t.Fatal("unexpected data processed")
	case <-time.After(100 * time.Millisecond):
		// Expected behavior
	}
}

func TestDoubleBuffer_DrainProcessor(t *testing.T) {
	sc, err := config.NewSwitchCondition(config.SwitchConfig{
		SizeThreshold:    100,
		PercentThreshold: 80,
		TimeThreshold:    time.Second,
	})
	require.NoError(t, err)

	db, err := NewDoubleBuffer(10, sc)
	require.NoError(t, err)
	db.swapSignal = make(chan struct{}, 1)
	db.readq = make(chan [][]byte, 10)

	// Write some data
	for i := 0; i < 5; i++ {
		db.Write([]byte{byte(i)})
	}

	// Trigger switch
	db.swapSignal <- struct{}{}
	db.switchChannel()

	// Close should drain processor
	db.Close()
	assert.Equal(t, 0, db.pendingHeap.Len())
}

// Helper function to drain a channel
func drainChannel(ch chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
