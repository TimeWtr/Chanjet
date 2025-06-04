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

	"github.com/TimeWtr/Chanjet"
	"github.com/TimeWtr/Chanjet/config"
	"github.com/TimeWtr/Chanjet/metrics"
)

type Options func(buffer *DoubleBuffer) error

// WithMetrics 开启指标采集，指定采集器类型
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

// WithSwitchCondition Set the channel switching conditions
func WithSwitchCondition(config config.SwitchConfig) Options {
	return func(buffer *DoubleBuffer) error {
		return buffer.sc.UpdateConfig(config)
	}
}
