# gwatch 功能计划书 —— 总体设计思路

> 本文档按模块拆分组织。各分册文档位于 `docs/plan/` 目录下，可独立阅读。

---

## 一、作者整体设计思路

作者将 gwatch 设计为一个"分层递进式"的 API 测试与监控工具，整体思路如下：

1. **入口层（cmd + main）**：用 Cobra 组织两种工作模式（`--test` 一次性测试 / 默认持续监控）。双击运行时保留终端窗口。
2. **启动层（bootstrap）**：按顺序初始化日志 → 配置 → HTTP 客户端 → 邮件 → 清理任务 → 启动提示。
3. **核心业务层**：作者将业务拆为三条独立但共享告警通道的管线：
   - `testcase/executor`：PSV 测试用例执行引擎（一次性）。
   - `monitor`：在 executor 之上包一层"周期性调度 + Worker 协程池"，实现持续接口监控。
   - `scraper`：独立的"远程资源采集器"，基于 config.yaml 而非 PSV 文件，专注于指标提取。
   - `sysmon`：本机系统资源（CPU/内存/磁盘/网络）采集与多级聚合。
4. **统一告警层（email/dispatcher）**：三条管线的告警汇入同一个带缓冲 channel，批量收集 + 冷却抑制 + 分组发送，避免告警风暴。
5. **报告层（report + scheduler）**：日报/周报/月报/年报从 CSV 存储反推生成，本地保存 + 邮件发送。
6. **持久化层（storage）**：全部使用 CSV 文件（SQLite/Postgres 过重），配合"覆盖重写 + 追加"两种写入模式实现增量聚合。
7. **基础设施层（timeutil / vars / httpclient / logger / assert）**：把时区、变量、HTTP、日志、断言这些"横切关注点"抽出来，让业务代码保持纯粹。

作者在设计流程中明显遵循了：**"先搭骨架（目录/接口），再填血肉（实现），最后打通管道（数据/告警/报告链路）"** 的思考顺序。

---

## 二、数据流总图

```
                     ┌──────────────────────────────────────────┐
                     │            cmd/root.go (Cobra)             │
                     │   --test → RunTests                       │
                     │   default → StartMonitorMode              │
                     └──────────────┬───────────────────────────┘
                                    │
    ┌───────────────────────────────┼───────────────────────────────┐
    │                               │                               │
    ▼                               ▼                               ▼
┌───────────────┐            ┌───────────────┐              ┌───────────────┐
│  testcase/    │            │  monitor/     │              │  scraper/     │
│  executor     │            │  worker+ticker│              │  loop         │
└──────┬────────┘            └──────┬────────┘              └──────┬────────┘
       │                            │                             │
       ▼                            ▼                             ▼
  HTTP 断言执行                告警检测 checkAlerts           HTTP+JSONPath 采集
       │                            │                             │
       └─────────────────┬──────────┴─────────────────────────────┘
                         ▼
              ┌────────────────────────┐
              │  email/dispatcher      │
              │  (channel + cooldown) │
              └──────────┬─────────────┘
                         ▼
              ┌────────────────────────┐
              │  storage (CSV 双写)    │
              └──────────┬─────────────┘
                         ▼
              ┌────────────────────────┐
              │  report/scheduler      │
              │  (日/周/月/年报告)     │
              └────────────────────────┘

         另一条独立链路：
    sysmon.StartSystemMonitor
      → CollectMetrics → hourlyAgg
      → 跨小时触发日/月/年聚合
      → CheckAlerts → DispatchSystemAlerts → 同一 dispatcher
```

---

## 三、设计亮点

1. **复用优先**：monitor 基于 executor 构建，scraper 和 executor 共享 resty client，避免重复造轮子。
2. **可插拔**：报告调度、邮件发送均通过函数式依赖注入，便于替换实现。
3. **可观测**：全链路结构化日志 + 控制台彩色输出 + ASCII 图表 + CSV 索引表。
4. **容错**：保存失败仍尝试发送邮件、回填历史、flush 未落盘数据、Windows 双击保留窗口。
5. **可扩展**：`UnifiedAlert` 单一大大降低了新增告警源的接入成本。
6. **用户体验优先**：PSV 表头驱动、ProbeURL 自动生成配置建议、变量 `{{var}}` 脱敏日志。

---

## 四、作者的制作流程复盘

### 阶段一：搭骨架
1. 初始化 `go.mod`、Cobra 命令结构，确定两种模式（test/monitor）。
2. 选定依赖栈：`resty`（HTTP）、`gjson`（JSONPath）、`gopsutil`（系统指标）、`zap`（日志）、`cobra`（CLI）、`doublestar`（glob）。
3. 定义 `TestCase`、`SystemMetric`、`UnifiedAlert`、`MonitorResultRecord` 等核心数据结构。

### 阶段二：打通数据采集管道
4. 实现 `psv.ParseFile`，让"配置驱动"变成"内存对象"。
5. 实现 `testcase.ExecuteTestCase`，跑通"前置 → HTTP → 断言 → 变量提取 → 后置"链路。
6. 实现 `scraper.Scrape` + `ProbeURL`，让用户可以先探测再配置。
7. 实现 `sysmon.CollectMetrics`，基于 gopsutil 采集 6 大系统指标。

### 阶段三：加上"持续"与"并发"
8. 在 `monitor` 里用 Worker 池 + Ticker 把一次性执行变为持续监控。
9. 在 `scraper/loop.go` 里用单协程循环，实现远程目标采集。
10. 在 `sysmon/collectLoop` 里引入 `hourlyAgg` 增量聚合 + 跨级触发。

### 阶段四：统一告警与报告
11. 引入 `UnifiedAlert` + 带缓冲 channel + 30 秒 Ticker + 冷却抑制。
12. 把 API、Scraper、System 三条告警链路汇入同一调度器。
13. 实现 `PeriodicScheduler`，驱动日报/周报/月报/年报生成。

### 阶段五：可观测性与容错
14. 设计 CSV 持久化方案（8 张表 + 索引）。
15. 实现"追加明细 + 覆盖汇总"的双写模式。
16. 启动时回填历史聚合，停止时 flush 未落盘数据。
17. 启动/停止邮件、错误邮件、Windows 双击窗口保留等细节打磨。

### 阶段六：工程化
18. 引入 `timeutil` 统一东八区时间，修复跨时区问题。
19. 引入 `vars` 全局变量模块，支持链路变量传递。
20. 丰富调试日志（Debug 级别结构化日志），便于生产排障。
21. 编写 `docs/*.md` 用户文档与本开发者文档。

---

## 五、分册索引

| 文件 | 覆盖内容 |
|------|----------|
| [00_overview.md](./00_overview.md) | 总体设计思路、数据流总图、设计亮点、制作流程复盘（本文件） |
| [01_bootstrap.md](./01_bootstrap.md) | 启动与入口：main.go、cmd/root.go、bootstrap |
| [02_psv.md](./02_psv.md) | PSV 测试用例解析：数据结构 + 全部解析函数 |
| [03_testcase.md](./03_testcase.md) | 测试执行引擎：executor / runner / assert / vars / httpclient |
| [04_monitor.md](./04_monitor.md) | 接口监控模块：monitor / task / alert |
| [05_scraper.md](./05_scraper.md) | 远程资源采集器：loop 与辅助函数 |
| [06_sysmon.md](./06_sysmon.md) | 系统资源监控：collector / sysmon / alert / storage / chart |
| [07_alert.md](./07_alert.md) | 统一告警调度：dispatcher 全部函数 |
| [08_report.md](./08_report.md) | 报告系统：scheduler + report |
| [09_storage.md](./09_storage.md) | 数据持久化层：CSV 存储全部函数 |
| [10_infrastructure.md](./10_infrastructure.md) | 基础设施层：timeutil / logger / email / util / cleaner |