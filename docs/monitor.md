# 接口监控详解

## 概述

接口监控是 gwatch 的核心功能之一，基于 PSV 测试用例实现持续在线监控。系统按配置的周期自动执行测试用例，检测接口响应速度和可用性，并在异常时通过邮件发送告警。

## 启动方式

```bash
# 默认即为监控模式
gwatch cases/

# 或指定目录和标签过滤
gwatch cases/api/ --tags critical
```

## 工作流程

```
1. 加载 PSV 用例 → 2. 过滤 monitor_enabled=true → 3. 启动 Worker 协程池
                                                              ↓
                                        4. 每个用例创建定时 Ticker
                                                              ↓
                                        5. 按周期执行测试用例
                                                              ↓
                                        6. 检查告警条件（失败/慢响应）
                                                              ↓
                                        7. 触发告警 → 邮件通知
                                                              ↓
                                        8. 持久化结果 → CSV 存储
                                                              ↓
                                        9. 纳入报告统计
```

## 配置说明

### PSV 用例中的监控字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `monitor_enabled` | bool | 开启监控模式 |
| `monitor_interval` | int | 监控周期（秒） |
| `response_threshold` | int | 慢响应阈值（毫秒），超过此值触发慢告警 |
| `alert_on_failure` | bool | 接口失败时是否发送邮件告警 |
| `alert_on_slow` | bool | 响应超时是否发送邮件告警 |

### config.yaml 中的监控配置

```yaml
monitor:
  enabled: true
  default_interval: 60       # 默认监控间隔
  alert_on_failure: true     # 全局默认失败告警开关
  alert_on_slow: true        # 全局默认慢响应告警开关
  max_workers: 1             # 最大并发执行数
  alert_interval: 21600      # 同一接口告警冷却时间（秒），默认 6 小时
  daily_report: true         # 启用日报
  weekly_report: false       # 启用周报
  monthly_report: false      # 启用月报
  yearly_report: false       # 启用年报
  report_time: "07:00"       # 报告生成时间
```

## 告警检测逻辑

### 失败告警

```
条件：!result.Passed && tc.alert_on_failure
级别：CRITICAL
```

接口返回非预期状态码、断言失败、网络超时等均视为失败。

### 慢响应告警

```
条件：duration_ms > response_threshold && tc.alert_on_slow
级别：WARNING
```

请求耗时超过配置的阈值时触发。耗时包含 DNS 解析、TLS 握手、服务器处理和数据传输全流程。

## 告警冷却机制

为防止告警风暴，系统实现了冷却抑制策略：

- 同一接口在冷却时间内只发送一次告警邮件
- 冷却时间通过 `monitor.alert_interval` 配置，默认 21600 秒（6 小时）
- 不同接口的告警独立计算冷却周期

## 热加载

监控模式支持配置热加载（每 30 秒扫描一次）：

- 新增的 PSV 文件会被自动加载
- 修改的用例会被自动更新
- 删除的用例会被自动停止

## 数据持久化

每次监控执行结果会写入 CSV 文件，包含：

- 用例标识、描述、执行时间
- 耗时、成功/失败状态、错误信息
- 响应状态码、请求/响应头和体
- 提取的变量值
- 告警类型和告警消息

这些历史数据会被报告系统用于生成统计报告。