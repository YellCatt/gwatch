package testcase

import (
	"fmt"
	"os"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/logger"
	"gwatch/internal/psv"
)

func ExecuteGlobalPreConditions(testCases []psv.TestCase) {
	if len(config.GlobalConfig.App.GlobalPre) == 0 {
		return
	}

	fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║ 执行全局前置条件                                       ║\n")
	fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")

	for _, preID := range config.GlobalConfig.App.GlobalPre {
		found := false
		for _, tc := range testCases {
			if tc.ID == preID {
				fmt.Printf("[全局前置] 执行: %s - %s\n", tc.ID, tc.Desc)
				result := ExecuteTestCase(tc)
				if !result.Passed {
					fmt.Printf("[全局前置] ❌ 失败: %s\n", result.Error)
					fmt.Printf("\n全局前置条件失败，终止执行\n")
					errorMsg := fmt.Sprintf("全局前置条件 '%s' 执行失败: %s", tc.ID, result.Error)
					if err := email.SendErrorReportEmail(errorMsg); err != nil {
						logger.Error("Failed to send error report email", zap.Error(err))
					}
					os.Exit(1)
				}
				fmt.Printf("[全局前置] ✅ 成功\n")
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("[全局前置] ⚠️ 未找到测试用例: %s\n", preID)
		}
	}
	fmt.Println()
}

func ExecuteGlobalPostConditions(testCases []psv.TestCase) {
	if len(config.GlobalConfig.App.GlobalPost) == 0 {
		return
	}

	fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║ 执行全局后置条件                                       ║\n")
	fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")

	for _, postID := range config.GlobalConfig.App.GlobalPost {
		found := false
		for _, tc := range testCases {
			if tc.ID == postID {
				fmt.Printf("[全局后置] 执行: %s - %s\n", tc.ID, tc.Desc)
				result := ExecuteTestCase(tc)
				if !result.Passed {
					fmt.Printf("[全局后置] ❌ 失败: %s\n", result.Error)
				} else {
					fmt.Printf("[全局后置] ✅ 成功\n")
				}
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("[全局后置] ⚠️ 未找到测试用例: %s\n", postID)
		}
	}
	fmt.Println()
}