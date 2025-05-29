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

const (
	capacity       = 4096 // 双通道的容量
	BatchCheckSize = 32   // 写入数据时，多少批次进行一次切换检查，避免每次都检查
)

const (
	minBatchSize    = 32
	middleBatchSize = 128
	maxBatchSize    = 256
)

const (
	minBatchSizePreloads    = 20
	middleBatchSizePreloads = 10
	maxBatchSizePreloads    = 5
)

// SwitchCondition 双缓冲通道执行切换的条件
type SwitchCondition struct {
	sizeThreshold    int64         // 通道写入大小阈值，这个阈值会触发通道切换
	percentThreshold int           // 写入数据的条数占据通道容量的比例，范围：0-100
	timeThreshold    int64         // 多久执行一次定时切换，无论是否达到percentThreshold和sizeThreshold
	interval         time.Duration // 定时切换的时间窗口
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
	capacity            int64                  // 双缓冲通道的容量
	active              chan []byte            // 活跃缓冲区
	passive             chan []byte            // 异步刷盘缓冲区
	readq               chan [][]byte          // 异步批量读取通道
	sig                 chan struct{}          // 关闭缓冲区的信号
	size                atomic.Int64           // 活跃缓冲区写入的字节大小
	swapLock            atomic.Uint32          // 原子保护
	channelPool         sync.Pool              // 通道缓冲池
	minSizeBatchPool    sync.Pool              // 最小大小的内存池
	middleSizeBatchPool sync.Pool              // 中等大小的内存池
	maxSizeBatchPool    sync.Pool              // 最大大小的内存池
	taskQueue           chan chan []byte       // 任务队列，用于传输需要异步读取数据的channel
	processing          chan struct{}          // 处理任务队列channel的消息通知
	remainingQueue      chan chan []byte       // 当禁止向taskQueue推送passive通道时，转存到remainingQueue,后续程序同步处理
	writeCount          atomic.Int64           // 当前缓冲区写入的数据数量
	enableMetrics       bool                   // 是否开启指标采集
	mc                  metrics.BatchCollector // 指标数据采集器
	sc                  SwitchCondition        // 通道的切换条件
	lastSwitchTime      atomic.Int64           // 上次切换的时间，毫秒时间戳
	wg                  sync.WaitGroup         // 同步控制
	status              atomic.Int64           // 缓冲区状态
}

// NewBuffer 双缓冲通道设计，capacity为单个缓冲通道的容量，maxSize为对象池中
// 允许创建的最大对象数量
func NewBuffer(opts ...Options) (*Buffer, error) {
	const (
		bufferMultiplier = 4
		taskQueueSize    = 50
	)
	b := &Buffer{
		capacity:       capacity,
		sig:            make(chan struct{}),
		readq:          make(chan [][]byte, capacity*bufferMultiplier),
		taskQueue:      make(chan chan []byte, taskQueueSize),
		remainingQueue: make(chan chan []byte, taskQueueSize),
		processing:     make(chan struct{}, 1),
		sc: SwitchCondition{
			sizeThreshold:    _const.DefaultSizeThreshold,
			percentThreshold: _const.DefaultPercentThreshold,
			timeThreshold:    _const.DefaultTimeThreshold.Milliseconds(),
			interval:         _const.DefaultTimeThreshold,
		},
		mc: metrics.NewBatchCollector(metrics.NewPrometheus()),
	}

	for _, opt := range opts {
		if err := opt(b); err != nil {
			return b, err
		}
	}

	b.initializePools()
	b.mc.Start()
	b.channelPool = sync.Pool{
		New: func() interface{} {
			b.mc.RecordPoolAlloc()
			return make(chan []byte, capacity)
		},
	}

	b.active, _ = b.channelPool.Get().(chan []byte)
	b.passive, _ = b.channelPool.Get().(chan []byte)
	b.lastSwitchTime.Store(time.Now().UnixMilli())
	b.status.Store(_const.WritingStatus)

	go b.orderProcessor()
	go b.asyncWork()

	return b, nil
}

// initializePools 初始化多级Size缓存池，并进行多级别缓存预热
func (b *Buffer) initializePools() {
	b.initializeMinBatchPool()
	b.initializeMiddleBatchPool()
	b.initializeMaxBatchPool()
}

// initializeMinBatchPool 初始化最小Size缓存池并预热20个可用对象
func (b *Buffer) initializeMinBatchPool() {
	b.minSizeBatchPool = sync.Pool{
		New: func() interface{} {
			b.mc.RecordPoolAlloc()
			return make([][]byte, 0, minBatchSize)
		},
	}

	for i := 0; i < minBatchSizePreloads; i++ {
		obj := b.minSizeBatchPool.Get()
		b.minSizeBatchPool.Put(obj)
	}
}

// initializeMiddleBatchPool 初始化中等Size缓存池并预热10个可用对象
func (b *Buffer) initializeMiddleBatchPool() {
	b.middleSizeBatchPool = sync.Pool{
		New: func() interface{} {
			b.mc.RecordPoolAlloc()
			return make([][]byte, 0, middleBatchSize)
		},
	}

	for i := 0; i < middleBatchSizePreloads; i++ {
		obj := b.middleSizeBatchPool.Get()
		b.middleSizeBatchPool.Put(obj)
	}
}

// initializeMaxBatchPool 初始化最大Size缓存池并预热5个可用对象
func (b *Buffer) initializeMaxBatchPool() {
	b.maxSizeBatchPool = sync.Pool{
		New: func() interface{} {
			b.mc.RecordPoolAlloc()
			return make([][]byte, 0, maxBatchSize)
		},
	}

	for i := 0; i < maxBatchSizePreloads; i++ {
		obj := b.maxSizeBatchPool.Get()
		b.maxSizeBatchPool.Put(obj)
	}
}

// needSwitch 是否需要执行切换逻辑，每32次执行一次切换逻辑。
// 条件：
// 1.不满足32批次检查，直接跳过
// 2 写入活跃缓冲区的数据条数超过了容量一定的比例
// 3. 写入活跃缓冲区的数据总大小超过了总大小的阈值
func (b *Buffer) needSwitch(pSize int64) bool {
	if b.writeCount.Add(1)%BatchCheckSize != 0 {
		return false
	}

	l := int64(len(b.active))
	if l >= (b.capacity*int64(b.sc.percentThreshold))/100 {
		return true
	}

	return b.size.Load()+pSize >= b.sc.sizeThreshold
}

// needTickerSwitch 是否需要执行切换逻辑
// 条件：
// 1.距离上次切换时间间隔大于切换窗口周期的1/2
// 2 写入活跃缓冲区的数据条数超过了容量一定的比例
// 3. 写入活跃缓冲区的数据总大小超过了总大小的阈值
// 需要同时满足1、2或1、3才可以切换通道
func (b *Buffer) needTickerSwitch() bool {
	t := time.Now().UnixMilli()
	if t-b.lastSwitchTime.Load() < b.sc.timeThreshold/2 {
		return false
	}

	l := int64(len(b.active))
	if l >= (b.capacity*int64(b.sc.percentThreshold))/100 {
		return true
	}

	return b.size.Load() >= b.sc.sizeThreshold
}

func (b *Buffer) Write(p []byte) error {
	if b.status.Load() == _const.ClosedStatus {
		return ErrBufferClose
	}

	pSize := len(p)
	if b.needSwitch(int64(pSize)) {
		select {
		case <-b.sig:
			return ErrBufferClose
		case b.processing <- struct{}{}:
			b.sw()
		}
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
		return b.handleBufferFull(p)
	}
}

func (b *Buffer) Register() <-chan [][]byte {
	return b.readq
}

// handleBufferFull 处理写入时活跃缓冲区需要切换的问题
func (b *Buffer) handleBufferFull(p []byte) error {
	select {
	case <-b.sig:
		return ErrBufferClose
	case b.processing <- struct{}{}:
		b.sw()
	default:
	}

	if b.status.Load() == _const.ClosedStatus {
		return ErrBufferClose
	}

	select {
	case b.active <- p:
		size := int64(len(p))
		b.size.Add(size)
		b.mc.RecordWrite(size, nil)
		return nil
	default:
		return ErrBufferFull
	}
}

// sw 执行切换逻辑
func (b *Buffer) sw() {
	defer func() { <-b.processing }()
	if !b.swapLock.CompareAndSwap(0, 1) {
		return
	}
	defer b.swapLock.Store(0)

	start := time.Now()
	defer func() {
		b.mc.RecordSwitch(_const.SwitchSuccess, int64(time.Since(start).Seconds()))
		b.mc.RecordWrite(0, nil)
	}()

	active := b.active
	b.mc.ObserveAsyncWorker(_const.MetricsIncOp)

	newBuf, _ := b.channelPool.Get().(chan []byte)
	b.active, b.passive = b.passive, newBuf
	b.size.Store(0)
	b.writeCount.Store(0)

	select {
	case <-b.sig:
		select {
		case b.remainingQueue <- active:
			// 收到缓冲区关闭的信号后，taskQueue禁止写入，转存到暂存队列等待处理
		default:
		}
	case b.taskQueue <- active:
		// 优先发送channel到队列中，即使关闭了，数据也会正常被处理
	}
}

// orderProcessor 顺序处理器，顺序的从taskQueue中来获取需要处理的channel
func (b *Buffer) orderProcessor() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	batchSize := minBatchSize
	for {
		if b.status.Load() == _const.ClosedStatus {
			return
		}

		select {
		case ch, ok := <-b.taskQueue:
			if !ok {
				return
			}

			select {
			case <-ticker.C:
				qLen := len(b.taskQueue)
				switch {
				case qLen < 20:
					batchSize = minBatchSize
				case qLen < 50:
					batchSize = middleBatchSize
				default:
					batchSize = maxBatchSize
				}
			default:
			}

			b.wg.Add(1)
			b.processChannel(ch, batchSize)
			b.channelPool.Put(ch)
		}
	}
}

// processChannel 批量从channel中读取出数据，写入到readq
func (b *Buffer) processChannel(ch chan []byte, batchSize int) {
	defer b.wg.Done()

	var batch [][]byte
	switch batchSize {
	case minBatchSize:
		batch = b.minSizeBatchPool.Get().([][]byte)
		defer b.minSizeBatchPool.Put(batch)
	case middleBatchSize:
		batch = b.middleSizeBatchPool.Get().([][]byte)
		defer b.middleSizeBatchPool.Put(batch)
	case maxBatchSize:
		batch = b.maxSizeBatchPool.Get().([][]byte)
		defer b.maxSizeBatchPool.Put(batch)
	default:
		batch = b.minSizeBatchPool.Get().([][]byte)
		defer b.minSizeBatchPool.Put(batch)
	}

	count, size := 0, 0
	flushBatch := func() {
		if b.status.Load() == _const.ClosedStatus {
			return
		}

		b.readq <- batch[:count]
		b.mc.RecordRead(int64(count), int64(size), nil)
		count, size = 0, 0
		batch = batch[:0]
	}

	for len(ch) > 0 {
		select {
		case data, ok := <-ch:
			if !ok {
				flushBatch()
				return
			}
			batch = append(batch, data)
			size += len(data)
			count++
			if count == batchSize {
				flushBatch()
				continue
			}
		default:
			if len(ch) == 0 {
				flushBatch()
				return
			}
		}
	}

	if len(batch) > 0 {
		flushBatch()
	}
}

// drainQueue 处理关闭buffer后的通道数据处理问题
func (b *Buffer) drainQueue() {
	for ch := range b.taskQueue {
		b.wg.Add(1)
		b.processChannel(ch, 1)
		b.channelPool.Put(ch)
	}

	for ch := range b.remainingQueue {
		b.wg.Add(1)
		b.processChannel(ch, 1)
		b.channelPool.Put(ch)
	}

	b.wg.Add(2)
	b.processChannel(b.active, 1)
	b.processChannel(b.passive, 1)
}

func (b *Buffer) asyncWork() {
	ticker := time.NewTicker(b.sc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-b.sig:
			return
		case <-ticker.C:
			if b.needTickerSwitch() {
				b.mc.RecordSwitch(_const.SwitchSkip, 0)
				continue
			}

			select {
			case b.processing <- struct{}{}:
				b.sw()
			case <-b.sig:
				return
			}
		}
	}
}

func (b *Buffer) Close() {
	if !b.status.CompareAndSwap(_const.WritingStatus, _const.ClosedStatus) {
		return
	}

	close(b.sig)
	close(b.active)
	close(b.passive)
	close(b.remainingQueue)
	close(b.taskQueue)
	b.drainQueue()

	b.wg.Wait()
	// TODO 解决指标数据强制刷新的问题
	close(b.readq)
	b.mc.Stop()
}
