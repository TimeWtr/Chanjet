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
	"github.com/TimeWtr/Chanjet/_const"
)

// Collector 指标监控接口
type Collector interface {
	CollectSwitcher(enable bool) // 采集器开关
	WriteMetrics
	ReadMetrics
	PoolMetrics
	AsyncGoroutineMetrics
	ChannelMetrics
}

// WriteMetrics 写操作指标
type WriteMetrics interface {
	// ObserveWrite 写入的数量、写入的大小、错误数
	ObserveWrite(counts, bytes, errors float64)
}

// ReadMetrics 读取数据写入readq通道的指标数据
type ReadMetrics interface {
	// ObserveRead 读取的数量、写入的大小、错误数
	ObserveRead(counts, bytes, errors float64)
}

// PoolMetrics 缓存池指标数据
type PoolMetrics interface {
	// AllocInc 分配的对象计数增加的差值
	AllocInc(delta float64)
}

// AsyncGoroutineMetrics 异步读通道数据的goroutine数量
type AsyncGoroutineMetrics interface {
	// ObserveAsyncGoroutine 异步goroutine数量的监控，增加/减少都对应各自的数量
	ObserveAsyncGoroutine(operation _const.OperationType, delta float64)
}

// ChannelMetrics 通道相关的指标数据
type ChannelMetrics interface {
	ChannelSwitchMetrics
	ActiveChannelMetrics
}

// ChannelSwitchMetrics 通道切换的指标数据
type ChannelSwitchMetrics interface {
	SwitchWithLatency(status _const.SwitchStatus, counts float64, millSeconds float64)
}

// ActiveChannelMetrics 活跃缓冲区的通道指标
type ActiveChannelMetrics interface {
	ObserveActive(counts, size float64)
}
