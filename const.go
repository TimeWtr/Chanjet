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

package chanjet

const Unknown = "unknown"

type CollectorType int

const (
	PrometheusCollector CollectorType = iota
	OpenTelemetryCollector
)

func (c CollectorType) String() string {
	switch c {
	case PrometheusCollector:
		return "Prometheus"
	case OpenTelemetryCollector:
		return "OpenTelemetry"
	default:
		return Unknown
	}
}

func (c CollectorType) Validate() bool {
	switch c {
	case PrometheusCollector, OpenTelemetryCollector:
		return true
	default:
		return false
	}
}

type OperationType int

const (
	MetricsIncOp OperationType = iota
	MetricsDecOp
)

type SwitchStatus int

const (
	SwitchSuccess SwitchStatus = iota
	SwitchFailure
	SwitchSkip
)

func (s SwitchStatus) String() string {
	switch s {
	case SwitchSuccess:
		return "Switch success"
	case SwitchFailure:
		return "Switch failure"
	case SwitchSkip:
		return "Switch skip"
	default:
		return "unknown"
	}
}

const (
	WritingStatus = iota
	PendingStatus
	ClosedStatus
)

const (
	SizeWeight   float64 = 0.6
	TimeWeight   float64 = 0.4
	FullCapacity float64 = 0.85
)

type ReadMode int

const (
	SafeRead ReadMode = iota + 1
	ZeroCopyRead
)

func (m ReadMode) String() string {
	switch m {
	case SafeRead:
		return "Safe read"
	case ZeroCopyRead:
		return "Zero-copy read"
	default:
		return "unknown"
	}
}

func (m ReadMode) Validate() bool {
	switch m {
	case SafeRead, ZeroCopyRead:
		return true
	default:
		return false
	}
}
