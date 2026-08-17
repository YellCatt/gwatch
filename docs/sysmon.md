# 系统资源监控详解

## 概述

系统资源监控（sysmon）负责采集本机的 CPU、内存、磁盘、网络等指标，生成 ASCII 图表报告，并在资源超限时发送邮件告警。

## 启动方式

```bash
# 生成一次性系统报告
gwatch sys-report

# 在 config.yaml 中启用后，随主程序自动启动
```

## 监控指标

| 指标 | 字段 | 说明 |
|------|------|------|
| CPU 使用率 | `cpu_percent` | CPU 占用百分比 |
| 内存使用率 | `memory_percent` | 内存占用百分比 |
| 内存已用 | `memory_used` | 已用内存（字节） |
| 内存总量 | `memory_total` | 总内存（字节） |
| 磁盘使用率 | `disk_percent` | 根分区占用百分比 |
| 磁盘已用 | `disk_used` | 已用磁盘（字节） |
| 磁盘总量 | `disk_total` | 总磁盘（字节） |
| 网络下行 | `net_down_kbps` | 网络下行速度（KB/s） |
| 网络上行 | `net_up_kbps` | 网络上行速度（KB/s） |
| 磁盘读取 | `disk_read_kbps` | 磁盘读取速度（KB/s） |
| 磁盘写入 | `disk_write_kbps` | 磁盘写入速度（KB/s） |

## 配置说明

```yaml
sys_monitor:
  enabled: true
  interval: 10              # 采集间隔（秒）
  chart_enabled: true        # 是否生成 ASCII 图表
  email_enabled: true        # 是否启用邮件告警
  cpu_threshold: 85          # CPU 严重告警阈值（%）
  memory_threshold: 90       # 内存严重告警阈值（%）
  disk_usage_threshold: 90   # 磁盘严重告警阈值（%）
  network_down_threshold: 3072    # 下行严重告警阈值（KB/s）
  network_up_threshold: 1024      # 上行严重告警阈值（KB/s）
  network_down_warn_threshold: 2048  # 下行警告阈值（KB/s）
  network_up_warn_threshold: 512    # 上行警告阈值（KB/s）
  alert_cooldown: 7200       # 告警冷却时间（秒）
```

## 告警阈值等级

| 等级 | 触发条件 | 说明 |
|------|----------|------|
| CRITICAL | 超过严重阈值 | 需要立即处理 |
| WARNING | 超过警告阈值但低于严重阈值 | 需要关注 |

## 数据采集

使用 `shirou/gopsutil` 库获取系统指标：

- **CPU**：通过 `cpu.Percent()` 获取 CPU 使用率
- **内存**：通过 `mem.VirtualMemory()` 获取虚拟内存信息
- **磁盘**：通过 `disk.Usage()` 获取磁盘使用情况
- **网络**：通过两次间隔 1 秒的 `net.IOCounters()` 差值计算速率
- **磁盘 I/O**：通过两次间隔 1 秒的 `disk.IOCounters()` 差值计算速率

## ASCII 图表

系统报告支持生成 ASCII 趋势图表，展示最近 24 小时的指标变化：

```
CPU 使用率趋势 (%)
  100 ┤
   80 ┤          ████
   60 ┤    ████  ████  ████
   40 ┤ ██ ████  ████  ████ ████
   20 ┤ ██ ████  ████  ████ ████ ████
    0 ┼──┬───┬───┬───┬───┬───┬───┬───┬──
      -24h  -20h -16h -12h -8h  -4h   0h
```

## 历史数据与聚合

系统监控数据采用多级聚合存储：

| 级别 | 说明 | 保留策略 |
|------|------|----------|
| 原始数据 | 每次采集的完整指标 | 按天滚动 |
| 小时聚合 | 按小时统计平均值 | 保留最近 7 天 |
| 日聚合 | 按天统计 | 长期保留 |
| 月聚合 | 按月统计 | 长期保留 |
| 年聚合 | 按年统计 | 长期保留 |

启动时自动回填历史缺失的聚合记录。

## 报告生成

`gwatch sys-report` 命令会：

1. 从存储加载最近 24 小时的指标数据
2. 计算当前 CPU/内存/磁盘等指标
3. 检测告警状态
4. 生成 ASCII 图表报告
5. 输出到控制台并保存到 reports 目录