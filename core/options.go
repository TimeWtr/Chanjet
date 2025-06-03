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
	"errors"
	"time"

	"github.com/TimeWtr/Chanjet/_const"
	"github.com/TimeWtr/Chanjet/metrics"
)

type Options func(buffer *DoubleBuffer) error

// WithMetrics 开启指标采集，指定采集器类型
func WithMetrics(collector _const.CollectorType) Options {
	return func(buffer *DoubleBuffer) error {
		if !collector.Validate() {
			return errors.New("invalid metrics collector")
		}
		buffer.enableMetrics = true
		switch collector {
		case _const.PrometheusCollector:
			buffer.mc = metrics.NewBatchCollector(metrics.NewPrometheus())
		case _const.OpenTelemetryCollector:
		}

		return nil
	}
}

// WithSizeThreshold 设置通道大小限制阈值，当设置为1024 * 1024 * 10 时，
// 当写入的Size达到了这个阈值就会触发通道切换
func WithSizeThreshold(size int64) Options {
	return func(buffer *DoubleBuffer) error {
		buffer.sc.sizeThreshold = size
		return nil
	}
}

// WithPercentThreshold 设置通道数量限制阈值，当设置为80%时，通道中的数据条数
// 达到总容量的80%时就会触发通道切换，范围（0-100）
func WithPercentThreshold(percentThreshold int) Options {
	return func(buffer *DoubleBuffer) error {
		buffer.sc.percentThreshold = percentThreshold
		return nil
	}
}

// WithTimeThreshold 设置定时切换通道的时间间隔，当设置为1s时，后台监控goroutine
// 会每隔1s执行一次通道切换，确保数据不会长期积压。
func WithTimeThreshold(timeThreshold time.Duration) Options {
	return func(buffer *DoubleBuffer) error {
		buffer.sc.timeThreshold = timeThreshold
		buffer.sc.timeThresholdMillis = timeThreshold.Milliseconds()
		return nil
	}
}
