# 架构设计

## 系统架构

```
┌───────────────────────────────────────────────────────────────────┐
│                          gwatch 主程序                              │
│                                                                   │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐               │
│  │  PSV 测试    │  │  数据采集    │  │  系统监控    │               │
│  │  (testcase)  │  │  (scraper)   │  │  (sysmon)   │               │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘               │
│         │                 │                 │                       │
│         └─────────────────┼─────────────────┘                       │
│                           ▼                                         │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                  统一告警调度 (email/dispatcher)              │   │
│  │         api_monitor / scraper / system_monitor               │   │
│  └─────────────────────────┬───────────────────────────────────┘   │
│                            ▼                                       │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                  报告系统 (report/scheduler)                 │   │
│  │    日报 / 周报 / 月报 / 年报 / 启动报告                      │   │
│  └─────────────────────────┬───────────────────────────────────┘   │
│                            ▼                                       │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │              数据持久化 (storage)                            │   │
│  │    执行记录 / 监控结果 / 指标数据 / 告警记录 / 系统指标       │   │
│  └─────────────────────────┬───────────────────────────────────┘   │
│                            ▼                                       │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │           基础设施层                                         │   │
│  │  日志(logger) / 配置(config) / 变量(vars) / HTTP客户端        │   │
│  │  清理(cleaner) / 调度(scheduler) / 时间(timeutil)            │   │
│  └─────────────────────────────────────────────────────────────┘   │
└───────────────────────────────────────────────────────────────────┘
```

## 目录结构

```
internal/
├── assert/          # 断言引擎（JSON匹配、正则匹配）
├── bootstrap/       # 应用启动初始化
├── cleaner/         # 自动清理模块
├── email/           # 邮件发送与告警调度
│   └── templates/   # 邮件模板
├── httpclient/      # HTTP 客户端封装
├── logger/          # 日志模块（基于 zap）
├── monitor/         # 接口监控调度
├── psv/             # PSV 文件解析
├── report/          # 报告生成与调度
│   └── templates/   # 报告模板
├── scheduler/       # 周期调度器
├── scraper/         # 数据采集器
├── storage/         # 数据持久化（CSV）
├── sysmon/          # 系统资源监控
├── testcase/        # 测试用例执行引擎
├── timeutil/        # 东八区时间工具
└── vars/            # 全局变量管理
```

## 核心模块说明

### 1. PSV 解析器 (psv)

负责解析 `.psv` 和 `.csv` 文件，将每行数据转换为 `TestCase` 结构体。支持 glob 模式匹配多个文件。

**关键文件**：`internal/psv/psv.go`

### 2. 测试执行引擎 (testcase)

负责执行单个测试用例，包括：
- 前置条件检查
- 变量替换（URL、Headers、Body 中的 `{{var}}`）
- HTTP 请求发送
- 响应断言（状态码、JSON 匹配、正则匹配）
- 变量提取
- 后置条件执行
- 结果记录

**关键文件**：`internal/testcase/executor.go`

### 3. 接口监控 (monitor)

在测试执行引擎之上构建的持续监控层，提供：
- Worker 协程池并发执行
- 按周期定时调度
- 热加载（新增/修改/删除用例）
- 告警检测与触发

**关键文件**：`internal/monitor/monitor.go`、`internal/monitor/alert.go`

### 4. 数据采集器 (scraper)

独立于 PSV 测试的另一条监控链路：
- 从 config.yaml 读取目标配置
- 使用 JSONPath 提取指标值
- 阈值比较与告警
- 数据记录

**关键文件**：`internal/scraper/scraper.go`、`internal/scraper/loop.go`

### 5. 系统监控 (sysmon)

采集本机系统指标：
- CPU、内存、磁盘使用率
- 网络上/下行速度
- 磁盘读写速度
- ASCII 图表生成
- 多级数据聚合（小时/日/月/年）

**关键文件**：`internal/sysmon/collector.go`、`internal/sysmon/sysmon.go`

### 6. 告警调度 (email/dispatcher)

统一的告警分发中心：
- 三类告警源合并
- 通道缓冲（200 条）
- 30 秒批量收集
- 冷却抑制策略
- 邮件模板渲染

**关键文件**：`internal/email/dispatcher.go`

### 7. 报告系统 (report)

按周期生成统计报告：
- 日报：每天定时
- 周报：每周一
- 月报：每月 1 日
- 年报：每年 1 月 1 日
- 启动报告：启动时生成

**关键文件**：`internal/report/scheduler.go`、`internal/report/report_generator.go`

### 8. 数据持久化 (storage)

所有监控数据通过 CSV 文件持久化：

| 文件 | 内容 |
|------|------|
| `execution_history.csv` | 测试用例执行记录 |
| `monitor_results.csv` | 接口监控结果 |
| `scraper_metrics.csv` | 采集器指标数据 |
| `alert_history.csv` | 告警历史记录 |
| `sysmon_*.csv` | 系统监控指标（原始/小时/日/月/年） |

**关键文件**：`internal/storage/init.go`、`internal/storage/record_*.go`

### 9. 时间工具 (timeutil)

统一使用东八区（Asia/Shanghai）时间：
- `Now()`：获取当前东八区时间
- `FormatDateTime()`：格式化时间输出
- 所有业务模块的时间戳均使用此工具

**关键文件**：`internal/timeutil/timeutil.go`

## 数据流

### 接口监控数据流

```
PSV 文件 → psv.ParseFile() → []TestCase
    ↓
monitor.SetupMonitor() → 过滤 monitor_enabled
    ↓
monitor.worker() ← taskChan ← Ticker
    ↓
testcase.ExecuteTestCase() → HTTP 请求 + 断言
    ↓
monitor.checkAlerts() → 检测异常
    ↓
email.DispatchAlert() → 告警通道
    ↓
storage.RecordMonitorResult() → CSV 持久化
    ↓
report.GenerateReport() → 统计报告
```

### 数据采集数据流

```
config.yaml → scraper.StartLoop()
    ↓
scraper.Scrape() → HTTP 请求 + JSONPath 提取
    ↓
告警检测（compare_op + threshold）
    ↓
email.DispatchAlert() → 告警通道
    ↓
storage.RecordScraperResult() → CSV 持久化
```

## 并发模型

- **接口监控**：Worker 协程池（`max_workers`），通过 channel 分发任务
- **数据采集**：串行执行所有目标（单协程循环）
- **告警调度**：单协程消费告警通道，30 秒批量处理
- **报告生成**：定时调度器触发，同步执行
- **热加载**：30 秒扫描一次文件变化