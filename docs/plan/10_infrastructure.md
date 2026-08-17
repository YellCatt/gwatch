# 十、基础设施层

## 10.1 `timeutil/timeutil.go`

| 函数 | 作用 |
|------|------|
| `initShanghaiLocation() *time.Location` | 优先加载 `Asia/Shanghai`，失败时回退到固定 `UTC+8`。 |
| `Now() time.Time` | 返回东八区当前时间。 |
| `Format / FormatDateTime / FormatDateTimeMs / FormatCompact` | 统一时间格式化。 |

## 10.2 `logger/logger.go`

| 函数 | 作用 |
|------|------|
| `InitLogger(cfg)` | 按配置初始化 zap 日志（输出到控制台和文件）。 |
| `Debug/Info/Warn/Error` 系列 | 封装 zap 的结构化日志接口。 |

## 10.3 `email/email.go`

| 函数 | 作用 |
|------|------|
| `InitEmail()` | 从配置加载 SMTP 设置。 |
| `SendEmail(subject, body) error` | 底层 SMTP 发送。 |
| `SendCustomEmail` / `SendErrorReportEmail` | 便捷封装。 |

## 10.4 `util/util.go`

| 函数 | 作用 |
|------|------|
| `FormatBytes(b uint64) string` | 智能格式化 B/KB/MB/GB/TB。 |
| `FormatSpeed(kbps float64) string` | 智能格式化速度（带单位选择）。 |
| `IsSpeedUnit(unit) bool` | 判断是否为速度类单位。 |
| `GetDeviceName() string` | 获取当前主机名用于邮件展示。 |

## 10.5 `cleaner/cleaner.go`

| 函数 | 作用 |
|------|------|
| `StartCleaner()` | 定时触发历史数据清理（过期的 CSV 行、旧报告等）。 |