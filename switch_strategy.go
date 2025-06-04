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

package Chanjet
	
import "time"

type SwitchStrategy interface {
	NeedSwitch(currentCount, bufferSize int32, elapsed, interval time.Duration) bool
}

type DefaultStrategy struct{}

func (d *DefaultStrategy) NeedSwitch(currentCount, bufferSize int32, elapsed, interval time.Duration) bool {
	if currentCount >= bufferSize {
		return true
	}

	if elapsed >= interval {
		return true
	}

	// Calculate capacity factor (0-1)
	countFactor := float64(currentCount) / float64(bufferSize)
	// Calculate switch time factor (0-1)
	switchFactor := float64(elapsed) / float64(interval)
	combined := TimeWeight*switchFactor + SizeWeight*countFactor

	return combined > FullCapacity
}

type SizeOnlyStrategy struct{}

func (s *SizeOnlyStrategy) NeedSwitch(currentCount, bufferSize int32, _, _ time.Duration) bool {
	return currentCount >= bufferSize
}

type TimeWindowOnlyStrategy struct{}

func (s *TimeWindowOnlyStrategy) NeedSwitch(_, _ int32, elapsed, interval time.Duration) bool {
	return elapsed >= interval
}
