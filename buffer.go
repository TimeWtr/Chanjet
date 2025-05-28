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

	"github.com/TimeWtr/Chanjet/_const"
	"github.com/TimeWtr/Chanjet/metrics"
)

// SwitchCondition 双缓冲通道执行切换的条件
type SwitchCondition struct {
	sizeThreshold    int64         // 通道写入大小阈值，这个阈值会触发通道切换
	percentThreshold float64       // 写入数据的条数占据通道容量的比例，比如：80%
	timeThreshold    time.Duration // 多久执行一次定时切换，无论是否达到percentThreshold和sizeThreshold
}

// Buffer 缓冲区包含两个缓冲通道，active缓冲区为活跃缓冲区，实时接收数据
// passive缓冲区为备用缓冲区，当active缓冲区达到阈值/定时，进行缓冲通道的切换，passive缓冲区
// 切换为活跃缓冲区，开始实时接收数据，原来的active缓冲区切换为异步刷盘缓冲区，异步从缓冲区中读取
// 数据给到Writer写入器写入文件。循环往复，不断切换缓冲区。
// 缓冲区切换的条件：
// 1. 缓冲区的达到指定的大小限制(100M)
// 2. 缓冲区的条数即长度达到容量的80%
// 3. 每隔固定时间执行定时切换(5秒)，防止长期没有数据，导致缓冲区中的没有办法写入
type Buffer struct {
	active         chan []byte            // 活跃缓冲区
	passive        chan []byte            // 异步刷盘缓冲区
	readq          chan []byte            // 异步读取通道
	sig            chan struct{}          // 关闭缓冲区的信号
	once           sync.Once              // 单例
	size           atomic.Int64           // 活跃缓冲区写入的字节大小
	swapLock       atomic.Uint32          // 原子保护
	pool           sync.Pool              // 缓冲池
	wg             sync.WaitGroup         // 同步
	enableMetrics  bool                   // 是否开启指标采集
	mc             metrics.BatchCollector // 指标数据采集器
	sc             SwitchCondition        // 通道的切换条件
	lastSwitchTime atomic.Int64           // 上次切换的时间，毫秒时间戳
}

// NewBuffer 双缓冲通道设计，capacity为单个缓冲通道的容量，maxSize为对象池中
// 允许创建的最大对象数量
func NewBuffer(capacity int64, opts ...Options) (*Buffer, error) {
	const bufferMultiplier = 3
	b := &Buffer{
		sig:   make(chan struct{}),
		readq: make(chan []byte, capacity*bufferMultiplier),
		wg:    sync.WaitGroup{},
		sc: SwitchCondition{
			sizeThreshold:    _const.DefaultSizeThreshold,
			percentThreshold: _const.DefaultPercentThreshold,
			timeThreshold:    _const.DefaultTimeThreshold,
		},
		mc: metrics.NewBatchCollector(metrics.NewPrometheus()),
	}

	for _, opt := range opts {
		if err := opt(b); err != nil {
			return b, err
		}
	}

	b.mc.Start()
	b.pool = sync.Pool{
		New: func() interface{} {
			b.mc.RecordPoolAlloc()
			return make(chan []byte, capacity)
		},
	}
	b.active, _ = b.pool.Get().(chan []byte)
	b.passive, _ = b.pool.Get().(chan []byte)
	b.lastSwitchTime.Store(time.Now().UnixMilli())

	go b.asyncWork()

	return b, nil
}

func (b *Buffer) Write(p []byte) error {
	select {
	case <-b.sig:
		return ErrBufferClose
	default:
	}

	pSize := len(p)
	if b.size.Load()+int64(pSize) > b.sc.sizeThreshold ||
		float64(len(b.active)) >= float64(cap(b.active))*b.sc.percentThreshold {
		// 执行切换逻辑
		b.sw()
	}

	select {
	case <-b.sig:
		b.mc.RecordWrite(int64(pSize), ErrBufferClose)
		return ErrBufferClose
	case b.active <- p:
		b.size.Add(int64(pSize))
		b.mc.RecordWrite(int64(pSize), nil)
		return nil
	default:
		b.mc.RecordWrite(int64(pSize), ErrBufferFull)
		return ErrBufferFull
	}
}

func (b *Buffer) Register() <-chan []byte {
	return b.readq
}

// sw 执行切换逻辑
func (b *Buffer) sw() {
	start := time.Now()
	defer func() {
		b.mc.RecordSwitch(_const.SwitchSuccess, int64(time.Since(start).Seconds()))
		b.mc.RecordWrite(0, nil)
	}()

	if !b.swapLock.CompareAndSwap(0, 1) {
		return
	}
	defer b.swapLock.Store(0)

	active := b.active
	b.wg.Add(1)
	b.mc.ObserveAsyncWorker(_const.MetricsIncOp)
	go b.asyncReader(active)

	for {
		select {
		case <-b.sig:
			return
		default:
			newBuf, _ := b.pool.Get().(chan []byte)
			b.active, b.passive = b.passive, newBuf
			b.size.Store(0)
			return
		}
	}
}

func (b *Buffer) asyncWork() {
	ticker := time.NewTicker(b.sc.timeThreshold)
	defer ticker.Stop()

	for range ticker.C {
		select {
		case <-b.sig:
			return
		default:
			// 当本地轮转的时间与上次轮转的时间间隔小于设置时间间隔的三分之一，且通道内的容量和大小都未达到
			// 限制阈值，则跳过本次通道切换，防止通道切换过于频繁。
			if time.Now().UnixMilli()-b.lastSwitchTime.Load() < int64(b.sc.timeThreshold)/3 &&
				b.size.Load() < b.sc.sizeThreshold &&
				float64(len(b.active)) < float64(cap(b.active))*b.sc.percentThreshold {
				b.mc.RecordSwitch(_const.SwitchSkip, 0)
				continue
			}

			b.sw()
		}
	}
}

// asyncReader 异步读取器，后台异步的把缓冲通道中的数据读取出来，并写入到readq中
func (b *Buffer) asyncReader(ch chan []byte) {
	defer b.wg.Done()
	defer b.mc.ObserveAsyncWorker(_const.MetricsDecOp)

	// 读取缓冲区中所有的数据，直到为空退出
	for len(ch) > 0 {
		select {
		case <-b.sig:
			return
		default:
			data := <-ch
			b.readq <- data
			b.mc.RecordRead(int64(len(data)), nil)
		}
	}
}

func (b *Buffer) Close() {
	b.once.Do(func() {
		close(b.sig)
		// TODO 解决强制刷新的问题
		b.mc.Stop()
		b.wg.Wait()

		close(b.active)
		close(b.passive)
		for len(b.active) > 0 {
			data := <-b.active
			b.readq <- data
		}

		close(b.readq)
	})
}
