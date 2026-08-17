# gwatch 项目缺陷清单

> 本文档记录对当前代码库（main 分支）全量审查发现的缺陷与不足，按严重程度分级。
> 生成日期：2026-08-17

## 目录

- [一、高危问题](#一高危问题)
- [二、中危问题](#二中危问题)
- [三、低危问题](#三低危问题)
- [四、修复优先级建议](#四修复优先级建议)

---

## 一、高危问题

> 可致进程崩溃、数据错误、安全事故，应优先修复。

### H1. 测试模式 HTTP 客户端未初始化，执行任何用例必然 panic

- **位置**：`cmd/root.go` → `internal/testcase/runner.go` `RunTests()` → `internal/testcase/executor.go:191`
- **问题**：`RunTests` 全程未调用 `httpclient.InitClient()`（仅 `monitor.StartMonitorMode` 和无人使用的遗留函数 `testcase.RunAll` 调用）。`ExecuteTestCase` 中 `httpclient.Client.R()` 直接 nil 指针解引用，普通运行模式必崩。

### H2. 流式断言失败结果被强制改写为"通过"

- **位置**：`internal/testcase/executor.go:259-305`
- **问题**：`executeStreamAssert` 返回的结果即使 `Passed=false`，代码仍无条件执行 `result.Passed = true`。流式模式下的失败用例在结果、报告、监控告警中全部被记为成功。同时该分支重建 result，丢弃了 `ProcessedURL`、`ActualStatus`、`RequestHeaders/Body`，且不校验 `ExpectedStatus`。

### H3. `vars.Replace` 无锁读 map，并发下进程级 fatal

- **位置**：`internal/vars/vars.go:74-84`
- **问题**：`Replace` 在正则回调中直接读 `vars[key]`，未持有 `varsMu`；而 `Set/Delete` 在其他 goroutine（监控 worker、变量提取）中加锁写同一 map。监控模式多 worker 并发时触发 `fatal error: concurrent map read and map write`，进程崩溃且无法 recover。

### H4. 监控间隔为 0 时 `time.NewTicker` panic

- **位置**：`internal/monitor/task.go:26`；根源在 `config/defaults.go`（未给 `DefaultInterval` 兜底）
- **问题**：`monitor.default_interval` 缺省或为 0/负数（PSV 行内也可显式写 0）时，`time.NewTicker(0)` 直接 panic：`non-positive interval for NewTicker`。`startTask` 无任何防御。

### H5. Ctrl+C 后进程无法退出：报告调度器永不停止

- **位置**：`internal/scheduler/scheduler.go:48`（无 stop channel 的无限 for + `time.Sleep(duration)` 一睡最长 24 小时）；`internal/report/scheduler.go:30`；`internal/monitor/monitor.go:335-358`
- **问题**：`StartMonitorMode` 把 `ReportScheduler.Start()` 加入 `wg`，退出时只停 monitor/scraper/sysmon，报告调度 goroutine 永不退出，`wg.Wait()` 永久阻塞。用户按 Ctrl+C 后程序挂死，只能强杀。

### H6. 告警调度器 close 后再 send → panic

- **位置**：`internal/email/dispatcher.go:262-270 CloseDispatcher` 与 `48-65 DispatchAlert`；调用顺序见 `internal/monitor/monitor.go:352-355`
- **问题**：退出流程先 `email.CloseDispatcher()`（close alertChan），再停各监控模块。仍在运行的 worker/scraper/sysmon 继续 `DispatchAlert` → 向已关闭 channel 发送 → panic。且 `dispatcherRunning` 置 false 后 `DispatchAlert` 会重启 dispatcher goroutine 读已关闭 channel，逻辑进一步错乱。

### H7. SMTP 全程硬编码 `InsecureSkipVerify: true`

- **位置**：`internal/email/email.go:78-81`；另见 `internal/scraper/loop.go:319`（probe 命令对所有 HTTPS 无条件跳过验证）
- **问题**：TLS 配置跳过证书校验且不可配置，SMTP 凭据在 465 端口以可被中间人攻击的方式传输。

### H8. 明文 SMTP 授权码提交进仓库

- **位置**：`config/config_dev.yaml:145-148`
- **问题**：真实 QQ 邮箱地址与授权码硬编码并入库，等同于邮箱账号失陷。应立即吊销该授权码，并改用环境变量/密钥管理注入。

### H9. 周报/月报/年报统计区间取错周期，报告基本为空

- **位置**：`internal/report/scheduler.go:42-82`
- **问题**：周一触发周报时统计区间是**本周**（周一 0 点 → 下周一），即只统计了刚开始的几个小时加未来时间；月报、年报同理。正确做法是统计**上一个完整周期**（日报取昨天是对的，可对照）。

### H10. `fixRegexEscapes` 无差别破坏合法正则

- **位置**：`internal/assert/assert.go:344-374`
- **问题**：对 pattern 中所有未转义的 `d D w W s S b B n t r` 字母强行前加 `\`。任何含普通英文文本的正则（如 `{{regex:test}}` → `\ttes\t`）匹配语义完全错误。`validateRegexPattern` 还会擅自往用户 pattern 里插入 `.`，改变匹配语义。

### H11. 告警/报告时间显示偏移 8 小时（UTC 误解析）

- **位置**：`internal/report/report_generator.go:35-36`；同类见 `internal/storage/record_monitor.go:81`、`record_scraper.go:75`、`record_sysalert.go` 各 `GetXxxByPeriod`
- **问题**：存储写入的是东八区墙钟字符串，读回却用 `time.Parse`（按 UTC 解析），报告再转东八区 → 首/末次告警时间整体 +8 小时；跨天归属也错误。

### H12. 日志明文泄露全部变量/密钥

- **位置**：`internal/bootstrap/bootstrap.go:36`（`zap.Any("vars", vars.GetAll())` 之类的全量变量输出）
- **问题**：启动时将全部自定义变量（通常含 api_key、token 等敏感信息）明文写入日志文件。

---

## 二、中危问题

| # | 问题 | 位置 |
|---|------|------|
| M1 | Kbps 单位换算错误：`threshold * 1024 / 8`，Kbps 阈值被放大约 1024 倍，网络告警失效 | `internal/util/format.go:31` |
| M2 | `disk.Usage("/")` 在 Windows 上可能失败且错误被静默吞掉，磁盘指标恒为 0；`CollectMetrics` 返回的 error 永远为 nil | `internal/sysmon/collector.go:33,48` |
| M3 | 采集循环固定 `Sleep(10s)`，完全忽略每个 target 的 `interval` 配置，配置项形同虚设 | `internal/scraper/loop.go:294` |
| M4 | `StopLoop` 对 `stopChan`/`running` 无锁读写，极端时序可 close 已关闭 channel panic；同类无锁 running 标志也见于 cleaner | `scraper/loop.go:22-44,299`；`cleaner/cleaner.go:50-93` |
| M5 | PSV 首行若是注释/空行，`lineNum==1` 被跳过导致真正表头被当数据行解析，整列用例静默丢失 | `internal/psv/psv.go:149-162` |
| M6 | `bufio.Scanner` 默认 64KB 行上限，PSV 中含大 JSON 的超长行导致整个文件解析失败 | `internal/psv/psv.go:143` |
| M7 | 前置条件无循环依赖检测，pre 相互引用会无限递归直至栈溢出 | `internal/testcase/executor.go:24-43` |
| M8 | cleaner 模块从未被实例化启动（全项目无 `NewCleaner` 调用），`cleaner.enabled` 默认 true 纯属误导；且若启用，默认 `*.csv` + data_dir 会删存储 CSV | `internal/cleaner/`，`cmd/root.go` |
| M9 | `StopAllTasks` 后 `scheduleTask` 可能阻塞在 `taskChan<-`（channel 置 nil、worker 已退出），goroutine 泄漏 | `internal/monitor/task.go:40-45`，`monitor.go:216` |
| M10 | 汇总类存储每条记录都全量读+重写整个 CSV（写放大 O(n²)），且 `os.Create` 截断写入非原子，中途崩溃丢全部汇总数据 | `storage/record_monitor.go:213`，`storage/init.go:450` |
| M11 | `Truncate(24*time.Hour)` 按 UTC 对齐，东八区日/月/年聚合窗口错位 8 小时（独立于 H11 的缺陷） | `internal/sysmon/sysmon.go:155-161` |

---

## 三、低危问题

### 3.1 采集器（scraper）

| # | 问题 | 位置 |
|---|------|------|
| L1 | 每次采集新建 resty.Client 无连接复用；目标串行执行，单目标超时拖累整轮 | `scraper/scraper.go:126`，`loop.go:58` |
| L2 | 采集失败时对每个 metric 各发一条 CRITICAL 告警+写汇总，一次宕机放大成 N 条告警 | `scraper/loop.go:113-158` |
| L3 | ProbeURL 对 https 强制 `InsecureSkipVerify` | `scraper/loop.go:318` |
| L4 | `consecutiveAlertCounts` 只增不清，目标删除后 key 残留，长期运行缓慢泄漏 | `scraper/scraper.go:401-452` |

### 3.2 测试执行（testcase / assert / psv）

| # | 问题 | 位置 |
|---|------|------|
| L5 | `vars.Replace` 每次调用重新 `regexp.MustCompile`，热路径性能浪费 | `internal/vars/vars.go:74` |
| L6 | `StreamAssert` 语义为"任一断言通过即通过"，与多断言应全过的直觉相反且无文档说明 | `internal/assert/assert.go:235-242` |
| L7 | `parseInt` 用 `\d+` 提取，负号丢失（"-1"→1） | `psv/psv.go:410-424` |
| L8 | `stream_assert` 字段 `json.Unmarshal` 错误被忽略，配置写错静默不生效 | `psv/psv.go:275` |
| L9 | `parseKeyValueMap` 回退路径按逗号朴素分割，值含逗号/冒号即错乱 | `psv/psv.go:350-359` |
| L10 | 流式模式实际用 resty 一次性读完整个 body 再"伪流式"扫描，`MaxWaitMs` 完全不生效（文档与实现不符） | `testcase/executor.go:310-341` |
| L11 | extract 表达式按逗号分割，JSONPath 含逗号会被截断 | `assert/assert.go:294` |
| L12 | 报告 `escapePipe` 转义后读取端不反转义；Error 字段含换行会破坏报告行结构 | `testcase/report.go:50`，`helper.go:289` |
| L13 | 每执行完一个用例就重新生成并写盘全量报告，O(n²) IO | `testcase/runner.go:91-98` |
| L14 | `AggregateResultsByFile` 用 map 聚合，报告输出顺序每次运行不稳定 | `testcase/helper.go:197-204` |
| L15 | `ErrUnexpectedStatus` 用 `rune(code+'0')` 拼错误信息，状态码输出乱码；`RegisterTest/RunAll/LoadFromPSV/PrettyPrintBody` 为死代码 | `testcase/psv_loader.go:91-93` |
| L16 | `RunParallel` 名为并行实为串行，误导性 API；`StartMonitor`、`DisplayReport` 等导出函数无调用方（死代码） | `testcase/helper.go:293` 等 |

### 3.3 系统监控（sysmon）

| # | 问题 | 位置 |
|---|------|------|
| L17 | 小时 CSV 无限增长无轮转；关机 flush 与整点落盘可能写同一小时重复记录 | `sysmon/storage.go`，`sysmon.go:325-343` |
| L18 | `loadMetrics` 打开失败返回 `(nil, nil)`，与"无数据"无法区分 | `sysmon/storage.go:159-162` |
| L19 | 图表时间标签假设数据均匀覆盖过去 24h，与实际采集时间戳不对齐；中文 `%-46s` 宽度计算错误致边框错位 | `sysmon/chart.go:233-252`，`monitor/monitor.go:366` |

### 3.4 邮件（email）

| # | 问题 | 位置 |
|---|------|------|
| L20 | SMTP 仅支持 465 隐式 TLS，不支持 587 STARTTLS；`tls.Dial` 无超时，服务器 hang 会永久阻塞告警协程 | `email/email.go:78-83` |
| L21 | `formatBody` 空操作死函数；`groupBySource` 尾部 `_ = src` 死代码 | `email/email.go:56`，`dispatcher.go:254` |
| L22 | alertChan 缓冲仅 200，告警风暴时 `DispatchAlert` 阻塞采集循环 | `email/dispatcher.go:39,64` |

### 3.5 配置与调度（config / scheduler / bootstrap）

| # | 问题 | 位置 |
|---|------|------|
| L23 | `fmt.Sscanf` 解析 report_time 失败时静默使用部分零值，无格式校验 | `scheduler/scheduler.go:27` |
| L24 | bootstrap 默认配置 network 阈值 1.0 KB/s 与 defaults.go 的 3072/1024 不一致，默认配置启动会告警轰炸 | `bootstrap/bootstrap.go:147-148` vs `config/defaults.go:139-144` |
| L25 | `Monitor.AlertInterval` 仅用于启动报告展示，实际冷却用 `email.*_cooldown`，两套配置易混淆；`SevereStatus`/`Authorization`/`UserId` 定义但从未使用 | `config/types.go` |
| L26 | `logger.InitLogger` 若 encoding 为空串直接 Fatal，无默认值兜底；`sys-report` 末尾 `os.Exit(0)` 跳过 `logger.Sync` | `logger/logger.go:46,66`；`sysmon/report.go:46` |

### 3.6 热加载与监控（monitor）

| # | 问题 | 位置 |
|---|------|------|
| L27 | 热加载只扫 CaseDir 忽略命令行 paths；新增用例不更新 `allTestCases`（pre 引用找不到）；ReloadConfig 后仅日志级别生效，email/sysmon/scraper 新配置均不生效 | `monitor/hotreload.go:36-48` |

### 3.7 存储（storage）

| # | 问题 | 位置 |
|---|------|------|
| L28 | `appendRecord` 的 `ensureCSV` 硬编码 executionHeader，其他 CSV 丢失重建时写错表头 | `storage/init.go:430-431` |
| L29 | `readRecords` 用 `ReadAll` 全量加载，追加型 CSV 无限增长后查询内存占用失控 | `storage/init.go:419` |
| L30 | 周期查询用 `After/Before` 严格比较，恰好等于边界的记录被丢弃 | `storage/record_scraper.go:80` |
| L31 | `ensureColumnsSystemAlert` 的 `needCol` 参数 idx 未使用，补列顺序与表头不保证一致；`upgradeAlertSummaryRecords` 固定 10 列 copy，超长记录被截断 | `storage/record_sysalert.go:111-124`，`record_alert.go:129` |

### 3.8 报告（report）

| # | 问题 | 位置 |
|---|------|------|
| L32 | `HourlyData[avg.Hour]` 索引无边界检查，依赖存储层保证 0-23 | `report/report_generator.go:191` |

### 3.9 其他

| # | 问题 | 位置 |
|---|------|------|
| L33 | `main.go` isTerminal 判断与注释意图相反：双击运行的控制台程序 stdin 仍是字符设备，"按任意键退出"实际不触发 | `main.go:22-41` |
| L34 | 全项目 0 个 `*_test.go` 文件，核心解析/断言/存储逻辑无任何测试覆盖 | 全项目 |

---

## 四、修复优先级建议

### 第一梯队（立即处理，涉及安全与可用性）

1. **H8**：吊销并移除入库的 SMTP 授权码，改用环境变量注入
2. **H7**：移除硬编码 `InsecureSkipVerify`，改为可配置项
3. **H1**：在 `RunTests` 入口补 `httpclient.InitClient()`
4. **H3**：`vars.Replace` 加读锁（或改用 `sync.Map` / 快照拷贝）
5. **H5/H6**：修复退出流程——报告调度器支持 stop，email dispatcher 先停生产者再 close channel

### 第二梯队（影响结果正确性）

6. **H2**：流式断言分支不得无条件 `Passed = true`，保留完整 result 字段
7. **H9**：周/月/年报改为统计上一完整周期
8. **H10**：重写 `fixRegexEscapes`，仅在确有必要时做最小化转义（或移除该机制，文档要求用户写合法正则）
9. **H11/M11**：统一时间处理链路，存储与解析使用同一时区（建议统一存 UTC，展示时转本地）
10. **H12**：日志输出变量时过滤敏感 key（含 key/token/secret/password/auth 字样的一律打码）

### 第三梯队（稳定性与体验）

11. **H4**：`startTask` 对 interval ≤ 0 做兜底（回退默认值并告警）
12. **M5/M6**：PSV 解析改为先扫描定位表头行、增大 Scanner buffer
13. **M7**：pre 条件加循环依赖检测（visited set）
14. **M8**：要么接入 cleaner 启动逻辑，要么删除该模块与配置项
15. **M3**：采集循环按 target 各自的 interval 调度
16. **M2**：sysmon 磁盘采集按平台取根路径（Windows 用 `C:\` 或当前盘符），并透出 error

### 长期改进

- 补充核心模块单元测试（psv 解析、assert、vars、storage），当前测试覆盖为 0（L34）
- 存储层引入原子写（临时文件 + rename）与 CSV 轮转策略（M10、L17、L29）
