# Chanjet
<div align="center">
<a title="Build Status" target="_blank" href="https://github.com/TimeWtr/Chanjet/actions?query=workflow%3ATests"><img>
<img src="https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white" alt="Go Version">
<img src="https://img.shields.io/badge/license-Apache2.0-blue" alt="License">
<img src="https://img.shields.io/badge/performance-optimized-brightgreen" alt="Performance">
<a title="Tag" target="_blank" href="https://github.com/TimeWtr/Chanjet/tags"><img src="https://img.shields.io/github/v/tag/TimeWtr/Chanjet?color=%23ff8936&logo=fitbit&style=flat-square" /></a>
<br/>
<a title="Doc for Poolx" target="_blank" href="https://pkg.go.dev/github.com/TimeWtr/Chanjet?tab=doc"><img src="https://img.shields.io/badge/go.dev-doc-007d9c?style=flat-square&logo=read-the-docs" /></a>
<img src="https://goreportcard.com/badge/github.com/TimeWtr/Chanjet" alt="Go Report">
</div>

High-Performance double-buffered channel using Go slice, buffer pool and zero-copy.

## 📊 Features
🔹 **Dual Write Buffer Design:**：Active channel receives data in real-time, while passive channel processes data asynchronously.
When switching conditions are met, the roles rotate: passive becomes active for receiving data, while active becomes passive for asynchronous processing.

🔹 **Global Message Ordering Guarantee**：Messages are sequentially written to the active channel. During channel rotation,
the passive channel is assigned a globally unique, monotonically increasing sequence number and added to a min-heap sorted by sequence numbers. Consumers process messages in strict sequence order.

🔹 **Flexible Channel Rotation Strategies**：

- **Default Strategy (Composite)**： 
  - Immediate rotation when active channel exceeds capacity.
    Time-based rotation when exceeding configurable time windows.
    Hybrid decision using 40% time factor and 60% data volume factor.
- **Time-Based Strategy**：Rotate when time since last rotation exceeds threshold.
- **Data-Volume Strategy**：Rotate when active buffer occupancy exceeds defined percentage.

🔹 **Lock-Free Implementation**：Atomic operations ensure safe channel rotation without mutex overhead, significantly boosting performance.

🔹 **Buffer Pool Optimization**：Reuses channels from pool during rotation, eliminating allocation/deallocation overhead.

🔹 **Async Batch Wake Coordinator**：For blocking read APIs, a notification coordinator registers waiters and performs batched notifications during channel rotation using multiplexed notification channels.

🔹 **Tri-Modal Reader API**：

- **Blocking API**：Try to read the data first. If there is no data, block and wait. The API adopts a multiplexing mechanism. When the context times out or the double-buffered channel is closed, an error is returned. When a notification is received, the data is processed.
- **Batch API**：Specify the number of data to be read in batches. When there is data to be processed, return the data in batches (to be implemented).
- **Callback function processing**：Register a callback function. When data needs to be processed, call the registered callback function to process the data (to be implemented).

🔹 **Dual-mode reading mechanism**：
- **Safe reading mechanism**：
  - When the data size is less than 1024 bytes, copy the data and return;
  - When the data is larger than 1024 bytes and smaller than 32KB, and the data pointer is valid, the data is returned with zero copy. Otherwise, the data is copied and returned, and zero copy is used for life cycle management (reference count management).
  - When the data is larger than 32KB, the data is directly returned with zero copy, and life cycle management (reference count management) is performed with zero copy.
- **Zero-copy read mechanism**：All data is returned with zero copy, and zero copy is used for life cycle management (reference count management).

🔹 **Improve the design of monitoring**：Complete monitoring indicator design, supporting Prometheus and OpenTelemetry. Currently, Prometheus indicators are supported, abstract batch reporting interface, and the reported indicator data is regularly refreshed in batches to the underlying indicator collector.


## 🚀 Performance
- **System architecture**: darwin/arm64
- **processor**: Apple M4

### 128B Test results
```text
# Zero copy mode
BenchmarkBlockingRead_Throughput_Zero_Copy_128Bytes-10    	 4261556	       293.7 ns/op
BenchmarkBlockingRead_Throughput_Zero_Copy_128Bytes-10    	 4243588	       278.8 ns/op
BenchmarkBlockingRead_Throughput_Zero_Copy_128Bytes-10    	 4145382	       282.2 ns/op
BenchmarkBlockingRead_Throughput_Zero_Copy_128Bytes-10    	 4110026	       284.3 ns/op

# Safe read mode
BenchmarkBlockingRead_Throughput_Safe_Read_128Bytes-10    	 3553357	       335.4 ns/op
BenchmarkBlockingRead_Throughput_Safe_Read_128Bytes-10    	 3291746	       335.5 ns/op
BenchmarkBlockingRead_Throughput_Safe_Read_128Bytes-10    	 3459871	       333.0 ns/op
BenchmarkBlockingRead_Throughput_Safe_Read_128Bytes-10    	 3500275	       335.0 ns/op
```

### 64KB Test results
```text
# Zero copy mode
BenchmarkBlockingRead_Throughput_Zero_Copy_64KB_10    	 9410623	       129.3 ns/op
BenchmarkBlockingRead_Throughput_Zero_Copy_64KB_10    	 9096145	       129.4 ns/op
BenchmarkBlockingRead_Throughput_Zero_Copy_64KB_10    	 9354147	       129.4 ns/op
BenchmarkBlockingRead_Throughput_Zero_Copy_64KB_10    	 9155121	       129.9 ns/op

# Safe read mode
BenchmarkBlockingRead_Throughput_Safe_Read_64KB-10    	 9770590	       124.2 ns/op
BenchmarkBlockingRead_Throughput_Safe_Read_64KB-10    	 9383720	       123.8 ns/op
BenchmarkBlockingRead_Throughput_Safe_Read_64KB-10    	 9513310	       123.7 ns/op
BenchmarkBlockingRead_Throughput_Safe_Read_64KB-10    	 9623844	       125.2 ns/op
```

## Monitoring indicator description

### global namespace
All indicators are prefixed with `Chanjet_` as the namespace.

---

### Write relevant indicators
| Indicator name                          | Type       | Label/Dimension | Describe                                |
|------------------------------------|------------|-----------------|-----------------------------------------|
| `Chanjet_write_counts_total`       | CounterVec | `result`        | Total number of write operations (label value: `success` success / `failure` failure)|
| `Chanjet_write_sizes_total`        | Counter    | -               | The total number of bytes of data written (unit: bytes)                       |
| `Chanjet_write_errors_total`       | Counter    | -               | Number of write failures (including network errors, verification failures, etc.)                  |

---

### Read related indicators
| Indicator name                         | Type       | Label/Dimension | Describe                                                                             |
|------------------------------------|------------|-----------------|--------------------------------------------------------------------------------------|
| `Chanjet_read_counts_total`        | CounterVec | `result`        | Total number of read operations (label value: `success` success / `failure` failure) |
| `Chanjet_read_sizes_total`         | Counter    | -               | The total number of bytes of data read (unit: bytes)                                 |
| `Chanjet_read_errors_total`        | Counter    | -               | Number of read failures (including timeouts, verification failures, etc.)                                                                 |

---

### Buffer switching indicator
| Indicator name                        | Type       | Describe                                           |
|------------------------------------|------------|----------------------------------------------------|
| `Chanjet_switch_counts_total`      | Counter    | Total number of buffer switch operations                                        |
| `Chanjet_switch_latency`           | Histogram  | Switching delay distribution (unit: seconds, preset bucket boundaries: [0.001, 0.005, 0.01, 0.05, 0.1]) |
| `Chanjet_skip_switch_counts_total` | Counter    | The number of times the scheduled task skips switching (counted when the switching condition is not met)                            |

---

### Asynchronous processing metrics
| Indicator name                          | Type       | Describe                                                           |
|------------------------------------|------------|-------------------------------------------------------------|
| `Chanjet_async_workers`            | Gauge      | The number of currently active asynchronous work coroutines |

---

### buffer pool metrics
| Indicator name                           | Type    | Describe                                                                  |
|------------------------------------|---------|----------------------------------------------------------------------|
| `Chanjet_pool_alloc_total`         | Counter | Object pool memory allocation times                                                  |

---

### Channel status indicator
| Indicator name                           | Type  | Describe                                                                 |
|------------------------------------|-------|----------------------------------------------------------------------|
| `Chanjet_active_channel_data_counts` | Gauge | The number of unprocessed data items in the current active channel                               |
| `Chanjet_active_channel_data_sizes`  | Gauge | The total size of unprocessed data in the current active channel (unit: bytes)                       |

---

### Indicator type description
| Type           | Characteristic                 |
|----------------|--------------------------------|
| **Counter**    | A cumulative counter that only increases but never decreases, suitable for statistics such as the number of requests and errors      |
| **Gauge**      | Instantaneous values that can change arbitrarily, suitable for real-time resource usage (such as memory, number of coroutines)   |
| **Histogram**  | Measures the distribution of observed values and automatically calculates quantiles for metrics such as latency and response size |
| **CounterVec** | Counters with labels, supporting multi-dimensional segmentation statistics (such as classification by success/failure status)  |

---


## 📦 Installation
```bash
go get github.com/TimeWtr/chanjet
```
## 🧩 Usage
To be added！