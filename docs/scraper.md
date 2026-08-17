# 数据采集器（Scraper）详解

## 概述

数据采集器（Scraper）是 gwatch 的另一种监控模式，用于从任意 HTTP 接口采集 JSON 指标数据。通过 JSONPath 路径提取目标值，支持阈值告警，适用于监控主机状态、服务健康检查等场景。

## 启动方式

```bash
# 启动数据采集器
gwatch scraper

# 探测接口 JSON 结构（辅助工具）
gwatch probe http://example.com/api/status
```

## 工作流程

```
1. 读取 config.yaml 中的 scraper.targets 配置
           ↓
2. 遍历所有启用的目标（enabled: true）
           ↓
3. 对每个目标发送 HTTP 请求
           ↓
4. 使用 JSONPath 从响应中提取指标值
           ↓
5. 检查每个指标的阈值（gt/lt/eq/ge/le）
           ↓
6. 触发告警 → 邮件通知
           ↓
7. 记录指标数据 → CSV 存储
           ↓
8. 按 interval 周期重复执行
```

## 目标配置

在 `config.yaml` 的 `scraper.targets` 中配置采集目标：

```yaml
scraper:
  enabled: true
  targets:
    - name: "主机监控"
      url: "http://192.168.1.100/api/status"
      method: GET
      timeout: 5s
      interval: 10
      enabled: true
      headers:
        Authorization: "Bearer {{token}}"
      body: ""
      proxy: ""
      metrics:
        - name: cpu_usage
          path: "$.info.cpu.use_percent"
          alias: "CPU使用率"
          unit: "%"
          scale: 1
          auto_percent: false
          compare_op: "gt"
          threshold: 85
          warning_threshold: 70
          alert: true
          alert_level: "CRITICAL"
          consecutive: 1
          optional: false
```

## 指标字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 指标唯一名称（英文） |
| `path` | string | JSONPath 路径，如 `$.info.cpu.usage` |
| `alias` | string | 指标别名（中文，用于报告显示） |
| `unit` | string | 单位，如 `%`、`MB`、`Mbps` |
| `scale` | number | 缩放倍数，采集值乘以此值 |
| `auto_percent` | bool | 自动百分比转换（0.45 → 45%） |
| `compare_op` | string | 比较操作符：gt/lt/eq/ge/le |
| `threshold` | number | 告警阈值（严重级别） |
| `warning_threshold` | number | 警告阈值（低于严重级别） |
| `alert` | bool | 是否启用该指标的告警 |
| `alert_level` | string | 告警级别：CRITICAL / WARNING |
| `consecutive` | int | 连续触发次数才告警 |
| `optional` | bool | 是否为可选指标（接口不返回时不报错） |

## JSONPath 提取

使用 `tidwall/gjson` 库实现 JSONPath 提取，支持标准语法：

```
$.info.cpu.use_percent     → 嵌套对象属性
$.data.items[0].name       → 数组元素
$.data.items.length        → 数组长度
$.info.memory.*            → 通配符
```

## 比较操作符

| 操作符 | 含义 | 示例 |
|--------|------|------|
| `gt` | 大于 | CPU > 85% 告警 |
| `lt` | 小于 | 网速 < 10Mbps 告警 |
| `eq` | 等于 | 状态码 = 500 告警 |
| `ge` | 大于等于 | 温度 >= 90°C 告警 |
| `le` | 小于等于 | 剩余 <= 5% 告警 |

## 告警级别

- **CRITICAL**：严重告警，需要立即处理
- **WARNING**：警告，需要关注

## 连续触发

通过 `consecutive` 字段设置需要连续多少次超过阈值才触发告警，避免瞬时抖动导致的误报。

## 可选指标

`optional: true` 的指标在接口不返回该字段时不会报错，适用于某些接口可能不返回的可选指标（如 GPU 使用率、网速等）。

## 辅助工具

### Probe 探测

```bash
gwatch probe http://example.com/api/status
```

发送 HTTP 请求到目标 URL，解析返回的 JSON 并打印树形结构，方便获取 JSONPath 路径。