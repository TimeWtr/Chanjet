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
	"unsafe"

	"github.com/TimeWtr/Chanjet/_const"
	"github.com/TimeWtr/Chanjet/core/pools"
	"github.com/TimeWtr/Chanjet/errorx"
	"github.com/TimeWtr/Chanjet/metrics"
)

var unsafePointerPool = sync.Pool{
	New: func() interface{} {
		return make([]unsafe.Pointer, 1024)
	},
}

func init() {
	for i := 0; i < 5; i++ {
		obj, _ := unsafePointerPool.Get().([]unsafe.Pointer)
		unsafePointerPool.Put(obj)
	}
}

const (
	DefaultInterval = 5 * time.Second
	BatchCheckSize  = 32 // 写入数据时，多少批次进行一次切换检查，避免每次都检查
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

// RingBuffer 环形缓冲区设计
type RingBuffer struct {
	buf    []unsafe.Pointer // 存储数据指针
	size   uint32           // 缓冲区大小
	head   int32            // 写入位置
	tail   int32            // 读取的位置
	count  uint32           // 当前元素数量
	status uint32           // 当前缓冲区的状态
}

func newRingBuffer(size uint32) *RingBuffer {
	buf, _ := unsafePointerPool.Get().([]unsafe.Pointer)
	b := &RingBuffer{
		buf:  buf,
		size: size,
		head: -1,
		tail: -1,
	}

	atomic.StoreUint32(&b.status, _const.WritingStatus)
	return b
}

func (b *RingBuffer) Len() int {
	return int(atomic.LoadUint32(&b.count))
}

func (b *RingBuffer) push(p []byte) bool {
	if atomic.LoadUint32(&b.status) == _const.ClosedStatus {
		return false
	}

	for {
		currentCount := atomic.LoadUint32(&b.count)
		if currentCount >= b.size {
			// 缓冲区满了
			return false
		}

		if atomic.CompareAndSwapUint32(&b.count, currentCount, currentCount+1) {
			break
		}
	}

	ptr := pools.GetBytesPool(int64(len(p)))
	copy(*ptr, p)

	pos := atomic.AddInt32(&b.head, 1) % int32(b.size)
	if pos < 0 {
		pos += int32(b.size)
	}
	pos %= int32(b.size)

	atomic.StorePointer(&b.buf[pos], unsafe.Pointer(ptr))
	return true
}

func (b *RingBuffer) pop() ([]byte, bool) {
	if atomic.LoadUint32(&b.count) == 0 {
		return nil, false
	}

	pos := atomic.AddInt32(&b.tail, 1) % int32(b.size)
	dataPtr := atomic.SwapPointer(&b.buf[pos], nil)
	if dataPtr == nil {
		return nil, false
	}

	atomic.AddUint32(&b.count, ^uint32(0))
	return *(*[]byte)(dataPtr), true
}

func (b *RingBuffer) close() {
	atomic.StoreUint32(&b.status, _const.ClosedStatus)
}

// SwitchCondition 双缓冲通道执行切换的条件
type SwitchCondition struct {
	sizeThreshold    int64         // 通道写入大小阈值，这个阈值会触发通道切换
	percentThreshold int           // 写入数据的条数占据通道容量的比例，范围：0-100
	timeThreshold    int64         // 多久执行一次定时切换，无论是否达到percentThreshold和sizeThreshold
	interval         time.Duration // 定时切换的时间窗口
}

// Buffer 双缓冲通道设计
type Buffer struct {
	ringBufferCapacity  uint32                 // 环形缓冲区大小
	active              *RingBuffer            // 活跃缓冲区，实时接收数据写入
	passive             *RingBuffer            // 异步读取缓冲区，用于异步将数据传递给消费者
	swapPending         uint32                 // 是否交换缓冲区，切换的标识
	swapSignal          chan struct{}          // 切换信号
	readq               chan [][]byte          // 消费通道，将passive数据写入到readq，消费者消费readq
	size                uint32                 // 活跃缓冲区大小
	channelPool         sync.Pool              // 通道内存池
	minSizeBatchPool    sync.Pool              // 最小大小的内存池
	middleSizeBatchPool sync.Pool              // 中等大小的内存池
	maxSizeBatchPool    sync.Pool              // 最大大小的内存池
	taskQueue           chan *RingBuffer       // 任务队列，用于传输需要异步读取数据的channel
	remainingQueue      chan *RingBuffer       // 当禁止向taskQueue推送passive通道时，转存到remainingQueue,后续程序同步处理
	enableMetrics       bool                   // 是否开启指标采集
	mc                  metrics.BatchCollector // 指标数据采集器
	sc                  SwitchCondition        // 切换条件
	lastSwitchTime      int64                  // 上次切换的时间，毫秒时间戳
	wg                  sync.WaitGroup         // 同步控制
	status              uint32                 // 缓冲区状态
	closeSignal         chan struct{}          // 关闭缓冲区信号
}

func NewBuffer(capacity uint32, opts ...Options) (*Buffer, error) {
	const (
		bufferMultiplier = 4
		taskQueueSize    = 100
	)

	b := &Buffer{
		ringBufferCapacity: capacity,
		taskQueue:          make(chan *RingBuffer, taskQueueSize),
		readq:              make(chan [][]byte, bufferMultiplier*capacity),
		remainingQueue:     make(chan *RingBuffer, taskQueueSize),
		closeSignal:        make(chan struct{}),
		swapSignal:         make(chan struct{}, 1),
	}
	b.sc.interval = DefaultInterval
	for _, opt := range opts {
		if err := opt(b); err != nil {
			return nil, err
		}
	}

	b.initializePools()
	go b.orderProcessor()
	go b.swapMonitor()
	atomic.StoreUint32(&b.status, _const.WritingStatus)

	return b, nil
}

func (b *Buffer) Register() <-chan [][]byte {
	return b.readq
}

// initializePools 初始化多级Size缓存池，并进行多级别缓存预热
func (b *Buffer) initializePools() {
	b.initializeChannelPool()
	b.initializeMinBatchPool()
	b.initializeMiddleBatchPool()
	b.initializeMaxBatchPool()
}

// initializeChannelPool 初始化通道缓存池
func (b *Buffer) initializeChannelPool() {
	b.channelPool = sync.Pool{
		New: func() interface{} {
			return newRingBuffer(b.ringBufferCapacity)
		},
	}

	b.active, _ = b.channelPool.Get().(*RingBuffer)
	b.passive, _ = b.channelPool.Get().(*RingBuffer)
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

func (b *Buffer) swapMonitor() {
	ticker := time.NewTicker(b.sc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-b.closeSignal:
			return
		case <-ticker.C:
			if b.needSwitch() {
				atomic.StoreUint32(&b.swapPending, 1)
				select {
				case b.swapSignal <- struct{}{}:
				default:
				}
			}
		case <-b.swapSignal:
			b.performSwap()
		}
	}
}

// needSwitch 判断是否需要执行通道切换，切换条件有如下：
// 1. 综合因子超过阈值
// 2. 当前活跃缓冲区的大小超过缓冲区的容量
// 3. 当前时间与上次切换的时间间隔超过了规定的时间窗口
// 满足任意一条，即需要立即执行通道切换
func (b *Buffer) needSwitch() bool {
	const (
		timeWeight   = 0.4 // 时间窗口周期权重
		sizeWeight   = 0.6 // 缓冲区数据写入大小权重
		fullCapacity = 1.0 // 满载阈值
	)

	lastSwitch := atomic.LoadInt64(&b.lastSwitchTime)
	currentSize := atomic.LoadUint32(&b.size)
	// 计算时间因子(0-1)
	timeFactor := float64(time.Since(time.UnixMilli(lastSwitch))) / float64(b.sc.interval)
	// 计算大小因子(0-1)
	sizeFactor := float64(currentSize) / float64(b.ringBufferCapacity)
	combined := timeFactor*timeWeight + sizeFactor*sizeWeight

	return combined > fullCapacity ||
		currentSize >= b.ringBufferCapacity ||
		time.Since(time.UnixMilli(lastSwitch)) >= b.sc.interval
}

// performSwap 执行切换通道的逻辑
func (b *Buffer) performSwap() {
	defer func() {
		select {
		case <-b.swapSignal:
		default:
		}
	}()
	if !atomic.CompareAndSwapUint32(&b.swapPending, 1, 0) {
		return
	}

	oldActive := b.active
	oldPassive := b.passive
	newBuffer, _ := b.channelPool.Get().(*RingBuffer)
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&b.active)),
		unsafe.Pointer(oldPassive))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&b.passive)), unsafe.Pointer(newBuffer))
	atomic.StoreInt64(&b.lastSwitchTime, time.Now().UnixMilli())
	atomic.StoreUint32(&b.size, 0)

	if atomic.LoadUint32(&b.status) == _const.ClosedStatus {
		select {
		case b.remainingQueue <- oldActive:
		default:
		}
		return
	}

	select {
	case b.taskQueue <- oldActive:
	default:
	}
}

// Write 执行数据写入操作，写入失败时最多重试5次写入
func (b *Buffer) Write(p []byte) error {
	const maxReties = 5
	if atomic.LoadUint32(&b.status) == _const.ClosedStatus {
		return errorx.ErrBufferClose
	}

	pSize := int64(len(p))
	if b.active.push(p) {
		atomic.AddUint32(&b.size, uint32(pSize))
		b.mc.RecordWrite(pSize, nil)
		return nil
	}

	if atomic.CompareAndSwapUint32(&b.swapPending, 0, 1) {
		select {
		case b.swapSignal <- struct{}{}:
			b.performSwap()
		default:
		}
	}

	for i := 0; i < maxReties; i++ {
		if b.active.push(p) {
			atomic.AddUint32(&b.size, uint32(pSize))
			b.mc.RecordWrite(pSize, nil)
			return nil
		}

		time.Sleep(time.Millisecond * 10)
	}

	return errorx.ErrBufferFull
}

// orderProcessor 顺序消费通道数据
func (b *Buffer) orderProcessor() {
	var batchSize int64 = minBatchSize
	count := 0
	for {
		select {
		case <-b.closeSignal:
			return
		case buf, ok := <-b.taskQueue:
			if !ok {
				return
			}

			// 每5次计算一次batchSize值，平衡性能和准确度
			count++
			if count%5 == 0 {
				qLen := len(b.taskQueue)
				switch {
				case qLen < 20:
					batchSize = minBatchSize
				case qLen > 50:
					batchSize = middleBatchSize
				default:
					batchSize = maxBatchSize
				}
			}

			b.wg.Add(1)
			b.processBuffer(buf, batchSize)
		}
	}
}

// processBuffer 读取RingBuffer中缓冲数据，并批量发送给readq
func (b *Buffer) processBuffer(buf *RingBuffer, batchSize int64) {
	defer b.wg.Done()
	defer func() {
		buf.head, buf.tail = -1, -1
		b.channelPool.Put(buf)
	}()

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
	}

	var (
		count int64
		size  int64
	)
	flushBatch := func() {
		b.readq <- batch
		b.mc.RecordRead(count, size, nil)
		count, size = 0, 0
		batch = batch[:0]

		for _, bf := range batch {
			pools.PutBytesPool(bf)
		}
	}

	for {
		select {
		case <-b.closeSignal:
			if len(batch) > 0 {
				flushBatch()
			}
			return
		default:
		}

		data, ok := buf.pop()
		if !ok {
			if len(batch) > 0 {
				flushBatch()
			}
			return
		}

		batch = append(batch, data)
		count++
		size += int64(len(data))
		if count >= batchSize {
			flushBatch()
		}
	}
}

func (b *Buffer) drainQueue() {
	if len(b.taskQueue) > 0 {
		for {
			buf, ok := <-b.taskQueue
			if !ok {
				return
			}
			b.wg.Add(1)
			b.processBuffer(buf, 1)
		}
	}

	if len(b.remainingQueue) > 0 {
		for {
			buf, ok := <-b.remainingQueue
			if !ok {
				return
			}
			b.wg.Add(1)
			b.processBuffer(buf, 1)
		}
	}

	b.wg.Add(2)
	b.processBuffer(b.active, 1)
	b.processBuffer(b.passive, 1)
}

func (b *Buffer) Close() {
	status := atomic.LoadUint32(&b.status)
	if !atomic.CompareAndSwapUint32(&b.status, status, _const.ClosedStatus) {
		return
	}

	close(b.closeSignal)
	b.active.close()
	close(b.taskQueue)
	b.drainQueue()
	b.wg.Wait()
	close(b.readq)
}
