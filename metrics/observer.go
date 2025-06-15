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
	"encoding/json"
	"fmt"

	"github.com/TimeWtr/TurboStream/utils/log"
	"github.com/prometheus/client_golang/prometheus"
)

type ConsoleObserver struct {
	l log.Logger
}

func NewConsoleObserver(l log.Logger) *ConsoleObserver {
	return &ConsoleObserver{l: l}
}

func (c *ConsoleObserver) Update(metrics Metrics) {
	summary := fmt.Sprintf(
		"[Metrics] Timestamp: %d | CPU: %.1f%% | Memory: %.1f%% | Goroutines: %d",
		metrics.Timestamp,
		metrics.CPU.Usage,
		metrics.Memory.UsedPercent,
		metrics.Runtime.Goroutines,
	)
	c.l.Info(summary)

	if data, err := json.MarshalIndent(metrics, "", "  "); err == nil {
		c.l.Info("Detailed metrics:")
		c.l.Info(string(data))
	}
}

type PrometheusObserver struct {
	cpuUsage    prometheus.Gauge
	memUsage    prometheus.Gauge
	goroutines  prometheus.Gauge
	heapAlloc   prometheus.Gauge
	networkSent prometheus.Gauge
	networkRcv  prometheus.Gauge
	diskUsage   *prometheus.GaugeVec
	registry    *prometheus.Registry
	l           log.Logger
}

func NewPrometheusObserver(l log.Logger) *PrometheusObserver {
	registry := prometheus.NewRegistry()
	observer := &PrometheusObserver{
		registry: registry,
		cpuUsage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "system_cpu_usage_percent",
			Help: "Current CPU usage in percent",
		}),
		memUsage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "system_memory_usage_percent",
			Help: "Current memory usage in percent",
		}),
		goroutines: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "runtime_goroutines",
			Help: "Number of active goroutines",
		}),
		heapAlloc: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "runtime_heap_alloc_bytes",
			Help: "Bytes allocated on the heap",
		}),
		networkSent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "network_bytes_sent",
			Help: "Total bytes sent over the network",
		}),
		networkRcv: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "network_bytes_received",
			Help: "Total bytes received over the network",
		}),
		diskUsage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "disk_usage_percent",
			Help: "Disk usage by partition",
		}, []string{"partition", "mountpoint"}),
		l: l,
	}

	registry.MustRegister(
		observer.cpuUsage,
		observer.memUsage,
		observer.goroutines,
		observer.heapAlloc,
		observer.networkSent,
		observer.networkRcv,
		observer.diskUsage,
	)

	return observer
}

func (p *PrometheusObserver) Update(metric Metrics) {
	p.cpuUsage.Set(metric.CPU.Usage)
	p.memUsage.Set(metric.Memory.UsedPercent)
	p.goroutines.Set(float64(metric.Runtime.Goroutines))
	p.heapAlloc.Set(float64(metric.Runtime.HeapAlloc))
	p.networkSent.Set(float64(metric.Network.TotalBytesSent))
	p.networkRcv.Set(float64(metric.Network.TotalBytesRecv))

	for index := range metric.Disk.Partitions {
		partition := metric.Disk.Partitions[index]
		p.diskUsage.WithLabelValues(partition.Device, partition.MountPoint).Set(partition.UsedPercent)
	}
}

func (p *PrometheusObserver) Registry() *prometheus.Registry {
	return p.registry
}
