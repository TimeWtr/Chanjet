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
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/TimeWtr/Chanjet/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fillRealisticData(data []byte) []byte {
	timestamp := uint64(time.Now().UnixNano())
	binary.BigEndian.PutUint64(data[0:8], timestamp)

	rand.Read(data[8:32])
	copy(data[32:40], []byte("HEADER"))
	copy(data[40:100], []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit."))
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
		sb := newSmartBuffer(100)
		defer sb.Close()

		data := make([]byte, 128*1024)
		data = fillRealisticData(data)
		for i := 0; i < 100; i++ {
			rand.Read(data)
			ok := sb.write(data)
			require.True(t, ok)
		}

		for i := 1; i <= 100; i++ {
			readData, ok := sb.zeroCopyRead()
			require.True(t, ok)
			assert.Equal(t, data, readData)
			assert.Equal(t, 100-i, sb.len())
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

func TestDoubleBuffer_BasicWriteRead(t *testing.T) {
	sc, err := config.NewSwitchCondition(config.SwitchConfig{
		SizeThreshold:    100,
		PercentThreshold: 80,
		TimeThreshold:    time.Second * 5,
	})
	require.NoError(t, err)

	db, err := NewDoubleBuffer(5, sc)
	require.NoError(t, err)
	defer db.Close()

	template := "test data, number: %d"
	for i := 0; i < 100; i++ {
		err = db.Write([]byte(fmt.Sprintf(template, i+1)))
		require.NoError(t, err)
	}

	for i := 0; i < 100; i++ {
		res, err1 := db.ZeroCopyRead()
		assert.NoError(t, err1)
		t.Log(string(res))
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

		for i := 0; i < 50; i++ {
			err = db.Write([]byte{byte(i)})
			require.NoError(t, err)
		}

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
