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
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/semaphore"
)

func TestCounting_int(t *testing.T) {
	p, err := NewWrapPool[int](
		func() int { return -1 },
		nil,
		nil,
		10,
	)
	assert.NoError(t, err)

	const total = 10000
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			obj, err := p.Get()
			assert.NoError(t, err)
			assert.NotNil(t, obj)
			p.Put(obj)
		}()
	}
	wg.Wait()

	a, r, _ := p.Stats()
	if p.stats.totalGets.Load() != total {
		t.Fatalf("totalGets计数错误，期望%d，实际%d", total, p.stats.totalGets.Load())
	}
	if a+r != total {
		t.Fatalf("统计不匹配: allocations(%d) + reuses(%d) != total(%d)", a, r, total)
	}
	t.Logf("totalGets计数: %d, allocations计数：%d", p.stats.totalGets.Load(), p.stats.allocations.Load())
}

func TestCounting_string(t *testing.T) {
	p, err := NewWrapPool[string](
		func() string { return "" },
		nil,
		nil,
		10,
	)
	assert.NoError(t, err)

	const total = 10000
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			obj, err := p.Get()
			assert.NoError(t, err)
			assert.NotNil(t, obj)
			p.Put(obj)
		}()
	}
	wg.Wait()

	a, r, _ := p.Stats()
	if p.stats.totalGets.Load() != total {
		t.Fatalf("totalGets计数错误，期望%d，实际%d", total, p.stats.totalGets.Load())
	}
	if a+r != total {
		t.Fatalf("统计不匹配: allocations(%d) + reuses(%d) != total(%d)", a, r, total)
	}
	t.Logf("totalGets计数: %d, allocations计数：%d", p.stats.totalGets.Load(), p.stats.allocations.Load())
}

func TestCounting_chan(t *testing.T) {
	p, err := NewWrapPool[chan string](
		func() chan string { return make(chan string, 10) },
		func(ch chan string) chan string {
			for {
				select {
				case <-ch:
				default:
					return ch
				}
			}
		},
		func(ch chan string) { close(ch) },
		20,
	)
	assert.NoError(t, err)
	defer p.Close()

	const total = 20000
	sem := semaphore.NewWeighted(100)
	for i := 0; i < total; i++ {
		_ = sem.Acquire(context.Background(), 1)
		go func(i int) {
			defer sem.Release(1)
			obj, err := p.Get()
			if err != nil {
				if errors.Is(ErrPoolMaxSize, err) {
					obj, err = p.Get()
					if err != nil {
						t.Error(err)
						return
					}
				} else {
					t.Error(err)
					return
				}
			}

			for j := 0; j < 10; j++ {
				obj <- fmt.Sprintf("i: %d, v: %d", i, j)
			}
			p.Put(obj)
		}(i)
	}

	time.Sleep(time.Second)

	a, r, _ := p.Stats()
	if p.stats.totalGets.Load() != total {
		t.Fatalf("totalGets计数错误，期望%d，实际%d", total, p.stats.totalGets.Load())
	}
	if a+r != total {
		t.Fatalf("统计不匹配: allocations(%d) + reuses(%d) != total(%d)", a, r, total)
	}
	t.Logf("totalGets计数: %d, allocations计数：%d", p.stats.totalGets.Load(), p.stats.allocations.Load())
}

func TestCounting_chan_adjust(t *testing.T) {
	p, err := NewWrapPool[chan string](
		func() chan string { return make(chan string, 10) },
		func(ch chan string) chan string {
			for {
				select {
				case <-ch:
				default:
					return ch
				}
			}
		},
		func(ch chan string) { close(ch) },
		20,
	)
	assert.NoError(t, err)
	defer p.Close()

	const total = 20000
	sem := semaphore.NewWeighted(100)
	for i := 0; i < total; i++ {
		if i == 5000 {
			p.adjustMaxSize(100)
		}

		_ = sem.Acquire(context.Background(), 1)
		go func(i int) {
			defer sem.Release(1)
			obj, err := p.Get()
			if err != nil {
				if errors.Is(ErrPoolMaxSize, err) {
					obj, err = p.Get()
					if err != nil {
						t.Error(err)
						return
					}
				} else {
					t.Error(err)
					return
				}
			}

			for j := 0; j < 10; j++ {
				obj <- fmt.Sprintf("i: %d, v: %d", i, j)
			}
			p.Put(obj)
		}(i)
	}

	time.Sleep(time.Second)

	a, r, _ := p.Stats()
	if p.stats.totalGets.Load() != total {
		t.Fatalf("totalGets计数错误，期望%d，实际%d", total, p.stats.totalGets.Load())
	}
	if a+r != total {
		t.Fatalf("统计不匹配: allocations(%d) + reuses(%d) != total(%d)", a, r, total)
	}
	t.Logf("totalGets计数: %d, allocations计数：%d", p.stats.totalGets.Load(), p.stats.allocations.Load())
}

func TestCounting_chan_multi_adjust(t *testing.T) {
	p, err := NewWrapPool[chan string](
		func() chan string { return make(chan string, 10) },
		func(ch chan string) chan string {
			for {
				select {
				case <-ch:
				default:
					return ch
				}
			}
		},
		func(ch chan string) { close(ch) },
		20,
	)
	assert.NoError(t, err)
	defer p.Close()

	const total = 20000
	sem := semaphore.NewWeighted(100)
	for i := 0; i < total; i++ {
		if i == 5000 {
			p.adjustMaxSize(100)
		}

		_ = sem.Acquire(context.Background(), 1)
		go func(i int) {
			defer sem.Release(1)
			obj, err := p.Get()
			if err != nil {
				if errors.Is(ErrPoolMaxSize, err) {
					obj, err = p.Get()
					if err != nil {
						t.Error(err)
						return
					}
				} else {
					t.Error(err)
					return
				}
			}

			for j := 0; j < 10; j++ {
				obj <- fmt.Sprintf("i: %d, v: %d", i, j)
			}
			p.Put(obj)
		}(i)
	}

	time.Sleep(time.Second)

	a, r, _ := p.Stats()
	if p.stats.totalGets.Load() != total {
		t.Fatalf("totalGets计数错误，期望%d，实际%d", total, p.stats.totalGets.Load())
	}
	if a+r != total {
		t.Fatalf("统计不匹配: allocations(%d) + reuses(%d) != total(%d)", a, r, total)
	}
	t.Logf("totalGets计数: %d, allocations计数：%d", p.stats.totalGets.Load(), p.stats.allocations.Load())
}
