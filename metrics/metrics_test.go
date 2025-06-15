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
	"testing"
	"time"

	"github.com/TimeWtr/TurboStream/utils/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

const mockInterval = time.Second * 3

func getLog() *zap.Logger {
	l, _ := zap.NewDevelopment()
	return l
}

type MockObserver struct {
	mock.Mock
}

func (m *MockObserver) Update(metrics Metrics) {
	m.Called(metrics)
}

type MockCollector struct {
	mock.Mock
}

func (m *MockCollector) Collect() CPUStates {
	args := m.Called()
	state, _ := args.Get(0).(CPUStates)
	return state
}

func TestMonitorLifecycle(t *testing.T) {
	logger := log.NewZapAdapter(getLog())
	monitor, err := NewMonitor(logger,
		WithTimeoutController(newOpenSourceTimeout(1.5, logger)),
	)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	monitor.Start(ctx)

	time.Sleep(10 * time.Millisecond)

	mockObs := new(MockObserver)
	mockObs.On("Update", mock.Anything).Return()
	monitor.Register(mockObs)

	time.Sleep(mockInterval * 2)
	mockObs.AssertCalled(t, "Update", mock.Anything)

	cancel()
	monitor.Stop()
}

//func TestObserverRegistration(t *testing.T) {
//	logger := log.NewZapAdapter(getLog())
//	monitor, _ := NewMonitor(logger)
//
//	obs1 := new(MockObserver)
//	obs2 := new(MockObserver)
//
//	monitor.Register(obs1)
//	monitor.Register(obs2)
//	assert.Len(t, monitor.observers, 2)
//
//	monitor.Unregister(obs1)
//	assert.Len(t, monitor.observers, 1)
//
//	monitor.Unregister(obs2)
//	assert.Empty(t, monitor.observers)
//}
//
//func TestCollectionTimeout(t *testing.T) {
//	logger := log.NewZapAdapter(getLog())
//	timeoutCtrl := newOpenSourceTimeout(0, logger)
//
//	monitor, _ := NewMonitor(logger,
//		WithCollectInterval(time.Second),
//		WithTimeoutController(timeoutCtrl),
//	)
//
//	ctx := context.Background()
//	monitor.Start(ctx)
//	defer monitor.Stop()
//
//	time.Sleep(mockInterval * 2)
//	assert.Greater(t, monitor.meta.ErrCount, 0)
//}
//
////func TestDiskCollectorFirstRun(t *testing.T) {
////	collector := newDiskCollector()
////
////	firstResult := collector.Collect(time.Time{})
////	assert.Empty(t, firstResult.Partitions)
////	assert.NotNil(t, firstResult.PerDiskIO)
////
////	time.Sleep(10 * time.Millisecond)
////	secondResult := collector.Collect(time.Now())
////	assert.NotEmpty(t, secondResult.Partitions)
////	assert.NotEmpty(t, secondResult.PerDiskIO)
////}
//
//func TestMetaDataStatistics(t *testing.T) {
//	logger := log.NewZapAdapter(getLog())
//	monitor, _ := NewMonitor(logger)
//
//	for i := 0; i < 5; i++ {
//		_ = monitor.collectAllMetrics()
//		time.Sleep(10 * time.Millisecond)
//	}
//
//	assert.Equal(t, int64(5), monitor.meta.SuccessCount)
//	assert.Greater(t, monitor.meta.AverageTimeTaken, int64(0))
//	assert.Len(t, monitor.meta.TimeTakenQueue, 5)
//}
//
//func TestGracefulShutdown(t *testing.T) {
//	logger := log.NewZapAdapter(getLog())
//	monitor, _ := NewMonitor(logger)
//
//	_ = monitor.pool.Submit(func() {
//		time.Sleep(200 * time.Millisecond)
//	})
//
//	ctx := context.Background()
//	monitor.Start(ctx)
//	monitor.Stop()
//}
