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
	"encoding/binary"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/TimeWtr/Chanjet"
	"github.com/TimeWtr/Chanjet/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/net/context"
)

func fillRealisticData(data []byte) []byte {
	timestamp := uint64(time.Now().UnixNano())
	binary.BigEndian.PutUint64(data[0:8], timestamp)

	rand.Read(data[8:32])
	copy(data[32:40], "HEADER")
	copy(data[40:100], "Lorem ipsum dolor sit amet, consectetur adipiscing elit.")
	rand.Read(data[100:])
	return data
}

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

	t.Run("Write and zero-copy read large data(64KB)", func(t *testing.T) {
		sb := newSmartBuffer(10)
		defer sb.Close()

		data := make([]byte, 64*1024)
		data = fillRealisticData(data)
		for i := 0; i < 10; i++ {
			rand.Read(data)
			ok := sb.write(data)
			require.True(t, ok)
		}

		for i := 1; i <= 10; i++ {
			readData, ok := sb.zeroCopyRead()
			require.True(t, ok)
			assert.Equal(t, data, readData)
			assert.Equal(t, 10-i, sb.len())
		}
	})

	t.Run("Write and zero-copy read large data(128KB)", func(t *testing.T) {
		sb := newSmartBuffer(200)
		defer sb.Close()

		data := make([]byte, 128*1024)
		data = fillRealisticData(data)
		for i := 0; i < 200; i++ {
			rand.Read(data)
			ok := sb.write(data)
			require.True(t, ok)
		}

		for i := 1; i <= 200; i++ {
			readData, ok := sb.zeroCopyRead()
			require.True(t, ok)
			assert.Equal(t, data, readData)
			assert.Equal(t, 200-i, sb.len())
		}
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

func TestDoubleBuffer_BlockingRead(t *testing.T) {
	sc, err := config.NewSwitchCondition(config.SwitchConfig{
		PercentThreshold: 80,
		TimeThreshold:    time.Second * 5,
	})
	require.NoError(t, err)

	db, err := NewDoubleBuffer(1000, sc)
	require.NoError(t, err)
	defer db.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		data := make([]byte, 64*1024)
		data = fillRealisticData(data)

		for i := 0; i < 20000; i++ {
			err = db.Write(data)
			require.NoError(t, err)
		}
	}()

	go func() {
		defer wg.Done()

		_ = db.RegisterReadMode(Chanjet.SafeRead)

		count := 0
		for {
			if count >= 20000 {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_, err1 := db.BlockingRead(ctx)
			cancel()
			if err1 != nil {
				t.Log(err1)
				continue
			}
			//t.Log("receive data: ", string(res))
			count++
		}
	}()

	wg.Wait()
}

func TestDoubleBuffer_SwitchConditions(t *testing.T) {
	t.Run("Switch by capacity", func(t *testing.T) {
		sc, err := config.NewSwitchCondition(config.SwitchConfig{
			PercentThreshold: 80,
			TimeThreshold:    5 * time.Second,
		})
		require.NoError(t, err)

		db, err := NewDoubleBuffer(20, sc)
		require.NoError(t, err)
		defer db.Close()

		for i := 0; i < 50; i++ {
			err = db.Write([]byte{byte(i)})
			require.NoError(t, err)
		}

	})

	t.Run("Switch by time", func(t *testing.T) {
		sc, err := config.NewSwitchCondition(config.SwitchConfig{
			PercentThreshold: 80,
			TimeThreshold:    time.Millisecond * 50,
		})
		require.NoError(t, err)

		db, err := NewDoubleBuffer(20, sc)
		require.NoError(t, err)
		defer db.Close()
		db.swapSignal = make(chan struct{}, 1)

		db.Write([]byte("test"))

		// Wait for time-based switch
		time.Sleep(100 * time.Millisecond)

		assert.Equal(t, 1, db.pendingHeap.Len(), "expected one item in heap")
	})

	t.Run("Switch by combined factors", func(t *testing.T) {
		sc, err := config.NewSwitchCondition(config.SwitchConfig{
			PercentThreshold: 80,
			TimeThreshold:    time.Millisecond * 60,
		})
		require.NoError(t, err)

		db, err := NewDoubleBuffer(20, sc)
		require.NoError(t, err)
		defer db.Close()
		db.swapSignal = make(chan struct{}, 1)

		// Write enough data to be above threshold but below capacity
		for i := 0; i < 10; i++ {
			_ = db.Write([]byte{byte(i)})
		}

		// Should trigger switch after time passes
		time.Sleep(100 * time.Millisecond)

		assert.Equal(t, 1, db.pendingHeap.Len(), "expected one item in heap")
	})
}

const (
	TestBufferSize = 10000
	TestItems      = 1000000
	DataSize       = 128
)

func BenchmarkBlockingRead_Throughput(b *testing.B) {
	defer goleak.VerifyNone(b,
		goleak.IgnoreCurrent(),
	)

	sc, err := config.NewSwitchCondition(config.SwitchConfig{
		PercentThreshold: 80,
		TimeThreshold:    time.Second * 5,
	})

	db, err := NewDoubleBuffer(TestBufferSize, sc)
	assert.NoError(b, err)
	defer db.Close()

	err = db.RegisterReadMode(Chanjet.ZeroCopyRead)
	assert.NoError(b, err)

	go func() {
		data := make([]byte, DataSize)
		for i := 0; i < b.N; i++ {
			if err = db.Write(data); err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err1 := db.BlockingRead(ctx)
		if err1 != nil {
			b.Fatalf("BlockingRead failed: %v", err1)
		}
	}

	b.StopTimer()
}

func BenchmarkBlockingRead_PerfMetrics(b *testing.B) {
	defer goleak.VerifyNone(b, goleak.IgnoreCurrent())

	// Configuration
	const (
		TestBufferSize = 1024 * 1024 // 1MB buffer
		DataSize       = 1024        // 1KB messages
		WarmupMessages = 100000      // Pre-fill buffer
	)

	// Initialize
	sc, err := config.NewSwitchCondition(config.SwitchConfig{
		PercentThreshold: 80,
		TimeThreshold:    time.Second * 5,
	})
	if err != nil {
		b.Fatal(err)
	}

	db, err := NewDoubleBuffer(TestBufferSize, sc)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	if err := db.RegisterReadMode(Chanjet.ZeroCopyRead); err != nil {
		b.Fatal(err)
	}

	// Pre-fill buffer
	warmupData := make([]byte, DataSize)
	for i := 0; i < WarmupMessages; i++ {
		if err := db.Write(warmupData); err != nil {
			b.Fatal(err)
		}
	}

	// Writer goroutine (runs for entire benchmark duration)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		data := make([]byte, DataSize)
		for ctx.Err() == nil {
			if err := db.Write(data); err != nil {
				return
			}
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()

		// Operation we're benchmarking
		data, err := db.BlockingRead(ctx)
		if err != nil {
			b.Fatal(err)
		}
		_ = data // Prevent optimization

		// Report custom metrics
		latency := time.Since(start)
		b.ReportMetric(float64(latency.Nanoseconds()), "ns/msg")

		// Get memory stats
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		b.ReportMetric(float64(m.Mallocs), "allocs/msg")
		b.ReportMetric(float64(m.TotalAlloc), "bytes/msg")
	}

	// Report throughput at the end
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "msgs/s")
}

func BenchmarkBlockingRead_PerfMetrics_64KB(b *testing.B) {
	defer goleak.VerifyNone(b, goleak.IgnoreCurrent())

	// Configuration
	const (
		TestBufferSize = 1024 * 1024 // 1MB buffer
		DataSize       = 1024 * 64   // 1KB messages
		WarmupMessages = 10000       // Pre-fill buffer
	)

	// Initialize
	sc, err := config.NewSwitchCondition(config.SwitchConfig{
		PercentThreshold: 80,
		TimeThreshold:    time.Second * 5,
	})
	if err != nil {
		b.Fatal(err)
	}

	db, err := NewDoubleBuffer(TestBufferSize, sc)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	if err = db.RegisterReadMode(Chanjet.ZeroCopyRead); err != nil {
		b.Fatal(err)
	}

	// Pre-fill buffer
	warmupData := make([]byte, DataSize)
	for i := 0; i < WarmupMessages; i++ {
		if err = db.Write(warmupData); err != nil {
			b.Fatal(err)
		}
	}

	// Writer goroutine (runs for entire benchmark duration)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		data := make([]byte, DataSize)
		for ctx.Err() == nil {
			if err = db.Write(data); err != nil {
				return
			}
		}
	}()

	// Benchmark loop - this is what go test -bench measures
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()

		// This is the operation we're benchmarking
		data, err := db.BlockingRead(ctx)
		if err != nil {
			b.Fatalf("BlockingRead failed: %v", err)
		}
		if len(data) != DataSize {
			b.Fatalf("Unexpected data size: got %d, want %d", len(data), DataSize)
		}

		// Report custom metrics per operation
		b.ReportMetric(float64(time.Since(start).Nanoseconds()), "ns/op")

		// Memory stats (optional)
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		b.ReportMetric(float64(m.Mallocs), "allocs/op")
		b.ReportMetric(float64(m.TotalAlloc), "bytes/op")
	}
	b.StopTimer()
}
