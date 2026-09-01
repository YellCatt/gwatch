# 报告与告警系统

## 报告系统

### 报告类型

| 类型 | 周期 | 说明 |
|------|------|------|
| 日报 | 每天 | 统计前一天的接口执行情况、成功率、平均耗时、告警汇总 |
| 周报 | 每周一 | 统计上周的整体数据趋势 |
| 月报 | 每月 1 日 | 统计上月的汇总数据 |
| 年报 | 每年 1 月 1 日 | 统计上年度的汇总数据 |
| 启动报告 | 启动时 | 系统启动时发送的概要报告 |

### 报告配置

```yaml
monitor:
  daily_report: true        # 启用日报
  weekly_report: false      # 启用周报
  monthly_report: false     # 启用月报
  yearly_report: false      # 启用年报
  report_time: "07:00"      # 每日报告生成时间（东八区）
```

### 报告内容

每份报告包含以下部分：

1. **概要统计**：总执行次数、成功率、平均耗时
2. **接口排名**：按调用次数/成功率/平均耗时排序
3. **告警汇总**：报告周期内的所有告警记录
4. **异常详情**：失败用例的错误信息汇总
5. **趋势图表**：关键指标的趋势可视化（ASCII）

### 报告分发

- **本地保存**：报告以 CSV/TXT 格式保存到 `reports/` 目录
- **邮件发送**：报告通过 SMTP 邮件发送到配置的收件人
- 保存失败时仍尝试发送邮件，确保报告不丢失

### 报告调度

使用周期性调度器（`PeriodicScheduler`）：

- 日报：每天 `report_time` 时刻触发
- 周报：每周一触发
- 月报：每月 1 日触发
- 年报：每年 1 月 1 日触发
- 所有时间均使用东八区（Asia/Shanghai）

---

## 告警系统

### 统一告警调度

三类告警源合并到统一的告警调度通道：

| 告警源 | 标识 | 说明 |
|--------|------|------|
| 接口监控 | `api_monitor` | PSV 用例的失败/慢响应告警 |
| 数据采集 | `scraper` | JSON 指标的阈值告警 |
| 系统监控 | `system_monitor` | 主机资源超限告警 |

### 告警处理流程

```
1. 告警源调用 DispatchAlert() 发送告警
           ↓
2. 告警进入统一通道（缓冲 200 条）
           ↓
3. 调度器每 30 秒收集一次告警
           ↓
4. 检查冷却时间，抑制重复告警
           ↓
5. 合并告警内容，生成统一邮件
           ↓
6. 发送邮件通知
```

### 告警冷却

为防止告警风暴，每个告警源有独立的冷却配置：

```yaml
email:
  scraper_cooldown: 21600    # 数据采集告警冷却（秒），默认 6 小时
  api_cooldown: 21600        # 接口监控告警冷却（秒），默认 6 小时
  system_cooldown: 7200      # 系统资源告警冷却（秒），默认 2 小时
```

冷却机制：同一指标在冷却时间内只发送一次告警，后续告警被抑制。

### 告警级别

| 级别 | 含义 | 来源 |
|------|------|------|
| CRITICAL | 严重 | 接口失败 / 严重指标超限 |
| WARNING | 警告 | 慢响应 / 警告阈值超限 |

### 邮件配置

```yaml
email:
  enabled: true
  from: "sender@example.com"
  to:
    - "recipient@example.com"
  auth_code: "your_auth_code"
  smtp_server: "smtp.example.com"
  smtp_port: 465
  error_subject: "[gwatch] 监控系统错误告警"
```

---

## 自动清理

### 配置

```yaml
cleaner:
  enabled: true
  retention_days: 30         # 数据保留天数
  log_dir: "./logs"          # 日志目录
  report_dir: "./reports"    # 报告目录
  # data_dir 已废弃：数据存储目录下的系统指标/告警/汇总 CSV 属于业务数据，不参与清理
  include_patterns: ["*.log", "*.json", "*.txt"]
  exclude_patterns: []
  interval_hours: 24         # 清理执行间隔（小时）
```

### 清理策略

- 每 24 小时执行一次清理
- 保留 `retention_days` 天内的文件
- 超过保留期的日志、报告、数据文件会被自动删除
- 支持文件模式过滤（include/exclude）