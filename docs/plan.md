# gwatch 功能计划书 —— 函数级精读与设计思路

> 本文档已按模块分拆为多个独立文档，便于按功能查阅。

---

## 分册索引

| 文件 | 覆盖内容 |
|------|----------|
| [plan/00_overview.md](./plan/00_overview.md) | 总体设计思路、数据流总图、设计亮点、制作流程复盘 |
| [plan/01_bootstrap.md](./plan/01_bootstrap.md) | 启动与入口：main.go、cmd/root.go、bootstrap |
| [plan/02_psv.md](./plan/02_psv.md) | PSV 测试用例解析：数据结构 + 全部解析函数 |
| [plan/03_testcase.md](./plan/03_testcase.md) | 测试执行引擎：executor / runner / assert / vars / httpclient |
| [plan/04_monitor.md](./plan/04_monitor.md) | 接口监控模块：monitor / task / alert |
| [plan/05_scraper.md](./plan/05_scraper.md) | 远程资源采集器：loop 与辅助函数 |
| [plan/06_sysmon.md](./plan/06_sysmon.md) | 系统资源监控：collector / sysmon / alert / storage / chart |
| [plan/07_alert.md](./plan/07_alert.md) | 统一告警调度：dispatcher 全部函数 |
| [plan/08_report.md](./plan/08_report.md) | 报告系统：scheduler + report |
| [plan/09_storage.md](./plan/09_storage.md) | 数据持久化层：CSV 存储全部函数 |
| [plan/10_infrastructure.md](./plan/10_infrastructure.md) | 基础设施层：timeutil / logger / email / util / cleaner |

---

## 阅读建议

- 想了解**整体架构与设计哲学** → 读 [00_overview.md](./plan/00_overview.md)
- 想了解**程序如何启动** → 读 [01_bootstrap.md](./plan/01_bootstrap.md)
- 想了解**PSV 文件如何被解析** → 读 [02_psv.md](./plan/02_psv.md)
- 想了解**测试用例如何执行** → 读 [03_testcase.md](./plan/03_testcase.md)
- 想了解**接口如何被持续监控** → 读 [04_monitor.md](./plan/04_monitor.md)
- 想了解**远程资源如何被采集** → 读 [05_scraper.md](./plan/05_scraper.md)
- 想了解**系统指标如何被采集与聚合** → 读 [06_sysmon.md](./plan/06_sysmon.md)
- 想了解**告警如何被统一调度** → 读 [07_alert.md](./plan/07_alert.md)
- 想了解**报告如何生成与分发** → 读 [08_report.md](./plan/08_report.md)
- 想了解**数据如何持久化** → 读 [09_storage.md](./plan/09_storage.md)
- 想了解**基础设施细节** → 读 [10_infrastructure.md](./plan/10_infrastructure.md)