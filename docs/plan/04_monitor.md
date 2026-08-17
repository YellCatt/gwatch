# 四、接口监控模块（`internal/monitor/`）

## 4.1 `monitor.go` —— 监控模式主入口

| 函数 | 作用 |
|------|------|
| `StartMonitor(testCases)` | 调用 `SetupMonitor` 后阻塞等待 `SIGINT/SIGTERM`，收到信号调用 `StopAllTasks`。 |
| `SetupMonitor(testCases) bool` | 执行全局前置 → 过滤 `MonitorEnabled` 用例 → 创建 `taskChan`/`stopChan` → 启动 `maxWorkers` 个 worker → 为每个用例调用 `startTask` → 异步发送启动报告 → 启动热加载协程。返回 `false` 表示无可监控用例。 |
| `worker(id int)` | 死循环从 `taskChan` 取任务执行，直到 `stopChan` 关闭。 |
| `executeAndMonitorTask(tc)` | 调用 `testcase.ExecuteTestCase` → 包装 `MonitorResult` → `checkAlerts` → 结果追加到内存列表（上限 1000）→ 异步 `persistMonitorResult` → 必要时 `sendAlertEmail`。 |
| `persistMonitorResult(tc, result, monitorResult)` | 依次调用 `storage.RecordMonitorResult`、`storage.UpdateMonitorSummary`、`storage.UpdateAlertSummary`，完成三层落盘。 |
| `filterMonitorCases(testCases) []TestCase` | 过滤 `MonitorEnabled=true` 的用例。 |
| `StopAllTasks()` | 关闭所有任务的 Ticker 和 StopChan，再关闭全局 stopChan，释放 worker。 |
| `GetResults() []MonitorResult` / `GetTaskCount() int` | 线程安全读取内存中监控结果快照与任务数。 |
| `generateAndSendStartupReport(cases, maxWorkers)` | 构建启动信息并保存+邮件发送。 |
| `buildStartupInfo(cases, maxWorkers) *report.StartupInfo` | 将 TestCase 列表转为启动报告结构。 |
| `StartMonitorMode(paths, tags)` | 统一监控中心入口：初始化 HTTP 客户端、CSV、系统监控、采集器、PSV、接口监控、报告调度，注册退出信号并在结束时优雅关闭所有子系统。 |
| `PrintUnifiedBanner(started)` | 打印漂亮的启动横幅，列出已启动的子系统。 |

## 4.2 `task.go` —— 任务调度

| 函数 | 作用 |
|------|------|
| `startTask(tc)` | 加锁检查任务是否已存在 → 创建 `MonitorTask`（含 Ticker 和 StopChan）→ 启动 `scheduleTask` 协程。 |
| `scheduleTask(task)` | 立即执行一次（发送到 `taskChan`），之后按 Ticker 周期持续投递，直到 `StopChan` 关闭。 |
| `removeTask(id)` | 停止 Ticker、关闭 StopChan、从任务表中删除。 |

## 4.3 `alert.go` —— 告警检测与发送

| 函数 | 作用 |
|------|------|
| `checkAlerts(result)` | 依次检查：失败（`!Passed && AlertOnFailure` → `failure`）→ 慢响应（`duration > ResponseThreshold && AlertOnSlow` → `slow`）。 |
| `sendAlertEmail(result)` | 将 `MonitorResult` 转换为 `UnifiedAlert`（来源 `SourceAPI`、失败对应 `CRITICAL`、慢响应对应 `WARNING`），调用 `email.DispatchAlert`。 |

### 作者思考

将"持续监控"实现为"测试执行引擎 + 调度外壳"，这样既复用了一次性测试的所有能力（断言、变量、前置/后置），又通过 Worker 池和 Ticker 实现了周期性执行，避免重复实现 HTTP 请求逻辑。