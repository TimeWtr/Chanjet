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
	"reflect"
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
	SmallDataThreshold      = 1024            // Small data threshold (<1KB)
	LargeDataThreshold      = 32 * 1024       // Big data threshold (>32KB)
	MediumDataCacheDuration = 5 * time.Second // Cache time for medium-sized data
	SwitchCheckInterval     = 5 * time.Millisecond
)

type Options func(buffer *DoubleBuffer) error

// WithMetrics Enable indicator collection and specify the collector type
func WithMetrics(collector Chanjet.CollectorType) Options {
	return func(buffer *DoubleBuffer) error {
		if !collector.Validate() {
			return errors.New("invalid metrics collector")
		}

		buffer.enableMetrics = true
		switch collector {
		case Chanjet.PrometheusCollector:
			buffer.mc = metrics.NewBatchCollector(metrics.NewPrometheus())
		case Chanjet.OpenTelemetryCollector:
		}

		return nil
	}
}

//// WithSwitchCondition Set the channel switching conditions
//func WithSwitchCondition(config config.SwitchConfig) Options {
//	return func(buffer *DoubleBuffer) error {
//		return buffer.sc.UpdateConfig(config)
//	}
//}

type BufferItem struct {
	ptr  unsafe.Pointer
	size int32
}

type SmartBuffer struct {
	buf      []BufferItem            // stores the header information corresponding to []byte
	head     int32                   // write index
	tail     int32                   // read index
	count    int32                   // the number of data currently written
	capacity int32                   // smart buffer capacity setting
	status   int32                   // smart buffer status
	pm       *pools.LifeCycleManager // buffer pool lifecycle manager
	sem      chan struct{}           // closed signal
}

func newSmartBuffer(capacity int32) *SmartBuffer {
	s := &SmartBuffer{
		buf:      make([]BufferItem, capacity),
		head:     -1,
		tail:     -1,
		count:    0,
		capacity: capacity,
		status:   Chanjet.WritingStatus,
		pm:       pools.NewLifeCycleManager(),
		sem:      make(chan struct{}),
	}

	return s
}

func (s *SmartBuffer) len() int {
	return int(atomic.LoadInt32(&s.count))
}

func (s *SmartBuffer) write(p []byte) bool {
	if s.status == Chanjet.ClosedStatus {
		return false
	}

	header := (*reflect.SliceHeader)(unsafe.Pointer(&p))
	bufferItem := BufferItem{
		ptr:  unsafe.Pointer(header.Data),
		size: int32(header.Len),
	}
	switch {
	case bufferItem.size < SmallDataThreshold:
		buf, _ := s.pm.SmallPool.Get().([]byte)
		buf = buf[:bufferItem.size]
		copy(buf, p)
		bufferItem.ptr = unsafe.Pointer(&buf[0])
	case bufferItem.size < LargeDataThreshold:
		s.pm.BigDataPool.Put(uintptr(bufferItem.ptr), p)
	default:
		s.pm.MediumPool.Put(uintptr(bufferItem.ptr), time.Now())
	}

	return s.push(bufferItem)
}

// zeroCopyRead is a non-safe API. When using this API, you must ensure that the data is not modified
// after Write. Otherwise, the zero-copy data will be wrong.
func (s *SmartBuffer) zeroCopyRead() ([]byte, bool) {
	ptr, size := s.pop()
	if ptr == nil {
		return nil, false
	}

	res := *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Data: uintptr(ptr),
		Len:  int(size),
		Cap:  int(size),
	}))

	if size > LargeDataThreshold {
		ptrVal := uintptr(ptr)
		s.pm.BigDataPool.Release(ptrVal)
	}

	return res, true
}

// safeRead Read data, secure API, return default copy.
func (s *SmartBuffer) safeRead() ([]byte, bool) {
	ptr, size := s.pop()
	if size == 0 {
		return nil, false
	}

	ptrData := *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Data: uintptr(ptr),
		Len:  int(size),
		Cap:  int(size),
	}))
	switch {
	case size < SmallDataThreshold:
		buf, _ := s.pm.SmallPool.Get().([]byte)
		buf = buf[:size]
		copy(buf, ptrData)
		return buf, true
	case size > LargeDataThreshold:
		// Big data returns a copy (safe default)
		data := make([]byte, size)
		copy(data, ptrData)
		s.recycle(ptr, size)
		return data, true
	default:
		if s.pm.MediumPool.IsValid(uintptr(ptr)) {
			// Data within the validity period (zero copy return)
			return ptrData, true
		}

		// Cache invalidation, return a copy (safe default)
		data := make([]byte, size)
		copy(data, ptrData)
		return data, true
	}
}

// Release the data read by the zero-copy API
func (s *SmartBuffer) Release(data []byte) {
	if len(data) < LargeDataThreshold {
		return
	}

	ptrVal := uintptr(unsafe.Pointer(&data[0]))
	s.pm.BigDataPool.Release(ptrVal)
}

// push The method that actually executes the data writing
func (s *SmartBuffer) push(sli BufferItem) bool {
	if s.status == Chanjet.ClosedStatus {
		return false
	}

	var head int32
	for {
		currentCount := atomic.LoadInt32(&s.count)
		if currentCount >= s.capacity {
			return false
		}

		head = atomic.LoadInt32(&s.head)
		newHead := head + 1
		if atomic.CompareAndSwapInt32(&s.head, head, newHead) {
			break
		}
	}

	pos := (head + 1) % s.capacity
	if pos < 0 {
		pos += s.capacity
	}
	pos %= s.capacity

	s.buf[pos] = sli
	atomic.AddInt32(&s.count, 1)

	return true
}

// pop Execute data acquisition and return pointer and data length
func (s *SmartBuffer) pop() (unsafe.Pointer, int32) {
	var pos int32 = -1
	for {
		if atomic.LoadInt32(&s.count) == 0 {
			return nil, 0
		}

		tail := atomic.LoadInt32(&s.tail)
		newTail := tail + 1
		if atomic.CompareAndSwapInt32(&s.tail, tail, newTail) {
			pos = newTail % s.capacity
			break
		}
	}

	wrapper := s.buf[pos]
	if wrapper.ptr == nil {
		return nil, 0
	}

	s.buf[pos] = BufferItem{}
	atomic.AddInt32(&s.count, ^int32(0))
	return wrapper.ptr, wrapper.size
}

//func (s *SmartBuffer) recycleWorker() {
//	defer s.wg.Done()
//
//	ticker := time.NewTicker(time.Second)
//	defer ticker.Stop()
//
//	for {
//		if atomic.LoadInt32(&s.status) == Chanjet.ClosedStatus {
//			return
//		}
//
//		select {
//		case <-s.sem:
//			return
//		case <-ticker.C:
//			if atomic.LoadInt32(&s.status) == Chanjet.ClosedStatus {
//				return
//			}
//			s.pm.Cleanup()
//		}
//	}
//}

// recycle releases resources, three different sizes.
// 1. SmallData: Get the []byte corresponding to ptr, reset and put it back into the buffer pool
// 2. MediumData: Delete the mapping relationship between ptr and time
// 3. LargeData: -1 the reference count of ptr in the pool
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

func (s *SmartBuffer) Close() {
	if !atomic.CompareAndSwapInt32(&s.status, Chanjet.WritingStatus, Chanjet.ClosedStatus) {
		return
	}

	close(s.sem)
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
	// The buffer should to read data.
	currentBuffer *SmartBuffer
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
	// Blocking notifications for switching
	swapSignal chan struct{}
	// Switch pending state
	swapPending int32
	// buffer object pool
	pool sync.Pool
	//// Buffer for safe batch reads
	//readq chan [][]byte
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
	pendingHeap *WrapHeap
	// Used to determine whether to enable indicator monitoring,
	enableMetrics bool
	// Batch indicator collector for receiving indicator data from double-buffered channels in real time.
	mc metrics.BatchCollector
	// The condition to switch channels.
	sc       config.SwitchConfig
	scd      *config.SwitchCondition
	scNotify <-chan struct{}
	// The strategy switching channels.
	sw Chanjet.SwitchStrategy
	// The read mode, include zero copy read and safe read.
	readMode *Chanjet.ReadMode
	// Waiters manager
	wm *WaiterManager
}

func NewDoubleBuffer(size int32, sc *config.SwitchCondition, opts ...Options) (*DoubleBuffer, error) {
	d := &DoubleBuffer{
		active:          newSmartBuffer(size),
		passive:         newSmartBuffer(size),
		stop:            make(chan struct{}),
		size:            size,
		scd:             sc,
		sc:              sc.GetConfig(),
		scNotify:        sc.Register(),
		lastSwitch:      time.Now().UnixMilli(),
		pendingHeap:     NewWrapHeap(),
		currentSequence: 1,
		sw:              Chanjet.NewDefaultStrategy(),
		swapSignal:      make(chan struct{}, 1),
		mc:              metrics.NewBatchCollector(metrics.NewPrometheus()),
		wg:              sync.WaitGroup{},
		wm:              newWaiterManager(),
	}

	for _, opt := range opts {
		if err := opt(d); err != nil {
			return nil, err
		}
	}

	d.pool = sync.Pool{
		New: func() interface{} {
			return newSmartBuffer(size)
		},
	}

	d.active, _ = d.pool.Get().(*SmartBuffer)
	d.passive, _ = d.pool.Get().(*SmartBuffer)

	d.wg.Add(1)
	go d.swapMonitor()

	return d, nil
}

func (d *DoubleBuffer) Write(p []byte) error {
	if atomic.LoadInt32(&d.status) == Chanjet.ClosedStatus {
		return errorx.ErrBufferClose
	}

	const mod = 10
	if atomic.LoadInt32(&d.count)%mod == 0 {
		if d.needSwitch() {
			atomic.CompareAndSwapInt32(&d.swapPending, 0, 1)
			select {
			case d.swapSignal <- struct{}{}:
				d.switchChannel()
			default:
			}
		}
	}

	// Try to write to buffer
	if d.active.write(p) {
		atomic.AddInt32(&d.count, 1)
		return nil
	}

	return errors.New("write error")
}

// needSwitch determines whether a channel switch needs to be executed. The switching conditions
// are as follows:
// 1. The comprehensive factor exceeds the threshold
// 2. The size of the current active buffer exceeds the buffer capacity
// 3. The time interval between the current time and the last switch exceeds the specified time window
// If any of the conditions is met, the channel switch needs to be executed immediately.
func (d *DoubleBuffer) needSwitch() bool {
	currentCount := atomic.LoadInt32(&d.count)
	lastSwitch := time.UnixMilli(atomic.LoadInt64(&d.lastSwitch))
	elapsed := time.Since(lastSwitch)
	select {
	case <-d.scNotify:
		d.sc = d.scd.GetConfig()
	default:
	}

	return d.sw.NeedSwitch(currentCount, d.size, elapsed, d.sc.TimeThreshold)
}

// switchChannel Perform channel switching
func (d *DoubleBuffer) switchChannel() {
	defer func() {
		select {
		case <-d.swapSignal:
		default:
		}
	}()

	if !atomic.CompareAndSwapInt32(&d.swapPending, 1, 0) {
		return
	}

	newBuf, _ := d.pool.Get().(*SmartBuffer)
	oldActive := d.active
	d.active, d.passive = d.passive, newBuf
	seq := atomic.AddInt64(&d.sequence, 1)
	item := MinHeapItem{
		sequence: seq,
		buf:      oldActive,
	}
	oldActive.Close()

	d.pendingHeap.Push(&item)
	count := atomic.LoadInt32(&d.count)
	atomic.CompareAndSwapInt32(&d.count, count, 0)
	atomic.StoreInt64(&d.lastSwitch, time.Now().UnixMilli())
	go d.wm.notify(oldActive.len())
}

func (d *DoubleBuffer) swapMonitor() {
	defer d.wg.Done()

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

func (d *DoubleBuffer) pickBufferFromHeap() bool {
	counter := 0
	maxRetires := 3
	for counter < maxRetires {
		bufferItem := d.pendingHeap.Pick()
		if bufferItem == nil {
			return false
		}

		if bufferItem.sequence == atomic.LoadInt64(&d.currentSequence) {
			d.currentBuffer = bufferItem.buf
			atomic.AddInt64(&d.currentSequence, 1)
			break
		}

		d.pendingHeap.Push(bufferItem)
		counter++
		time.Sleep(100 * time.Millisecond)
	}

	if d.currentBuffer == nil {
		return false
	}

	return true
}

func (d *DoubleBuffer) RegisterReadMode(readMode Chanjet.ReadMode) error {
	if !readMode.Validate() {
		return errors.New("invalid read mode")
	}

	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&d.readMode)),
		unsafe.Pointer(&readMode))
	return nil
}

// BlockingRead Blocking reading requires passing in a Context with timeout control.
// If there is data, read the data immediately and return. If there is no data, wait
// for new data in a blocking manner until the context times out or the channel is closed.
// It will not block forever.
func (d *DoubleBuffer) BlockingRead(ctx context.Context) ([]byte, error) {
	data, err := d.tryRead()
	if err == nil {
		return data, nil
	}

	id, ch := d.wm.register()
	defer d.wm.unregister(id)

	select {
	case <-ch:
		return d.tryRead()
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-d.stop:
		return nil, errorx.ErrBufferClose
	}
}

func (d *DoubleBuffer) tryRead() ([]byte, error) {
	if d.currentBuffer == nil || d.currentBuffer.len() == 0 {
		if d.currentBuffer != nil {
			d.currentBuffer.Close()
		}

		if !d.pickBufferFromHeap() {
			return nil, errorx.ErrNoBuffer
		}
	}

	readMode := *(*Chanjet.ReadMode)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&d.readMode))))
	switch readMode {
	case Chanjet.SafeRead:
		res, ok := d.currentBuffer.safeRead()
		if ok {
			return res, nil
		}
	case Chanjet.ZeroCopyRead:
		res, ok := d.currentBuffer.zeroCopyRead()
		if ok {
			return res, nil
		}
	default:
		return nil, errorx.ErrReadMode
	}

	return nil, errorx.ErrRead
}

func (d *DoubleBuffer) RegisterCallback(cb DataCallBack) UnregisterFunc {
	//TODO implement me
	panic("implement me")
}

func (d *DoubleBuffer) BatchRead(ctx context.Context, batchSize int) [][]byte {
	//TODO implement me
	panic("implement me")
}

func (d *DoubleBuffer) drainProcessor() {
	for d.pendingHeap.Len() > 0 {
		item := d.pendingHeap.Pick()
		if item.buf != nil {
			item.buf.Close()
			d.pool.Put(item.buf)
		}
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
}
