# 八、报告系统（`internal/report/` + `internal/scheduler/`）

## 8.1 `scheduler/scheduler.go` —— 周期调度

| 函数 | 作用 |
|------|------|
| `NewPeriodicScheduler(opts...)` | 创建调度器，默认 `reportHour=7, reportMinute=0`。 |
| `WithReportTime` / `WithTriggerCallback` | 函数式配置选项。 |
| `Start()` | 计算下次触发时间 → 循环 `Sleep` → 触发后 +24h；用 `lastSentDate` 防止重复触发。 |
| `ShouldTriggerWeekly(now) bool` | 周一触发。 |
| `ShouldTriggerMonthly(now) bool` | 每月 1 日触发。 |
| `ShouldTriggerYearly(now) bool` | 每年 1 月 1 日触发。 |
| `GetWeekStart(date) time.Time` | 获得指定日期所在周的周一。 |

## 8.2 `report/scheduler.go` —— 报告调度

| 函数 | 作用 |
|------|------|
| `NewReportScheduler(sender)` | 注入邮件发送函数。 |
| `Start()` | 用 `PeriodicScheduler` 每日触发 `generateAllReports`。 |
| `generateAllReports()` | 根据配置判断哪些周期需要触发。 |
| `generateAndSendReport(period, date, sender)` | 计算 `startDate/endDate` → `GenerateReportFromStorage` → `SaveReport`（错误日志为 ERROR）→ `PrepareReportEmail` → `sender`（保存失败仍会尝试发送邮件）。 |

## 8.3 `report/report.go` —— 便捷入口

| 函数 | 作用 |
|------|------|
| `Generate / GenerateDaily / GenerateWeekly / GenerateMonthly / GenerateYearly / GenerateStartup` | 转调底层生成器，便于外部调用。 |

### 作者思考

报告生成与"测试/监控"解耦，只依赖 CSV 存储中的聚合数据，这样即使系统曾经停机，只要 CSV 仍在，报告就能生成。保存与发送采用"先存后发"的容错顺序，避免因存储问题导致报告丢失。