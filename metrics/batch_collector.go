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

package metrics

import (
	"sync/atomic"
	"time"

	chanjet "github.com/TimeWtr/Chanjet"
)

// BatchCollector 批量上报指标数据的采集器，抽象出来给到调用方的接口
type BatchCollector interface {
	Controller
	Recorder
}

// Recorder 提供给调用方的接口
type Recorder interface {
	RecordWrite(size int64, err error)                              // 上报写数据
	RecordRead(count, size int64, err error)                        // 上报读数据
	RecordSwitch(status chanjet.SwitchStatus, latencySeconds int64) // 上报通道切换数据
	ObserveAsyncWorker(op chanjet.OperationType)                    // 上报异步goroutine数据
	RecordPoolAlloc()                                               // 上报池对象创建数据
}

// Controller 批量更新控制器
type Controller interface {
	Start() // 启动异步批量更新
	Stop()  // 停止异步批量更新
	Flush() // 强制立即批量更新
}

// Write 写数据的指标
type Write struct {
	writeCounts             int64 // 写入的总条数数量
	writeSizes              int64 // 写入的总大小
	writeErrors             int64 // 写入失败的错误计数
	activeChannelDataCounts int64 // 活跃通道中写入数据的条数
	activeChannelDataSizes  int64 // 活跃通道中写入数据的大小
}

func (w *Write) Reset() {
	atomic.StoreInt64(&w.activeChannelDataCounts, 0)
	atomic.StoreInt64(&w.activeChannelDataSizes, 0)
	atomic.StoreInt64(&w.writeSizes, 0)
	atomic.StoreInt64(&w.writeCounts, 0)
	atomic.StoreInt64(&w.writeErrors, 0)
}

// Read 读数据的指标
type Read struct {
	readCounts int64 // 写入读取通道的总条数
	readSizes  int64 // 写入读取通道的总大小
	readErrors int64 // 写入读取通道错误计数
}

func (r *Read) Reset() {
	atomic.StoreInt64(&r.readCounts, 0)
	atomic.StoreInt64(&r.readSizes, 0)
	atomic.StoreInt64(&r.readErrors, 0)
}

type Supporting struct {
	asyncWorkerIncCounts int64 // 异步处理协程增加数
	asyncWorkerDecCounts int64 // 异步处理协程减少数
	poolAlloc            int64 // 对象池分配次数
	switchCounts         int64 // 缓冲区切换次数
	switchLatency        int64 // 切换延迟
	skipSwitchCounts     int64 // 定时任务跳过通道切换的次数
}

func (s *Supporting) Reset() {
	atomic.StoreInt64(&s.asyncWorkerIncCounts, 0)
	atomic.StoreInt64(&s.asyncWorkerDecCounts, 0)
	atomic.StoreInt64(&s.switchCounts, 0)
	atomic.StoreInt64(&s.switchLatency, 0)
	atomic.StoreInt64(&s.skipSwitchCounts, 0)
	atomic.StoreInt64(&s.poolAlloc, 0)
}

var _ Recorder = (*BatchCollectImpl)(nil)

// BatchCollectImpl 批量指标采集器，封装底层采集器，增加定时任务
// 定期将指标数据写入到底层采集器
type BatchCollectImpl struct {
	w   *Write        // 写数据指标
	r   *Read         // 读数据指标
	sp  *Supporting   // 支撑性指标
	mc  Collector     // 底层指标采集器
	t   *time.Ticker  // 定时器
	sem chan struct{} // 关闭定时器
}

func NewBatchCollector(mc Collector) *BatchCollectImpl {
	const duration = time.Second * 5
	b := &BatchCollectImpl{
		w:   &Write{},
		r:   &Read{},
		sp:  &Supporting{},
		mc:  mc,
		t:   time.NewTicker(duration),
		sem: make(chan struct{}),
	}

	b.mc.CollectSwitcher(true)

	return b
}

func (b *BatchCollectImpl) RecordWrite(size int64, err error) {
	if err != nil {
		atomic.AddInt64(&b.w.writeErrors, 1)
		return
	}

	atomic.AddInt64(&b.w.writeCounts, 1)
	atomic.AddInt64(&b.w.writeSizes, size)
	atomic.AddInt64(&b.w.activeChannelDataCounts, 1)
	atomic.AddInt64(&b.w.activeChannelDataSizes, size)
}

func (b *BatchCollectImpl) RecordRead(count, size int64, err error) {
	if err != nil {
		atomic.AddInt64(&b.r.readErrors, 1)
		return
	}

	atomic.AddInt64(&b.r.readCounts, count)
	atomic.AddInt64(&b.r.readSizes, size)
}

func (b *BatchCollectImpl) RecordSwitch(status chanjet.SwitchStatus, latencySeconds int64) {
	switch status {
	case chanjet.SwitchSkip:
		atomic.AddInt64(&b.sp.skipSwitchCounts, 1)
	case chanjet.SwitchSuccess:
		atomic.AddInt64(&b.sp.switchCounts, 1)
		atomic.StoreInt64(&b.sp.switchLatency, latencySeconds)
	case chanjet.SwitchFailure:
	}
}

func (b *BatchCollectImpl) ObserveAsyncWorker(op chanjet.OperationType) {
	if op == chanjet.MetricsIncOp {
		atomic.AddInt64(&b.sp.asyncWorkerIncCounts, 1)
		return
	}

	atomic.AddInt64(&b.sp.asyncWorkerDecCounts, 1)
}

func (b *BatchCollectImpl) RecordPoolAlloc() {
	atomic.AddInt64(&b.sp.poolAlloc, 1)
}

func (b *BatchCollectImpl) Start() {
	go b.asyncWorker()
}

func (b *BatchCollectImpl) Stop() {
	close(b.sem)
}

func (b *BatchCollectImpl) Flush() {
	b.report()
}

func (b *BatchCollectImpl) asyncWorker() {
	for {
		select {
		case <-b.sem:
			return
		case <-b.t.C:
			b.report()
		}
	}
}

// report 同步一次指标数据
func (b *BatchCollectImpl) report() {
	b.mc.ObserveWrite(float64(atomic.LoadInt64(&b.w.writeCounts)),
		float64(atomic.LoadInt64(&b.w.writeSizes)),
		float64(atomic.LoadInt64(&b.w.writeErrors)))
	b.mc.ObserveActive(float64(atomic.LoadInt64(&b.w.activeChannelDataCounts)),
		float64(atomic.LoadInt64(&b.w.activeChannelDataSizes)))
	b.w.Reset()

	b.mc.ObserveRead(float64(atomic.LoadInt64(&b.r.readCounts)),
		float64(atomic.LoadInt64(&b.r.readSizes)),
		float64(atomic.LoadInt64(&b.r.readErrors)))
	b.r.Reset()

	b.mc.ObserveAsyncGoroutine(chanjet.MetricsIncOp, float64(atomic.LoadInt64(&b.sp.asyncWorkerIncCounts)))
	b.mc.ObserveAsyncGoroutine(chanjet.MetricsDecOp, float64(atomic.LoadInt64(&b.sp.asyncWorkerDecCounts)))
	b.mc.AllocInc(float64(atomic.LoadInt64(&b.sp.poolAlloc)))
	b.mc.SwitchWithLatency(chanjet.SwitchSuccess,
		float64(atomic.LoadInt64(&b.sp.switchCounts)),
		float64(atomic.LoadInt64(&b.sp.switchLatency)))
	b.mc.SwitchWithLatency(chanjet.SwitchSkip,
		float64(atomic.LoadInt64(&b.sp.skipSwitchCounts)), 0)
	b.sp.Reset()
}
