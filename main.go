// Package main 是 gwatch 项目的主入口
// gwatch 是一个功能强大的企业级 API 测试工具
package main

import (
	"fmt"
	"os"
	"runtime"

	// cmd 包包含命令行接口的实现
	"gwatch/cmd"
)

// main 函数是程序的入口点
// 调用 cmd.Execute() 启动命令行应用
func main() {
	cmd.Execute()
	
	// 在 Windows 上，如果是双击运行，保持窗口打开
	if runtime.GOOS == "windows" {
		// 检查是否是交互式终端（双击运行时不是交互式）
		if !isTerminal() {
			fmt.Println()
			fmt.Println("按任意键退出...")
			var input string
			fmt.Scanln(&input)
		}
	}
}

// isTerminal 检查是否在交互式终端中运行
func isTerminal() bool {
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	// 判断是否是终端设备
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}