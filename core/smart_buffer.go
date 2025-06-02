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
		status: _const.WritingStatus,
		pm:     pools.NewLifeCycleManager(),
	}

	go s.recycleWorker()
	return s
}

func (s *SmartBuffer) write(p []byte) bool {
	if s.status == _const.ClosedStatus {
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

// zeroCopyRead 零拷贝读取，非安全API，使用该API必须保证数据在Write后没有进行修改
// 否则零拷贝的数据将是错误的
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

// read 读取数据，安全API，默认返回副本
func (s *SmartBuffer) read() ([]byte, bool) {
	ptr, l := s.pop()
	if s.status == _const.ClosedStatus {
		return nil, false
	}

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
	if s.status == _const.ClosedStatus {
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
		if atomic.LoadInt32(&s.status) == _const.ClosedStatus {
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
	atomic.StoreInt32(&s.status, _const.ClosedStatus)
	for s.count > 0 {
		ptr, _ := s.pop()
		if ptr == nil {
			continue
		}

		s.recycle(ptr, s.getSizeFromPtr(ptr))
	}
}

// DoubleBuffer 双缓冲区设计
type DoubleBuffer struct {
	active      *SmartBuffer      // 活跃缓冲区
	passive     *SmartBuffer      // 异步读取缓冲区
	stop        chan struct{}     // 关闭缓冲区信号
	size        int32             // 缓冲区的容量
	count       int32             // 当前活跃缓冲区写入的数据条数
	status      int32             // 缓冲区状态
	lastSwitch  int64             // 上次切换的时间，毫秒级别
	interval    time.Duration     // 后台轮转的时间窗口
	swapSignal  chan struct{}     // 阻塞进行切换的通知
	swapPending int32             // 切换阻塞状态
	directCond  *sync.Cond        // 等待读取的条件
	directMutex sync.RWMutex      // 直接读取的锁
	pool        sync.Pool         // 缓冲区对象池
	readq       [][]byte          // 批量安全读取的缓冲区
	taskQueue   chan *SmartBuffer // 需要被读取处理的缓冲区队列
	wg          sync.WaitGroup    // 同步控制
}

func NewDoubleBuffer(size int32) *DoubleBuffer {
	d := &DoubleBuffer{
		active:     newSmartBuffer(size),
		passive:    newSmartBuffer(size),
		stop:       make(chan struct{}),
		size:       size,
		lastSwitch: time.Now().UnixMilli(),
	}

	d.pool = sync.Pool{
		New: func() interface{} {
			return newSmartBuffer(size)
		},
	}

	d.active, _ = d.pool.Get().(*SmartBuffer)
	d.passive, _ = d.pool.Get().(*SmartBuffer)
	d.directCond = sync.NewCond(&d.directMutex)

	return d
}

func (d *DoubleBuffer) Write(p []byte) error {
	if atomic.LoadInt32(&d.status) == _const.ClosedStatus {
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

	// 尝试写入缓冲区
	if d.active.write(p) {
		if atomic.LoadInt32(&d.passive.count) == 0 {
			d.directCond.Broadcast()
		}
	}
	// TODO 处理写入
	return nil
}

// needSwitch 判断是否需要执行通道切换，切换条件有如下：
// 1. 综合因子超过阈值
// 2. 当前活跃缓冲区的大小超过缓冲区的容量
// 3. 当前时间与上次切换的时间间隔超过了规定的时间窗口
// 满足任意一条，即需要立即执行通道切换
func (d *DoubleBuffer) needSwitch() bool {
	const (
		SizeWeight   = 0.6
		TimeWeight   = 0.4
		FullCapacity = 1.0
	)

	currentCount := atomic.LoadInt32(&d.count)
	lastSwitch := atomic.LoadInt64(&d.lastSwitch)
	// 计算容量因子(0-1)
	countFactor := float64(currentCount) / float64(d.size)
	// 计算时间因子(0-1)
	switchFactor := float64(time.Since(time.UnixMilli(lastSwitch))) / float64(d.interval)
	combined := TimeWeight*switchFactor + SizeWeight*countFactor

	return combined > FullCapacity ||
		d.count >= d.size ||
		time.Since(time.UnixMilli(d.lastSwitch)) > d.interval
}

// switchChannel 执行通道切换
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
	// TODO 目前暂时以阻塞性来实现全局消息的有序性，后续优化为链表或序列号来实现全局有序
	d.taskQueue <- oldActiveVal
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
