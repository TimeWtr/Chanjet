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
	"errors"
	"sync"
	"sync/atomic"
)

type Stats struct {
	allocations atomic.Int64 // 总共分配的对象数量
	totalGets   atomic.Int64 // 总共获取的对象数量
	discards    atomic.Int64 // 因为池满丢弃的对象数量
}

type WrapPool[T any] struct {
	p            *sync.Pool    // 内置池
	maxSize      atomic.Int32  // 池中允许的最大对象数量
	currentCount atomic.Int32  // 当前池中的可用对象数量
	stats        Stats         // 统计计数信息
	resetFunc    func(T) T     // 重置对象函数
	newFunc      func() T      // 创建对象函数
	closeFunc    func(T)       // 在关闭Pool时关闭资源的方法
	sig          chan struct{} // 关闭的信号通知
}

func NewWrapPool[T any](fn func() T, resetFn func(T) T, closeFunc func(T), maxSize int32) (*WrapPool[T], error) {
	if fn == nil {
		return nil, errors.New("newFunc cannot be nil")
	}

	p := &WrapPool[T]{
		newFunc:   fn,
		resetFunc: resetFn,
		closeFunc: closeFunc,
		stats:     Stats{},
		sig:       make(chan struct{}),
	}

	p.maxSize.Store(maxSize)
	p.p = &sync.Pool{
		New: func() interface{} {
			return fn()
		},
	}

	// 预先生成最大数量30%的对象，提高性能，同时不过多消耗启动时间
	const scale = 0.3
	preloadSize := int(float64(maxSize) * scale)
	for i := 0; i < preloadSize; i++ {
		obj := p.p.Get()
		p.p.Put(obj)
		p.currentCount.Add(1)
	}

	return p, nil
}

func (p *WrapPool[T]) Get() (T, error) {
	var t T
	if p == nil {
		return t, ErrBufferClose
	}

	for {
		select {
		case <-p.sig:
			return t, ErrBufferClose
		default:
		}

		current := p.currentCount.Load()
		if current <= 0 {
			// 池中无可用对象
			break
		}

		if p.currentCount.CompareAndSwap(current, current-1) {
			t, ok := p.p.Get().(T)
			if !ok {
				p.currentCount.Add(1)
				return t, ErrPoolType
			}

			p.stats.totalGets.Add(1)
			return t, nil
		}
	}

	for {
		select {
		case <-p.sig:
			return t, ErrBufferClose
		default:
		}

		allocated := p.stats.allocations.Load()
		if allocated > int64(p.maxSize.Load()) {
			return t, ErrPoolMaxSize
		}

		// 二次验证
		if p.stats.allocations.Load() < int64(p.maxSize.Load()) {
			if p.stats.allocations.CompareAndSwap(allocated, allocated+1) {
				p.stats.totalGets.Add(1)
				return p.newFunc(), nil
			}
		}
	}
}

func (p *WrapPool[T]) Put(t T) {
	if p == nil {
		if p.closeFunc != nil {
			p.closeFunc(t)
		}

		return
	}

	if p.resetFunc != nil {
		t = p.resetFunc(t)
	}

	for {
		select {
		case <-p.sig:
			if p.closeFunc != nil {
				p.closeFunc(t)
			}
			return
		default:
		}

		current := p.currentCount.Load()
		if current >= p.maxSize.Load() {
			p.stats.allocations.Add(-1)
			p.stats.discards.Add(1)
			return
		}

		if p.currentCount.CompareAndSwap(current, current+1) {
			p.p.Put(t)
			return
		}
	}
}

func (p *WrapPool[T]) Stats() (allocations, reuses, discards int64) {
	t := p.stats.totalGets.Load()
	a := p.stats.allocations.Load()
	d := p.stats.discards.Load()
	return a, t - a, d
}

func (p *WrapPool[T]) Close() {
	close(p.sig)
	if p.closeFunc != nil {
		for {
			current := p.currentCount.Load()
			if current <= 0 {
				break
			}

			if p.currentCount.CompareAndSwap(current, current-1) {
				obj, ok := p.p.Get().(T)
				if !ok {
					continue
				}
				p.closeFunc(obj)
			}
		}
	}
	p.p = nil
}

func (p *WrapPool[T]) adjustMaxSize(maxSize int32) {
	oldSize := p.maxSize.Load()
	p.maxSize.CompareAndSwap(oldSize, maxSize)
	for {
		current := p.currentCount.Load()
		if current <= p.maxSize.Load() {
			return
		}

		if p.currentCount.CompareAndSwap(current, current-1) {
			obj, ok := p.p.Get().(T)
			if !ok {
				continue
			}
			p.closeFunc(obj)
		}
	}
}
