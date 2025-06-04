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
	"log"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/TimeWtr/Chanjet"
	"github.com/TimeWtr/Chanjet/config"
	"github.com/TimeWtr/Chanjet/errorx"
	"github.com/TimeWtr/Chanjet/metrics"
	"github.com/TimeWtr/Chanjet/pools"
)

const (
	SmallDataThreshold      = 1024            // 小数据阈值（<1KB）
	LargeDataThreshold      = 32 * 1024       // 大数据阈值（>32KB）
	MediumDataCacheDuration = 5 * time.Second // 中型数据的缓存时间
	SwitchCheckInterval     = 5 * time.Millisecond
)

type SmartBuffer struct {
	buf    []unsafe.Pointer        // 存储[]byte对应的指针
	head   int32                   // 写指针
	tail   int32                   // 读指针
	count  int32                   // 当前写入的数据条数
	size   int32                   // 智能缓冲区的容量设置
	status int32                   // 智能缓冲区的状态
	pm     *pools.LifeCycleManager // 缓冲池生命周期管理器
}

func newSmartBuffer(size int32) *SmartBuffer {
	s := &SmartBuffer{
		buf:    make([]unsafe.Pointer, size),
		head:   -1,
		tail:   -1,
		count:  0,
		size:   size,
		status: Chanjet.WritingStatus,
		pm:     pools.NewLifeCycleManager(),
	}

	go s.recycleWorker()
	return s
}

func (s *SmartBuffer) len() int {
	return int(atomic.LoadInt32(&s.count))
}

func (s *SmartBuffer) write(p []byte) bool {
	if s.status == Chanjet.ClosedStatus {
		return false
	}

	l := len(p)
	ptr := unsafe.Pointer(&p[0])
	switch {
	case l < SmallDataThreshold:
		buf, _ := s.pm.SmallPool.Get().([]byte)
		buf = buf[:l]
		copy(buf, p)
		return s.push(unsafe.Pointer(&buf[0]), int32(l))
	case l < LargeDataThreshold:
		s.pm.BigDataPool.Put(uintptr(ptr), p)
		return s.push(ptr, int32(l))
	default:
		s.pm.MediumPool.Put(uintptr(ptr), time.Now())
		return s.push(ptr, int32(l))
	}
}

// zeroCopyRead is a non-safe API. When using this API, you must ensure that the data is not modified
// after Write. Otherwise, the zero-copy data will be wrong.
func (s *SmartBuffer) zeroCopyRead() ([]byte, bool) {
	ptr, size := s.pop()
	if ptr == nil {
		return nil, false
	}

	if size > LargeDataThreshold {
		ptrVal := uintptr(ptr)
		s.pm.BigDataPool.Release(ptrVal)
	}

	return *(*[]byte)(ptr), true
}

// safeRead 读取数据，安全API，默认返回副本
func (s *SmartBuffer) safeRead() ([]byte, bool) {
	ptr, l := s.pop()

	switch {
	case l < SmallDataThreshold:
		buf, _ := s.pm.SmallPool.Get().([]byte)
		buf = buf[:l]
		copy(buf, *(*[]byte)(ptr))
		return buf, true
	case l > LargeDataThreshold:
		// 大数据返回副本（安全默认）
		data := make([]byte, l)
		copy(data, *(*[]byte)(ptr))
		s.recycle(ptr, l)
		return data, true
	default:
		if s.pm.MediumPool.IsValid(uintptr(ptr)) {
			// 有效期内的数据（零拷贝返回）
			return *(*[]byte)(ptr), true
		}

		// 缓存失效，返回副本（安全默认）
		data := make([]byte, l)
		copy(data, *(*[]byte)(ptr))
		return data, true
	}
}

// Release 释放零拷贝API读取到的数据
func (s *SmartBuffer) Release(data []byte) {
	if len(data) < LargeDataThreshold {
		return
	}

	ptrVal := uintptr(unsafe.Pointer(&data[0]))
	s.pm.BigDataPool.Release(ptrVal)
}

// push 实际执行数据的写入
func (s *SmartBuffer) push(ptr unsafe.Pointer, size int32) bool {
	if s.status == Chanjet.ClosedStatus {
		return false
	}

	for {
		currentCount := atomic.LoadInt32(&s.count)
		if currentCount >= size {
			return false
		}

		if atomic.CompareAndSwapInt32(&s.count, currentCount, currentCount+1) {
			break
		}
	}

	atomic.AddInt32(&s.size, size)
	pos := atomic.AddInt32(&s.head, 1) % s.size
	if pos < 0 {
		pos += s.size
	}
	pos %= s.size
	atomic.StorePointer(&s.buf[pos], ptr)

	return true
}

// pop 执行数据的获取，返回指针
func (s *SmartBuffer) pop() (unsafe.Pointer, int32) {
	if atomic.LoadInt32(&s.count) == 0 {
		return nil, 0
	}

	pos := atomic.AddInt32(&s.tail, 1) % s.size
	ptr := atomic.SwapPointer(&s.buf[pos], nil)
	if ptr == nil {
		return nil, 0
	}

	atomic.AddInt32(&s.count, ^int32(0))
	return ptr, s.getSizeFromPtr(ptr)
}

func (s *SmartBuffer) recycleWorker() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		if atomic.LoadInt32(&s.status) == Chanjet.ClosedStatus {
			return
		}

		select {
		case <-ticker.C:
			s.pm.Cleanup()
		default:
		}
	}
}

// recycle 释放资源，三种大小方式不一样。
// 1. SmallData：获取ptr对应的[]byte，重置并放回到缓冲池中
// 2. MediumData：将ptr与time的映射关系删除
// 3. LargeData：将ptr在pool中的引用计数-1
func (s *SmartBuffer) recycle(ptr unsafe.Pointer, size int32) {
	ptrVal := uintptr(ptr)
	switch {
	case size < SmallDataThreshold:
		data := *(*[]byte)(ptr)
		data = data[:0]
		s.pm.SmallPool.Put(data)
	case size > LargeDataThreshold:
		s.pm.BigDataPool.Release(ptrVal)
	default:
		s.pm.MediumPool.Release(ptrVal)
	}
}

// getSizeFromPtr 获取指针所指向值切片的长度
func (s *SmartBuffer) getSizeFromPtr(ptr unsafe.Pointer) int32 {
	return int32(len(*(*[]byte)(ptr)))
}

func (s *SmartBuffer) Close() {
	atomic.StoreInt32(&s.status, Chanjet.ClosedStatus)
	for s.count > 0 {
		ptr, _ := s.pop()
		if ptr == nil {
			continue
		}

		s.recycle(ptr, s.getSizeFromPtr(ptr))
	}
}

// DoubleBuffer Double buffer design
type DoubleBuffer struct {
	// The active buffer is used to receive written data in real time. When the channel switching
	// condition is met, it will switch to the asynchronous processing buffer.
	active *SmartBuffer
	// The asynchronous read buffer is used to process data asynchronously. The buffer is placed
	// in the blocking minimum heap for sorting and waiting for asynchronous goroutine processing.
	// When the channel is switched, the active buffer is switched to the asynchronous read buffer.
	// The asynchronous read buffer is assigned a new buffer and switched to the active buffer to
	// receive write data in real time.
	passive *SmartBuffer
	// Turn off buffer signal
	stop chan struct{}
	// Buffer capacity
	size int32
	// The number of data entries written to the current active buffer
	count int32
	// Buffer status
	status int32
	// The time of last switch, in milliseconds
	lastSwitch int64
	// The time window for background rotation
	interval time.Duration
	// Blocking notifications for switching
	swapSignal chan struct{}
	// Switch pending state
	swapPending int32
	// Waiting for read Condition
	directCond *sync.Cond
	//Direct read lock
	directMutex sync.RWMutex
	// buffer object pool
	pool sync.Pool
	// Buffer for safe batch reads
	readq chan [][]byte
	// Synchronous control
	wg sync.WaitGroup
	// A globally monotonically increasing unique sequence number used to perform sequential operations
	// on passive
	sequence int64
	// The passive sequence number currently being processed by the asynchronous program, concurrent
	// security updates
	currentSequence int64
	// The minimum heap is used to sort multiple passives to be processed. When switching channels,
	// the active channel will be converted to a passive channel.
	// Put it into the taskQueue for asynchronous processing. Because the taskQueue has a capacity limit,
	// if the taskQueue is full, this passive needs to block and wait, which will affect subsequent channel
	// switching and data writing. To solve this problem, use sequence+the minimum heap finds the passive
	// corresponding to the smallest sequence. The passive that needs to be switched is directly written to
	// the minimum heap for sorting. Each time it gets the passive that needs to be processed first, there
	// is no need to block and wait.
	pendingHeap *MinHeap
	// Used to notify other goroutines that there are new passives to be processed.
	heapCond *sync.Cond
	// Lock used to protect passive writes to the minimum heap when switching channels.
	heapMutex sync.Mutex
	// Used to determine whether to enable indicator monitoring,
	enableMetrics bool
	// Batch indicator collector for receiving indicator data from double-buffered channels in real time.
	mc metrics.BatchCollector
	// The condition to switch channels.
	sc *config.SwitchCondition
}

func NewDoubleBuffer(size int32, sc *config.SwitchCondition) *DoubleBuffer {
	d := &DoubleBuffer{
		active:      newSmartBuffer(size),
		passive:     newSmartBuffer(size),
		readq:       make(chan [][]byte, 3*size),
		stop:        make(chan struct{}),
		size:        size,
		sc:          sc,
		lastSwitch:  time.Now().UnixMilli(),
		pendingHeap: &MinHeap{},
	}

	d.pool = sync.Pool{
		New: func() interface{} {
			return newSmartBuffer(size)
		},
	}

	d.active, _ = d.pool.Get().(*SmartBuffer)
	d.passive, _ = d.pool.Get().(*SmartBuffer)
	d.directCond = sync.NewCond(&d.directMutex)
	// initialize min heap
	heap.Init(&MinHeap{})
	d.heapCond = sync.NewCond(&d.heapMutex)

	d.wg.Add(1)
	go d.processor()

	return d
}

func (d *DoubleBuffer) Write(p []byte) error {
	if atomic.LoadInt32(&d.status) == Chanjet.ClosedStatus {
		return errorx.ErrBufferClose
	}

	if d.needSwitch() {
		atomic.CompareAndSwapInt32(&d.swapPending, 0, 1)
		select {
		case d.swapSignal <- struct{}{}:
			d.switchChannel()
		default:
		}
	}

	// Try to write to buffer
	if d.active.write(p) {
		if atomic.LoadInt32(&d.passive.count) == 0 {
			d.directCond.Broadcast()
		}
	}
	// TODO 处理写入
	return nil
}

// needSwitch determines whether a channel switch needs to be executed. The switching conditions
// are as follows:
// 1. The comprehensive factor exceeds the threshold
// 2. The size of the current active buffer exceeds the buffer capacity
// 3. The time interval between the current time and the last switch exceeds the specified time window
// If any of the conditions is met, the channel switch needs to be executed immediately.
func (d *DoubleBuffer) needSwitch() bool {
	currentCount := atomic.LoadInt32(&d.count)
	if currentCount >= d.size {
		return true
	}

	lastSwitch := time.UnixMilli(atomic.LoadInt64(&d.lastSwitch))
	elapsed := time.Since(lastSwitch)
	if elapsed >= d.interval {
		return true
	}
	// Calculate capacity factor (0-1)
	countFactor := float64(currentCount) / float64(d.size)
	// Calculate switch time factor (0-1)
	switchFactor := float64(elapsed) / float64(d.interval)
	combined := Chanjet.TimeWeight*switchFactor + Chanjet.SizeWeight*countFactor

	return combined > Chanjet.FullCapacity
}

// switchChannel Perform channel switching
func (d *DoubleBuffer) switchChannel() {
	defer func() {
		<-d.swapSignal
	}()

	if !atomic.CompareAndSwapInt32(&d.swapPending, 1, 0) {
		return
	}

	oldActive := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&d.active)))
	oldPassive := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&d.passive)))
	newBuf, _ := d.pool.Get().(*SmartBuffer)

	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&d.active)), oldPassive)
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&d.passive)),
		unsafe.Pointer(newBuf))

	d.directCond.Broadcast()
	oldActiveVal := (*SmartBuffer)(oldActive)
	seq := atomic.AddInt64(&d.sequence, 1)
	item := MinHeapItem{
		sequence: seq,
		buf:      oldActiveVal,
	}

	d.heapMutex.Lock()
	d.pendingHeap.Push(item)
	d.heapMutex.Unlock()

	d.heapCond.Signal()
}

func (d *DoubleBuffer) swapMonitor() {
	ticker := time.NewTicker(SwitchCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			if d.needSwitch() {
				if atomic.CompareAndSwapInt32(&d.swapPending, 0, 1) {
					select {
					case d.swapSignal <- struct{}{}:
					default:
					}
				}
			}
		case <-d.swapSignal:
			d.switchChannel()
		}
	}
}

// processor Asynchronous processor, used to poll and obtain the buffer corresponding to the minimum
// sequence that needs to be processed in blocking sorting.
func (d *DoubleBuffer) processor() {
	defer d.wg.Done()

	const (
		smallSize  = 32
		mediumSize = 125
	)
	const mod = 5
	bufferSize := Chanjet.SmallBatchSize

	for {
		d.heapMutex.Lock()
		for {
			select {
			case <-d.stop:
				d.drainProcessor()
				d.heapMutex.Unlock()
				return
			default:
			}

			if d.pendingHeap.Len() > 0 {
				top := (*d.pendingHeap)[0]
				if top.sequence == atomic.LoadInt64(&d.currentSequence) {
					break
				}
			}

			if !d.condWithTimeout(100 * time.Millisecond) {
				continue
			}
		}

		top := heap.Pop(d.pendingHeap).(MinHeapItem)
		d.heapMutex.Unlock()

		l := d.pendingHeap.Len()
		if l == 0 {
			d.heapMutex.Unlock()
			continue
		}

		if l%mod == 0 {
			switch {
			case l < smallSize:
				bufferSize = Chanjet.SmallBatchSize
			case l < mediumSize:
				bufferSize = Chanjet.MediumBatchSize
			default:
				bufferSize = Chanjet.LargeBatchSize
			}
		}

		if top.sequence == atomic.LoadInt64(&d.currentSequence) {
			d.processBuffer(top.buf, bufferSize)
			atomic.AddInt64(&d.currentSequence, 1)
		} else {
			d.heapMutex.Lock()
			heap.Push(d.pendingHeap, top)
			d.heapMutex.Unlock()
		}
	}
}

// processBuffer Read the byte slices in the buffer in batches and write them to the readq queue.
func (d *DoubleBuffer) processBuffer(buf *SmartBuffer, batchSize int) {
	defer d.wg.Done()

	batch := make([][]byte, batchSize)
	var size int64
	var count int64
	flushFunc := func() {
		d.readq <- batch
		d.mc.RecordRead(count, size, nil)
		batch = batch[:0]
		size = 0
		count = 0
	}

	for {
		if buf.len() == 0 {
			break
		}

		ptr, sz := buf.pop()
		if ptr == nil {
			break
		}

		// safe read data
		data, _ := buf.safeRead()
		batch = append(batch, data)
		size += int64(sz)
		count++

		if count >= int64(batchSize) {
			flushFunc()
			continue
		}
	}

	if len(batch) == 0 {
		return
	}

	flushFunc()
}

func (d *DoubleBuffer) drainProcessor() {
	d.heapMutex.Lock()
	defer d.heapMutex.Unlock()

	for d.pendingHeap.Len() > 0 {
		item := heap.Pop(d.pendingHeap).(MinHeapItem)
		if item.buf != nil {
			item.buf.Close()
			d.pool.Put(item.buf)
		}
	}
}

func (d *DoubleBuffer) condWithTimeout(timeout time.Duration) bool {
	ch := make(chan struct{})

	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Println("cond.Wait panic:", err)
			}
		}()
		d.heapCond.Wait()
		close(ch)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ch:
		return true
	case <-timer.C:
		d.heapCond.Broadcast()
		return false
	case <-d.stop:
		d.heapCond.Broadcast()
		return false
	}
}

func (d *DoubleBuffer) Close() {
	if !atomic.CompareAndSwapInt32(&d.status, Chanjet.WritingStatus, Chanjet.ClosedStatus) {
		return
	}

	close(d.stop)
	d.active.Close()
	d.pool.Put(d.active)

	d.passive.Close()
	d.pool.Put(d.passive)
	d.wg.Wait()

	d.drainProcessor()

	if d.readq != nil {
		close(d.readq)
	}
}
