# 一、启动与入口模块

## 1.1 `main.go` —— 程序主入口

| 函数 | 作用 |
|------|------|
| `main()` | 调用 `cmd.Execute()` 启动 Cobra 命令；Windows 下若检测到非交互式终端（如双击运行），等待用户输入后再退出，便于查看结果。 |
| `isTerminal() bool` | 通过 `os.Stdin.Stat()` 检查标准输入是否为字符设备，用来区分"命令行启动"与"双击运行"。 |

## 1.2 `cmd/root.go` —— 根命令

| 函数 | 作用 |
|------|------|
| `Execute()` | Cobra 推荐的执行入口；若命令执行失败，会在邮件模块已配置时尝试发送错误报告邮件，保证异常可追溯。 |
| `init()` | 注册 Cobra 初始化回调（`bootstrap.InitApp`），绑定 `--config`、`--tags/-T`、`--test/-t` 三个 Flag，注册子命令 `system-report`。 |
| `rootCmd.Run` 匿名函数 | 根据 `--test` 标志分流：`true` 调用 `testcase.RunTests`（一次性测试），`false` 调用 `monitor.StartMonitorMode`（持续监控）。 |

### 作者思考

用 Cobra 的 Flag 把"测试模式"和"监控模式"合并在同一个二进制里，避免维护两个入口文件，降低分发成本。

## 1.3 `bootstrap/bootstrap.go` —— 初始化编排

| 函数 | 作用 |
|------|------|
| `InitApp()` | 依次调用 `logger.InitLogger`、`config.LoadConfig`、`httpclient.InitClient`、`email.InitEmail`、`cleaner.StartCleaner`、`printStartupBanner`，保证依赖按顺序就绪。 |

### 作者思考

把"顺序依赖"集中到 bootstrap，避免每个子模块自己判断配置是否加载，减少隐式耦合。