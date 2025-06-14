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

package monitor

import (
	"time"

	"github.com/shirou/gopsutil/v3/net"
)

type Meta struct {
	LastCollectTime  time.Time `json:"lastCollectTime"`
	SuccessCount     int64     `json:"successCount,omitempty"`
	ErrCount         int       `json:"errCount,omitempty"`
	AverageTimeTaken int64     `json:"averageTimeTaken,omitempty"`
	TimeTakenQueue   []int64   `json:"timeTakenQueue,omitempty"`
}

type Metrics struct {
	Timestamp int64         `json:"timestamp"`
	CPU       CPUStates     `json:"cpu"`
	Memory    MemoryStates  `json:"memory"`
	Disk      DiskStates    `json:"disk"`
	Runtime   RuntimeStates `json:"runtime"`
	Network   NetworkStates `json:"network"`
}

type CPUStates struct {
	Usage  float64 `json:"usage"`
	User   float64 `json:"user"`
	System float64 `json:"system"`
	Idle   float64 `json:"idle"`
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

type MemoryStates struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"usedPercent"`
	Cached      uint64  `json:"cached"`
	Buffers     uint64  `json:"buffers"`
}

type NetworkStates struct {
	Connections      []net.ConnectionStat      `json:"connections"`
	IOCounters       []net.IOCountersStat      `json:"ioCounters"`
	TotalBytesSent   uint64                    `json:"totalBytesSent"`
	TotalBytesRecv   uint64                    `json:"totalBytesRecv"`
	TotalPacketsSent uint64                    `json:"totalPacketsSent"`
	TotalPacketsRecv uint64                    `json:"totalPacketsRecv"`
	InterfaceStats   map[string]InterfaceStats `json:"interfaceStats"`
}

type InterfaceStats struct {
	BytesSent   uint64 `json:"bytesSent"`
	BytesRecv   uint64 `json:"bytesRecv"`
	PacketsSent uint64 `json:"packetsSent"`
	PacketsRecv uint64 `json:"packetsRecv"`
}

type RuntimeStates struct {
	Goroutines uint64 `json:"goroutines"`
	HeapAlloc  uint64 `json:"heapAlloc"`
	StackAlloc uint64 `json:"stackAlloc"`
	GCNums     uint64 `json:"gcNums"`
	GCPause    uint64 `json:"gcPause"`
}

type DiskStates struct {
	Timestamp  int64                  `json:"timestamp"`
	Partitions []PartitionState       `json:"partitions"`
	TotalIO    DiskIOStats            `json:"totalIo"`
	PerDiskIO  map[string]DiskIOStats `json:"perDiskIo"`
	Errors     []string               `json:"errors,omitempty"`
}

type PartitionState struct {
	Device      string  `json:"device"`
	MountPoint  string  `json:"mountPoint"`
	FsType      string  `json:"fsType"`
	Options     string  `json:"options"`
	Total       uint64  `json:"totalBytes"`
	Used        uint64  `json:"usedBytes"`
	Free        uint64  `json:"freeBytes"`
	UsedPercent float64 `json:"usedPercent"`
	InodesTotal uint64  `json:"inodesTotal"`
	InodesUsed  uint64  `json:"inodesUsed"`
	InodesFree  uint64  `json:"inodesFree"`
	ReadOnly    bool    `json:"readOnly"`
}

type DiskIOStats struct {
	DiskName           string  `json:"diskName"`
	ReadCountPerSec    float64 `json:"readCountPerSec"`
	WriteCountPerSec   float64 `json:"writeCountPerSec"`
	ReadBytesPerSec    float64 `json:"readBytesPerSec"`
	WriteBytesPerSec   float64 `json:"writeBytesPerSec"`
	ReadTimePerSec     float64 `json:"readTimePerSec"`
	WriteTimePerSec    float64 `json:"writeTimePerSec"`
	UtilizationPercent float64 `json:"utilizationPercent"`
}
