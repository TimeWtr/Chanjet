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
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

const (
	percentBase  = 100
	milliseconds = 1000
)

type CPUCollector struct{}

func newCPUCollector() *CPUCollector {
	return &CPUCollector{}
}

func (c *CPUCollector) Collect() CPUStates {
	percs, err := cpu.Percent(0, false)
	if err != nil {
		return CPUStates{}
	}

	cpuStats := CPUStates{}
	cpuStats.Usage = percs[0]

	timeStates, err := cpu.Times(false)
	if err == nil && len(timeStates) > 0 {
		state := timeStates[0]
		total := state.User + state.System + state.Idle
		if total > 0 {
			cpuStats.User = state.User / total * percentBase
			cpuStats.System = state.System / total * percentBase
			cpuStats.Idle = state.Idle / total * percentBase
		}
	}

	avg, err := load.Avg()
	if err == nil && avg != nil {
		cpuStats.Load1 = avg.Load1
		cpuStats.Load5 = avg.Load5
		cpuStats.Load15 = avg.Load15
	}

	return cpuStats
}

type RuntimesCollector struct{}

func newRuntimesCollector() *RuntimesCollector {
	return &RuntimesCollector{}
}

func (r *RuntimesCollector) Collect() RuntimeStates {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return RuntimeStates{
		Goroutines: uint64(runtime.NumGoroutine()),
		HeapAlloc:  memStats.HeapAlloc,
		StackAlloc: memStats.StackInuse,
		GCNums:     uint64(memStats.NumGC),
		GCPause: func() uint64 {
			if memStats.NumGC == 0 {
				return 0
			}
			idx := (memStats.NumGC - 1) % uint32(len(memStats.PauseNs))
			return memStats.PauseNs[idx]
		}(),
	}
}

type MemoryCollector struct{}

func newMemoryCollector() *MemoryCollector {
	return &MemoryCollector{}
}

func (m *MemoryCollector) Collect() MemoryStates {
	memStates := MemoryStates{}
	vms, err := mem.VirtualMemory()
	if err == nil && vms != nil {
		memStates.Total = vms.Total
		memStates.Used = vms.Used
		memStates.Free = vms.Free
		memStates.UsedPercent = vms.UsedPercent
		memStates.Cached = vms.Cached
		memStates.Buffers = vms.Buffers
	}

	return memStates
}

type NetworkCollector struct{}

func newNetworkCollector() *NetworkCollector {
	return &NetworkCollector{}
}

func (n *NetworkCollector) Collect() NetworkStates {
	connections, _ := net.Connections("all")
	ioCounters, _ := net.IOCounters(true)

	var totalBytesSent, totalBytesRecv, totalPacketsSent, totalPacketsRecv uint64
	interfaceStats := make(map[string]InterfaceStats)

	for _, counter := range ioCounters {
		totalBytesSent += counter.BytesSent
		totalBytesRecv += counter.BytesRecv
		totalPacketsSent += counter.PacketsSent
		totalPacketsRecv += counter.PacketsRecv

		interfaceStats[counter.Name] = InterfaceStats{
			BytesSent:   counter.BytesSent,
			BytesRecv:   counter.BytesRecv,
			PacketsSent: counter.PacketsSent,
			PacketsRecv: counter.PacketsRecv,
		}
	}

	return NetworkStates{
		Connections:      connections,
		IOCounters:       ioCounters,
		TotalBytesSent:   totalBytesSent,
		TotalBytesRecv:   totalBytesRecv,
		TotalPacketsSent: totalPacketsSent,
		TotalPacketsRecv: totalPacketsRecv,
		InterfaceStats:   interfaceStats,
	}
}

type DiskCollector struct {
	prevIOCounters map[string]disk.IOCountersStat
}

func newDiskCollector() *DiskCollector {
	return &DiskCollector{
		prevIOCounters: make(map[string]disk.IOCountersStat),
	}
}

func (d *DiskCollector) Collect(lastCollection time.Time) DiskStates {
	if lastCollection.IsZero() {
		currentIOCounters, _ := disk.IOCounters()
		d.prevIOCounters = currentIOCounters
		return DiskStates{Timestamp: time.Now().Unix()}
	}

	now := time.Now()
	elapsed := now.Sub(lastCollection).Seconds()
	if elapsed <= 0 {
		elapsed = 1.0
	}

	states := DiskStates{
		Timestamp: now.Unix(),
		PerDiskIO: make(map[string]DiskIOStats),
	}

	partitions, _ := disk.Partitions(true)

	for _, partition := range partitions {
		if d.shouldSkipPartition(partition) {
			continue
		}

		usage, err := disk.Usage(partition.Mountpoint)
		if err != nil {
			states.Errors = append(states.Errors, "Usage error ["+partition.Mountpoint+"]: "+err.Error())
			continue
		}

		isReadOnly := false
		for _, opt := range partition.Opts {
			if opt == "ro" {
				isReadOnly = true
				break
			}
		}

		states.Partitions = append(states.Partitions, PartitionState{
			Device:      partition.Device,
			MountPoint:  partition.Mountpoint,
			FsType:      partition.Fstype,
			Options:     strings.Join(partition.Opts, ","),
			Total:       usage.Total,
			Used:        usage.Used,
			Free:        usage.Free,
			UsedPercent: usage.UsedPercent,
			InodesTotal: usage.InodesTotal,
			InodesUsed:  usage.InodesUsed,
			InodesFree:  usage.InodesFree,
			ReadOnly:    isReadOnly,
		})
	}

	currentIOCounters, err := disk.IOCounters()
	if err != nil {
		states.Errors = append(states.Errors, "IO Counters error: "+err.Error())
		return states
	}

	for diskName := range currentIOCounters {
		counters := currentIOCounters[diskName]
		if strings.Contains(diskName, "loop") ||
			strings.HasPrefix(diskName, "dm-") ||
			strings.Contains(diskName, "nvme") {
			continue
		}

		if prev, exists := d.prevIOCounters[diskName]; exists {
			delta := func(current, prev uint64) float64 {
				if current < prev {
					return float64(current) / elapsed
				}
				return float64(current-prev) / elapsed
			}

			utilization := percentBase * float64(counters.IoTime-prev.IoTime) / (elapsed * milliseconds)

			states.PerDiskIO[diskName] = DiskIOStats{
				DiskName:           diskName,
				ReadCountPerSec:    delta(counters.ReadCount, prev.ReadCount),
				WriteCountPerSec:   delta(counters.WriteCount, prev.WriteCount),
				ReadBytesPerSec:    delta(counters.ReadBytes, prev.ReadBytes),
				WriteBytesPerSec:   delta(counters.WriteBytes, prev.WriteBytes),
				ReadTimePerSec:     delta(counters.ReadTime, prev.ReadTime),
				WriteTimePerSec:    delta(counters.WriteTime, prev.WriteTime),
				UtilizationPercent: utilization,
			}
		}
	}
	d.prevIOCounters = currentIOCounters

	return states
}

func (d *DiskCollector) shouldSkipPartition(p disk.PartitionStat) bool {
	skipFsTypes := map[string]bool{
		"tmpfs":       true,
		"devtmpfs":    true,
		"squashfs":    true,
		"overlay":     true,
		"autofs":      true,
		"cgroup":      true,
		"cgroup2":     true,
		"tracefs":     true,
		"debugfs":     true,
		"pstore":      true,
		"bpf":         true,
		"securityfs":  true,
		"configfs":    true,
		"fusectl":     true,
		"hugetlbfs":   true,
		"mqueue":      true,
		"binfmt_misc": true,
	}
	if _, exists := skipFsTypes[p.Fstype]; exists {
		return true
	}

	skipMounts := map[string]bool{
		"/dev":                     true,
		"/proc":                    true,
		"/sys":                     true,
		"/run":                     true,
		"/snap":                    true,
		"/sys/kernel":              true,
		"/boot/efi":                true,
		"/sys/fs/cgroup":           true,
		"/sys/fs/fuse":             true,
		"/sys/fs/pstore":           true,
		"/sys/fs/bpf":              true,
		"/sys/kernel/debug":        true,
		"/sys/kernel/tracing":      true,
		"/proc/sys/fs/binfmt_misc": true,
	}
	if _, exists := skipMounts[p.Mountpoint]; exists {
		return true
	}

	for _, opt := range p.Opts {
		if opt == "ro" {
			return true
		}
	}

	if strings.Contains(p.Mountpoint, "/var/lib/docker") ||
		strings.Contains(p.Mountpoint, "/var/lib/kubelet") ||
		strings.Contains(p.Mountpoint, "/var/lib/lxc") ||
		strings.Contains(p.Mountpoint, "/var/lib/lxd") ||
		strings.Contains(p.Mountpoint, "/var/lib/containers") ||
		strings.HasPrefix(p.Device, "gvfs") ||
		strings.Contains(p.Device, "ram") {
		return true
	}

	return false
}
