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

import "time"

type Observer interface {
	Update(metrics Metrics)
}

type Subject interface {
	Register(observer Observer)
	Unregister(observer Observer)
	NotifyAll()
}

type TimeoutController interface {
	Timeout(collectInterval time.Duration) time.Duration
	HandleTimeout(component string, collected int, latency time.Duration)
	Recover()
}
