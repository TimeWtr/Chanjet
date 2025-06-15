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
	"sync"
	"time"

	"github.com/TimeWtr/TurboStream/utils/atomicx"
	"github.com/TimeWtr/TurboStream/utils/log"
	"github.com/panjf2000/ants"
	"golang.org/x/net/context"
)

const (
	collectInterval = 5 * time.Second
	reportInterval  = 5 * time.Second
)

type Options func(*Monitor)

func WithTimeoutController(ctrl TimeoutController) Options {
	return func(r *Monitor) {
		r.timeoutCtrl = ctrl
	}
}

func WithCollectInterval(interval time.Duration) Options {
	return func(r *Monitor) {
		r.reportInterval = interval
	}
}

func WithProcessMonitoring(pid int) Options {
	return func(r *Monitor) {
		r.pid = pid
	}
}

const (
	workerPoolSize = 10
	workers        = 5
	threshold      = 5
	mod            = 5
)

type Monitor struct {
	collectInterval   time.Duration
	reportInterval    time.Duration
	pid               int
	observers         []Observer
	cpuCollector      *CPUCollector
	diskCollector     *DiskCollector
	memCollector      *MemoryCollector
	networkCollector  *NetworkCollector
	runtimesCollector *RuntimesCollector
	ctx               context.Context
	cancelFunc        context.CancelFunc
	metrics           Metrics
	meta              Meta
	wg                sync.WaitGroup
	mu                sync.RWMutex
	pool              *ants.Pool
	timeoutCtrl       TimeoutController
	state             *atomicx.Bool
}

func NewMonitor(l log.Logger, opts ...Options) (*Monitor, error) {
	m := &Monitor{
		collectInterval:   collectInterval,
		reportInterval:    reportInterval,
		observers:         []Observer{},
		cpuCollector:      newCPUCollector(),
		diskCollector:     newDiskCollector(),
		memCollector:      newMemoryCollector(),
		networkCollector:  newNetworkCollector(),
		runtimesCollector: newRuntimesCollector(),
		timeoutCtrl:       newOpenSourceTimeout(float64(collectInterval), l),
		state:             atomicx.NewBool(),
	}

	const taskExpireDuration = 60 * time.Second
	pool, err := ants.NewPool(workerPoolSize,
		ants.WithExpiryDuration(taskExpireDuration),
		ants.WithPreAlloc(true),
		ants.WithNonblocking(true))
	if err != nil {
		return nil, err
	}
	m.pool = pool

	for _, opt := range opts {
		opt(m)
	}

	return m, nil
}

func (m *Monitor) Register(observer Observer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.observers = append(m.observers, observer)
}

func (m *Monitor) Unregister(observer Observer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, ob := range m.observers {
		if ob == observer {
			m.observers = append(m.observers[:i], m.observers[i+1:]...)
			return
		}
	}
}

func (m *Monitor) NotifyAll() {
	m.mu.Lock()
	copyObservers := make([]Observer, len(m.observers))
	copy(copyObservers, m.observers)
	m.mu.Unlock()

	for _, observer := range copyObservers {
		observer.Update(m.metrics)
	}
}

func (m *Monitor) Start(ctx context.Context) {
	if !m.state.CompareAndSwap(false, true) {
		return
	}

	m.ctx, m.cancelFunc = context.WithCancel(ctx)
	m.wg.Add(1)
	go m.runCollector()
}

func (m *Monitor) runCollector() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.collectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := m.collectAllMetrics()
			if err != nil {
				continue
			}

			m.NotifyAll()
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *Monitor) collectAllMetrics() error {
	start := time.Now()

	var wg sync.WaitGroup
	wg.Add(workers)
	_ = m.pool.Submit(func() { defer wg.Done(); m.metrics.CPU = m.cpuCollector.Collect() })
	_ = m.pool.Submit(func() { defer wg.Done(); m.metrics.Memory = m.memCollector.Collect() })
	_ = m.pool.Submit(func() { defer wg.Done(); m.metrics.Network = m.networkCollector.Collect() })
	_ = m.pool.Submit(func() { defer wg.Done(); m.metrics.Runtime = m.runtimesCollector.Collect() })
	_ = m.pool.Submit(func() { defer wg.Done(); m.metrics.Disk = m.diskCollector.Collect(m.meta.LastCollectTime) })

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// collect success
		m.mu.Lock()
		defer m.mu.Unlock()
		timeTaken := time.Since(start).Milliseconds()
		m.metrics.Timestamp = timeTaken
		m.meta.TimeTakenQueue = append(m.meta.TimeTakenQueue, timeTaken)
		m.meta.SuccessCount++
		if m.meta.SuccessCount%mod == 0 {
			var totalDuration int64
			l := len(m.meta.TimeTakenQueue)
			for _, duration := range m.meta.TimeTakenQueue {
				totalDuration += duration
			}
			m.meta.AverageTimeTaken = totalDuration / int64(l)
		}
		m.meta.LastCollectTime = time.Now()

		return nil
	case <-time.After(m.timeoutCtrl.Timeout(m.reportInterval)):
		// timeout
		m.mu.Lock()
		defer m.mu.Unlock()
		m.meta.ErrCount++
		m.meta.LastCollectTime = time.Now()
		return context.DeadlineExceeded
	}
}

func (m *Monitor) Stop() {
	if !m.state.CompareAndSwap(true, false) {
		return
	}
	if m.cancelFunc != nil {
		m.cancelFunc()
	}

	m.pool.Release()
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return
	case <-time.After(m.timeoutCtrl.Timeout(m.reportInterval)):
		return
	}
}
