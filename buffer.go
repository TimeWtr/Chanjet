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
	"sync"
	"sync/atomic"
	"time"
)

const (
	// SizeThreshold 缓冲区的切换大小阈值
	SizeThreshold = 1024 * 1024 * 10
	// PercentThreshold 缓冲区切换的比例阈值
	PercentThreshold = 0.8
	// TimeThreshold 缓冲区切换的时间阈值
	TimeThreshold = 1 * time.Second
)

// Buffer 缓冲区包含两个缓冲通道，active缓冲区为活跃缓冲区，实时接收日志数据
// passive缓冲区为备用缓冲区，当active缓冲区达到阈值/定时，进行缓冲通道的切换，passive缓冲区
// 切换为活跃缓冲区，开始实时接收日志数据，原来的active缓冲区切换为异步刷盘缓冲区，异步从缓冲区中读取
// 日志数据给到Writer写入器写入日志文件。循环往复，不断切换缓冲区。
// 缓冲区切换的条件：
// 1. 缓冲区的日志达到指定的大小限制(10M)
// 2. 缓冲区日志的条数即长度达到容量的80%
// 3. 每隔固定时间执行定时切换(500毫秒)，防止长期没有日志数据，导致缓冲区中的日志没有办法写入
type Buffer struct {
	// 活跃缓冲区
	active chan []byte
	// 异步刷盘缓冲区
	passive chan []byte
	// 异步读取通道
	readq chan []byte
	// 关闭缓冲区的信号
	sig chan struct{}
	// 单例
	once sync.Once
	// 活跃缓冲区写入的字节大小
	size uint64
	// 加锁保护
	lock sync.Mutex
	// 异步刷盘的goroutine数量
	counter atomic.Int32
	// 对象池
	pool *WrapPool[chan []byte]
}

// NewBuffer 双缓冲通道设计，capacity为单个缓冲通道的容量，maxSize为对象池中
// 允许创建的最大对象数量
func NewBuffer(capacity int64, maxSize int) (*Buffer, error) {
	pool, err := NewWrapPool[chan []byte](func() chan []byte {
		return make(chan []byte, capacity)
	}, func(ch chan []byte) chan []byte {
		for {
			select {
			case <-ch:
			default:
				return ch
			}
		}
	}, func(ch chan []byte) {
		close(ch)
	}, int32(maxSize))
	if err != nil {
		return nil, err
	}

	active, err := pool.Get()
	if err != nil {
		return nil, err
	}
	passive, err := pool.Get()
	if err != nil {
		return nil, err
	}

	const bufferMultiplier = 2
	b := &Buffer{
		active:  active,
		passive: passive,
		sig:     make(chan struct{}),
		readq:   make(chan []byte, capacity*bufferMultiplier),
		lock:    sync.Mutex{},
		pool:    pool,
	}
	b.counter.Store(0)

	go b.asyncWork()

	return b, nil
}

func (b *Buffer) Write(p []byte) error {
	select {
	case <-b.sig:
		return ErrBufferClose
	default:
	}

	b.lock.Lock()
	defer b.lock.Unlock()
	pSize := len(p)
	if b.size+uint64(pSize) > SizeThreshold || float64(len(b.active)) >= float64(cap(b.active))*PercentThreshold {
		// 执行切换逻辑
		b.sw()
	}

	select {
	case <-b.sig:
		return ErrBufferClose
	case b.active <- p:
		b.size += uint64(pSize)
		return nil
	default:
		return ErrBufferFull
	}
}

func (b *Buffer) Register() <-chan []byte {
	b.lock.Lock()
	defer b.lock.Unlock()

	return b.readq
}

// sw 执行切换逻辑
func (b *Buffer) sw() {
	active := b.active
	b.counter.Add(1)
	go b.asyncReader(active)

	for {
		select {
		case <-b.sig:
			return
		default:
			newBuf, err := b.pool.Get()
			if err != nil {
				return
			}
			b.active, b.passive = b.passive, newBuf
			b.size = 0
			return
		}
	}
}

func (b *Buffer) asyncWork() {
	ticker := time.NewTicker(TimeThreshold)
	defer ticker.Stop()

	for range ticker.C {
		select {
		case <-b.sig:
			return
		default:
			b.lock.Lock()
			b.sw()
			b.lock.Unlock()
		}
	}
}

// asyncReader 异步读取器，后台异步的把缓冲通道中的日志数据读取出来，并写入到readq中
func (b *Buffer) asyncReader(ch chan []byte) {
	defer func() {
		b.counter.Add(-1)
		b.pool.Put(ch)
	}()

	// 读取缓冲区中所有的数据，直到为空退出
	for len(ch) > 0 {
		select {
		case <-b.sig:
			return
		case data := <-ch:
			select {
			case <-b.sig:
				return
			case b.readq <- data:
			}
		}
	}
}

func (b *Buffer) Close() {
	b.once.Do(func() {
		close(b.sig)
		close(b.active)
		close(b.passive)

		for len(b.active) > 0 {
			select {
			case data := <-b.active:
				b.readq <- data
			default:
				break
			}
		}

		const sleepInterval = time.Millisecond * 5
		for b.counter.Load() > 0 {
			time.Sleep(sleepInterval)
		}
		close(b.readq)
		b.pool.Close()
	})
}
