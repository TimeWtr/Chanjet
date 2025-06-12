# TurboStream
<div align="center">
<img src="https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white" alt="Go Version">
<img src="https://img.shields.io/badge/license-Apache2.0-blue" alt="License">
<img src="https://img.shields.io/badge/performance-optimized-brightgreen" alt="Performance">
<a title="Tag" target="_blank" href="https://github.com/TimeWtr/TurboStream/tags"><img src="https://img.shields.io/github/v/tag/TimeWtr/TurboStream?color=%23ff8936&logo=fitbit&style=flat-square" /></a>
<br/>
<a title="Doc for Poolx" target="_blank" href="https://pkg.go.dev/github.com/TimeWtr/TurboStream?tab=doc"><img src="https://img.shields.io/badge/go.dev-doc-007d9c?style=flat-square&logo=read-the-docs" /></a>
<img src="https://goreportcard.com/badge/github.com/TimeWtr/TurboStream" alt="Go Report">

[//]: # (<img src="https://img.shields.io/codecov/c/github/TimeWtr/TurboStream?logo=codecov" alt="Coverage">)
</div>


基于Go Slice、Buffer Pool和零拷贝实现的高性能双缓冲通道。

## 📊 Features
🔹 **双写缓冲区设计**：active通道用于实时接收数据，passive通道用于异步处理数据，当active满足
切换逻辑执行通道轮转后，passive转换为active实时写入通道接收数据，active转换成passive异步处理通道。

🔹 **消息全局有序特性**：每条消息的按照顺序写入到active通道中，当通道切换时active转换为passive，
切换程序会为passive分配一个全局唯一单调递增的序列号，并将passive添加到小顶堆中，小顶堆根据序列号
进行排序，消费数据时会按照序列号顺序来消息。

🔹 **灵活的通道切换策略**：

- **默认切换策略（复杂策略）**：
    - 当active通道中数据条数超过容量限制，立即轮转。
    - 当达到一定时间，即当前时间距离上次轮转时间较长，超过了时间窗口周期立即进行轮转，防止因长期没有新数据写入
      导致接收方无法获取通道内数据的问题，也尽可能减少数据丢失的风险。
    - 综合策略，时间因子(40%)、数据条数因子(60%)，根据综合因子判断是否进行通道轮转。
- **时间切换策略**：当当前时间与上次切换的时间差值超过了切换时间窗口时进行切换。
- **数据量切换策略**：当活跃缓冲区中的数据超过通道容量时进行切换。

🔹 **无锁化设计**：双缓冲通道不使用加锁保护通道切换，使用原子状态实现并发安全的通道切换，大大提升了性能。

🔹 **缓冲池设计**：使用缓冲池设计，通道切换时复用池中可用通道，防止出现频繁的通道创建和销毁的开销。

🔹 **异步批量唤醒协调器**：阻塞式读取API时，如果没有可读数据会阻塞等待数据，API注册通知通道到协调器，当通道切换时分批次
通知API读取数据，API采用多路复用机制，当context超时、双缓冲通道关闭时返回错误，收到通知则处理数据。

🔹 **三模式读取API**：提供三种读取数据的方式

- **阻塞式API**：先尝试读取数据，如果没有则阻塞等待，API采用多路复用机制，当context超时、双缓冲通道关闭时返回错误，收到通知则处理数据。
- **批量API**：指定批量读取数据的条数，当有数据需要处理时，批量返回数据（待实现）。
- **回调函数处理**：注册回调函数，当数据需要处理时，调用注册的回调函数处理数据（待实现）。

🔹 **双模式读取机制**：
- **安全读取机制**：
    - 当数据大小小于1024字节时，复制数据并返回；
    - 当数据大于1024字节且小于32KB，数据指针有效，则零拷贝返回数据，反之则复制数据并返回，零拷贝进行生命周期管理(引用计数管理)；
    - 当数据大于32KB，则直接零拷贝返回数据，零拷贝进行生命周期管理(引用计数管理)；
- **零拷贝读取机制**：所有数据全部以零拷贝返回数据，零拷贝进行生命周期管理(引用计数管理)；

🔹 **完善监控的设计**：完善的监控指标设计，支持Prometheus和OpenTelemetry，目前已支持Prometheus指标，抽象批量上报接口，
上报指标数据定时批量刷新到底层指标采集器。


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

## 监控指标说明

### 全局命名空间
所有指标均以 `TurboStream_` 作为命名空间前缀

---

### 写入相关指标
| 指标名称                           | 类型       | 标签/维度       | 描述                                                                 |
|------------------------------------|------------|-----------------|----------------------------------------------------------------------|
| `TurboStream_write_counts_total`       | CounterVec | `result`        | 写入操作总数（标签值：`success` 成功 / `failure` 失败）               |
| `TurboStream_write_sizes_total`        | Counter    | -               | 已写入数据的总字节数（单位：字节）                                   |
| `TurboStream_write_errors_total`       | Counter    | -               | 写入失败的次数（含网络错误、校验失败等场景）                         |

---

### 读取相关指标
| 指标名称                           | 类型       | 标签/维度       | 描述                                                                 |
|------------------------------------|------------|-----------------|----------------------------------------------------------------------|
| `TurboStream_read_counts_total`        | CounterVec | `result`        | 读取操作总数（标签值：`success` 成功 / `failure` 失败）               |
| `TurboStream_read_sizes_total`         | Counter    | -               | 已读取数据的总字节数（单位：字节）                                   |
| `TurboStream_read_errors_total`        | Counter    | -               | 读取失败的次数（含超时、校验失败等场景）                             |

---

### 缓冲区切换指标
| 指标名称                           | 类型       | 描述                                                                 |
|------------------------------------|------------|----------------------------------------------------------------------|
| `TurboStream_switch_counts_total`      | Counter    | 缓冲区切换操作总次数                                                 |
| `TurboStream_switch_latency`           | Histogram  | 切换延迟分布（单位：秒，预设桶边界：[0.001, 0.005, 0.01, 0.05, 0.1]）|
| `TurboStream_skip_switch_counts_total` | Counter    | 定时任务跳过切换的次数（未达到切换条件时计数）                       |

---

### 异步处理指标
| 指标名称                           | 类型       | 描述                                                                 |
|------------------------------------|------------|----------------------------------------------------------------------|
| `TurboStream_async_workers`            | Gauge      | 当前活跃的异步工作协程数量                                           |

---

### 缓冲池指标
| 指标名称                           | 类型       | 描述                                                                 |
|------------------------------------|------------|----------------------------------------------------------------------|
| `TurboStream_pool_alloc_total`         | Counter    | 对象池内存分配次数                                                   |

---

### 通道状态指标
| 指标名称                           | 类型       | 描述                                                                 |
|------------------------------------|------------|----------------------------------------------------------------------|
| `TurboStream_active_channel_data_counts` | Gauge    | 当前活跃通道中未处理的数据条目数量                                   |
| `TurboStream_active_channel_data_sizes`  | Gauge    | 当前活跃通道中未处理的数据总大小（单位：字节）                       |

---

### 指标类型说明
| 类型        | 特性                                                                 |
|-------------|----------------------------------------------------------------------|
| **Counter**   | 只增不减的累积计数器，适用于请求数、错误数等统计                     |
| **Gauge**     | 可任意变化的瞬时值，适用于实时资源用量（如内存、协程数）             |
| **Histogram** | 测量观测值的分布，自动计算分位数，适用于延迟、响应大小等指标         |
| **CounterVec**| 带标签的计数器，支持多维细分统计（如按成功/失败状态分类）            |

---

## 示例 PromQL 查询
```promql
# 计算写入吞吐量（次/秒）
rate(TurboStream_write_counts_total[1m])

# 获取活跃通道数据积压告警（>1MB 持续5分钟）
TurboStream_active_channel_data_sizes > 1e6

# 统计切换延迟的P99值
histogram_quantile(0.99, sum(rate(TurboStream_switch_latency_bucket[5m])) by (le))
```

## 📦 Installation
```bash
go get github.com/TimeWtr/TurboStream
```
## 🧩 用法
待补充！