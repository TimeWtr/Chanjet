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

package atomicx

import (
	"math"
	"sync"
	"testing"
)

func TestInt32(t *testing.T) {
	t.Run("Basic operations", func(t *testing.T) {
		v := NewInt32(10)

		if got := v.Load(); got != 10 {
			t.Errorf("Load() = %v, want 10", got)
		}

		v.Store(20)
		if got := v.Load(); got != 20 {
			t.Errorf("Store() failed, got %v, want 20", got)
		}

		if got := v.Add(5); got != 25 {
			t.Errorf("Add() = %v, want 25", got)
		}

		if got := v.Swap(30); got != 25 {
			t.Errorf("Swap() = %v, want 25", got)
		}

		if got := v.Load(); got != 30 {
			t.Errorf("After Swap(), got %v, want 30", got)
		}

		if !v.CompareAndSwap(30, 35) {
			t.Error("CAS should have succeeded")
		}

		if v.CompareAndSwap(30, 40) {
			t.Error("CAS should have failed")
		}

		if got := v.Inc(); got != 36 {
			t.Errorf("Inc() = %v, want 36", got)
		}

		if got := v.Dec(); got != 35 {
			t.Errorf("Dec() = %v, want 35", got)
		}
	})

	t.Run("Concurrent access", func(t *testing.T) {
		const goroutines = 100
		const opsPerGoroutine = 1000

		v := NewInt32(0)
		var wg sync.WaitGroup
		wg.Add(goroutines)

		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < opsPerGoroutine; j++ {
					v.Inc()
					v.Add(2)
					v.Dec()
				}
			}()
		}

		wg.Wait()

		expected := int32(goroutines * opsPerGoroutine * 2) // (Inc+2-Dec = +2) per op
		if got := v.Load(); got != expected {
			t.Errorf("Concurrent access result = %v, want %v", got, expected)
		}
	})
}

func TestUint32(t *testing.T) {
	t.Run("Basic operations", func(t *testing.T) {
		v := NewUint32(10)

		if got := v.Load(); got != 10 {
			t.Errorf("Load() = %v, want 10", got)
		}

		v.Store(20)
		if got := v.Load(); got != 20 {
			t.Errorf("Store() failed, got %v, want 20", got)
		}

		if got := v.Add(5); got != 25 {
			t.Errorf("Add() = %v, want 25", got)
		}

		if got := v.Swap(30); got != 25 {
			t.Errorf("Swap() = %v, want 25", got)
		}

		if got := v.Load(); got != 30 {
			t.Errorf("After Swap(), got %v, want 30", got)
		}

		if !v.CompareAndSwap(30, 35) {
			t.Error("CAS should have succeeded")
		}

		if v.CompareAndSwap(30, 40) {
			t.Error("CAS should have failed")
		}

		if got := v.Inc(); got != 36 {
			t.Errorf("Inc() = %v, want 36", got)
		}

		// Test decrement at zero
		v.Store(0)
		if got := v.Dec(); got != math.MaxUint32 {
			t.Errorf("Dec() at zero = %v, want %v", got, math.MaxUint32)
		}

		// Test decrement at non-zero
		v.Store(10)
		if got := v.Dec(); got != 9 {
			t.Errorf("Dec() = %v, want 9", got)
		}
	})
}

func TestUint64(t *testing.T) {
	v := NewUint64(0)

	// Test large values
	v.Store(math.MaxUint64 - 5)

	expected1 := uint64(math.MaxUint64 - 2)
	if got := v.Add(3); got != expected1 {
		t.Errorf("Add() = %v, want %v", got, expected1)
	}

	expected2 := uint64(math.MaxUint64 - 1)
	if got := v.Inc(); got != expected2 {
		t.Errorf("Inc() = %v, want %v", got, expected2)
	}

	expected3 := uint64(math.MaxUint64)
	if got := v.Inc(); got != expected3 {
		t.Errorf("Inc() = %v, want %v", got, expected3)
	}

	expected4 := uint64(0)
	if got := v.Inc(); got != expected4 {
		t.Errorf("Inc() at max = %v, want %v", got, expected4)
	}
}

func TestBool(t *testing.T) {
	v := NewBool()

	if got := v.Load(); got != false {
		t.Errorf("Load() = %v, want false", got)
	}

	v.Store(true)
	if got := v.Load(); got != true {
		t.Errorf("Store() failed, got %v, want true", got)
	}

	// Test CAS
	if !v.CompareAndSwap(true, false) {
		t.Error("CAS should have succeeded")
	}

	if got := v.Load(); got != false {
		t.Errorf("After CAS, got %v, want false", got)
	}

	if v.CompareAndSwap(true, true) {
		t.Error("CAS should have failed")
	}
}

//func TestFloat64(t *testing.T) {
//	v := NewFloat64(0.0)
//
//	// Store and load
//	v.Store(3.14159)
//	if got := v.Load(); got != 3.14159 {
//		t.Errorf("Load() = %v, want 3.14159", got)
//	}
//
//	// Swap
//	old := v.Swap(2.71828)
//	if old != 3.14159 {
//		t.Errorf("Swap() = %v, want 3.14159", old)
//	}
//
//	// CAS
//	if !v.CompareAndSwap(2.71828, 1.61803) {
//		t.Error("CAS should have succeeded")
//	}
//
//	if got := v.Load(); got != 1.61803 {
//		t.Errorf("After CAS, got %v, want 1.61803", got)
//	}
//}
//
//func TestConcurrency(t *testing.T) {
//	const (
//		goroutines = 100
//		duration   = 2 * time.Second
//	)
//
//	tests := []struct {
//		name string
//		init func() interface {
//			Load() interface{}
//			Add(interface{}) interface{}
//		}
//		expected interface{}
//	}{
//		{
//			name: "Int32",
//			init: func() interface {
//				Load() interface{}
//				Add(interface{}) interface{}
//			} {
//				return NewInt32(0)
//			},
//			expected: int32(goroutines * 1000), // Each goroutine does 1000 increments
//		},
//		{
//			name: "Uint32",
//			init: func() interface {
//				Load() interface{}
//				Add(interface{}) interface{}
//			} {
//				return NewUint32(0)
//			},
//			expected: uint32(goroutines * 1000),
//		},
//		{
//			name: "Int64",
//			init: func() interface {
//				Load() interface{}
//				Add(interface{}) interface{}
//			} {
//				return NewInt64(0)
//			},
//			expected: int64(goroutines * 1000),
//		},
//		{
//			name: "Uint64",
//			init: func() interface {
//				Load() interface{}
//				Add(interface{}) interface{}
//			} {
//				return NewUint64(0)
//			},
//			expected: uint64(goroutines * 1000),
//		},
//	}
//
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			counter := tt.init()
//			var wg sync.WaitGroup
//			wg.Add(goroutines)
//
//			stop := time.Now().Add(duration)
//			for i := 0; i < goroutines; i++ {
//				go func() {
//					defer wg.Done()
//					ops := 0
//					for time.Now().Before(stop) {
//						counter.Add(1)
//						ops++
//						// Ensure we don't run too many ops in tests
//						if ops > 1000 {
//							break
//						}
//					}
//				}()
//			}
//
//			wg.Wait()
//
//			if got := counter.Load(); got != tt.expected {
//				t.Errorf("Concurrency test failed for %s: got %v, want %v", tt.name, got, tt.expected)
//			}
//		})
//	}
//}
//
