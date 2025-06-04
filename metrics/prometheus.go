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
	"net/http"

	"github.com/TimeWtr/Chanjet"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	mc       *Prometheus
	registry *prometheus.Registry // 指标注册表
)

// GetHandler 返回HTTP处理器用于对接各种框架
func GetHandler() http.Handler {
	return promhttp.HandlerFor(
		registry,
		promhttp.HandlerOpts{EnableOpenMetrics: true},
	)
}

var _ Collector = (*Prometheus)(nil)

type Prometheus struct {
	enabled                 bool                   // 是否开启指标采集
	writeCounter            *prometheus.CounterVec // 写入的总条数数量
	writeSizes              prometheus.Counter     // 写入的总大小
	writeErrors             prometheus.Counter     // 写入失败的错误计数
	readCounter             *prometheus.CounterVec // 写入读取通道的总条数
	readSizes               prometheus.Counter     // 写入读取通道的总大小
	readErrors              prometheus.Counter     // 写入读取通道错误计数
	switchCounts            prometheus.Counter     // 缓冲区切换次数
	switchLatency           prometheus.Histogram   // 切换延迟
	skipSwitchCounts        prometheus.Counter     // 定时任务跳过通道切换的次数
	activeChannelDataCounts prometheus.Gauge       // 活跃通道中写入数据的条数
	activeChannelDataSizes  prometheus.Gauge       // 活跃通道中写入数据的大小
	asyncWorkers            prometheus.Gauge       // 异步处理协程数
	poolAlloc               prometheus.Counter     // 对象池分配次数
}

func NewPrometheus() *Prometheus {
	mc = &Prometheus{}
	registry = prometheus.NewRegistry()
	return mc.register()
}

func (p *Prometheus) register() *Prometheus {
	const namespace = "Chanjet"
	p.writeCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "write_counts_total",
		Help:      "Number of metrics written by write.",
	}, []string{"result"})
	registry.MustRegister(p.writeCounter)

	p.writeSizes = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "write_sizes_total",
		Help:      "Number of metrics written by write sizes.",
	})
	registry.MustRegister(p.writeSizes)

	p.writeErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "write_errors_total",
		Help:      "Number of errors encountered by write.",
	})
	registry.MustRegister(p.writeErrors)

	p.readCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "read_counts_total",
		Help:      "Number of metrics read.",
	}, []string{"result"})
	registry.MustRegister(p.readCounter)

	p.readSizes = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "read_metrics_sizes_total",
		Help:      "Number of metrics read sizes.",
	})
	registry.MustRegister(p.readSizes)

	p.readErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "read_metrics_errors_total",
		Help:      "Number of read errors.",
	})
	registry.MustRegister(p.readErrors)

	p.poolAlloc = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "pool_alloc_total",
		Help:      "Number of pool allocations.",
	})
	registry.MustRegister(p.poolAlloc)

	p.asyncWorkers = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "async_workers",
		Help:      "Number of async workers.",
	})
	registry.MustRegister(p.asyncWorkers)

	p.switchCounts = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "switch_counts_total",
		Help:      "Number of switches.",
	})
	registry.MustRegister(p.switchCounts)

	p.activeChannelDataCounts = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "active_channel_data_counts",
		Help:      "Number of active channels.",
	})
	registry.MustRegister(p.activeChannelDataCounts)

	p.activeChannelDataSizes = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "active_channel_data_sizes",
		Help:      "Number of active channel sizes.",
	})
	registry.MustRegister(p.activeChannelDataSizes)

	p.switchLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "switch_latency",
		Help:      "Latency of switches.",
	})
	registry.MustRegister(p.switchLatency)

	p.skipSwitchCounts = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "skip_switch_counts_total",
		Help:      "Number of skip switches.",
	})
	registry.MustRegister(p.skipSwitchCounts)

	return p
}

func (p *Prometheus) CollectSwitcher(enable bool) {
	p.enabled = enable
}

func (p *Prometheus) ObserveWrite(counts, bytes, errors float64) {
	if !p.enabled {
		return
	}

	p.writeCounter.With(prometheus.Labels{"result": "success"}).Add(counts)
	p.writeSizes.Add(bytes)
	p.writeErrors.Add(errors)
}

func (p *Prometheus) ObserveRead(counts, bytes, errors float64) {
	if !p.enabled {
		return
	}

	p.readCounter.With(prometheus.Labels{"result": "success"}).Add(counts)
	p.readSizes.Add(bytes)
	p.readErrors.Add(errors)
}

func (p *Prometheus) AllocInc(delta float64) {
	if !p.enabled {
		return
	}

	p.poolAlloc.Add(delta)
}

func (p *Prometheus) ObserveAsyncGoroutine(operation Chanjet.OperationType, counts float64) {
	if !p.enabled {
		return
	}

	if operation == Chanjet.MetricsIncOp {
		p.asyncWorkers.Add(counts)
	} else {
		p.asyncWorkers.Add(-counts)
	}
}

func (p *Prometheus) SwitchWithLatency(status Chanjet.SwitchStatus, counts, millSeconds float64) {
	if !p.enabled {
		return
	}

	switch status {
	case Chanjet.SwitchSuccess:
		p.switchCounts.Add(counts)
		p.switchLatency.Observe(millSeconds)
	case Chanjet.SwitchFailure:
	case Chanjet.SwitchSkip:
		p.skipSwitchCounts.Inc()
	}
}

func (p *Prometheus) ObserveActive(counts, size float64) {
	if !p.enabled {
		return
	}

	if size == 0 && counts == 0 {
		p.activeChannelDataCounts.Set(0)
		p.activeChannelDataSizes.Set(0)
	}

	p.activeChannelDataCounts.Add(counts)
	p.activeChannelDataSizes.Add(size)
}
