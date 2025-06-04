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

package errorx

import (
	"errors"
)

var (
	ErrBufferClose = errors.New("buffer is closed")
	ErrBufferFull  = errors.New("buffer is full")
)

var (
	ErrPoolNil                 = errors.New("pool returned nil object")
	ErrPoolType                = errors.New("pool returned invalid type")
	ErrPoolEmpty               = errors.New("pool returned empty object")
	ErrPoolMaxSize             = errors.New("pool object over max size")
	ErrReadChannelWriteTimeout = errors.New("readq channel write timeout")
)

var (
	ErrSizeThreshold    = errors.New("size threshold cannot be negative and zero")
	ErrPercentThreshold = errors.New("percent threshold must be between 0 and 100")
	ErrTimeThreshold    = errors.New("time threshold cannot be negative")
)
