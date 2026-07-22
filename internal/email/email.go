// Package email 提供邮件发送功能
package email

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
	"time"

	"pipetGo/internal/testcase"
	"pipetGo/internal/timeutil"
)

type EmailConfig struct {
	Enabled    bool
	FromEmail  string
	ToEmail    []string
	AuthCode   string
	SMTPServer string
	SMTPPort   int
	DeviceName string
}

var Config EmailConfig

func InitEmail(cfg EmailConfig) {
	Config = cfg
}

// getDeviceName 获取设备名称，优先使用配置值，未配置时自动获取主机名
func getDeviceName() string {
	if Config.DeviceName != "" {
		return Config.DeviceName
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "未知设备"
	}
	return hostname
}

func formatSubject(subject string) string {
	return subject
}

func formatBody(body string) string {
	return body
}

func SendEmail(subject, body string) error {
	subject = formatSubject(subject)
	body = formatBody(body)

	toEmails := strings.Join(Config.ToEmail, ", ")
	msg := []byte("From: " + Config.FromEmail + "\r\n" +
		"To: " + toEmails + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")

	addr := fmt.Sprintf("%s:%d", Config.SMTPServer, Config.SMTPPort)
	auth := smtp.PlainAuth("", Config.FromEmail, Config.AuthCode, Config.SMTPServer)

	log.Printf("连接 SMTP 服务器: %s\n", addr)

	// 使用 TLS 连接
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         Config.SMTPServer,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		log.Printf("TLS 连接失败: %v\n", err)
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, Config.SMTPServer)
	if err != nil {
		log.Printf("创建 SMTP 客户端失败: %v\n", err)
		return err
	}
	defer client.Close()

	// 认证
	if err := client.Auth(auth); err != nil {
		log.Printf("SMTP 认证失败: %v\n", err)
		return err
	}

	// 设置发件人
	if err := client.Mail(Config.FromEmail); err != nil {
		log.Printf("设置发件人失败: %v\n", err)
		return err
	}

	// 设置多个收件人
	for _, to := range Config.ToEmail {
		if err := client.Rcpt(to); err != nil {
			log.Printf("设置收件人 %s 失败: %v\n", to, err)
			return err
		}
	}

	// 发送邮件内容
	w, err := client.Data()
	if err != nil {
		log.Printf("获取数据写入器失败: %v\n", err)
		return err
	}

	_, err = w.Write(msg)
	if err != nil {
		log.Printf("写入邮件内容失败: %v\n", err)
		return err
	}

	err = w.Close()
	if err != nil {
		log.Printf("关闭数据写入器失败: %v\n", err)
		return err
	}

	log.Println("✅ 邮件发送成功")
	return nil
}

func GenerateTestReportContent(results []testcase.TestResult) string {
	var sb strings.Builder

	// 使用东八区时间
	now := timeutil.Now()

	sb.WriteString(fmt.Sprintf("===== 测试报告 =====\n\n"))
	sb.WriteString(fmt.Sprintf("执行时间: %s\n", timeutil.FormatDateTime(now)))
	sb.WriteString(fmt.Sprintf("测试设备: %s\n", getDeviceName()))

	var totalDuration time.Duration

	// 使用统一的统计函数，与测试开始邮件口径一致
	chainPassed, chainFailed, chainSkipped, independentPassed, independentFailed, independentSkipped, totalDuration := testcase.SummarizeResultsByType(results)

	totalPassed := chainPassed + independentPassed
	totalFailed := chainFailed + independentFailed
	totalSkipped := chainSkipped + independentSkipped

	// 聚合结果用于失败详情展示
	aggregated := testcase.AggregateResultsByFile(results)

	sb.WriteString(fmt.Sprintf("测试统计:\n"))
	sb.WriteString(fmt.Sprintf("  总测试数: %d\n", totalPassed+totalFailed+totalSkipped))
	sb.WriteString(fmt.Sprintf("  通过数:   %d\n", totalPassed))
	sb.WriteString(fmt.Sprintf("  失败数:   %d\n", totalFailed))
	sb.WriteString(fmt.Sprintf("  跳过数:   %d\n", totalSkipped))
	sb.WriteString(fmt.Sprintf("  通过率:   %.2f%%\n", float64(totalPassed)/float64(totalPassed+totalFailed)*100))
	sb.WriteString(fmt.Sprintf("  总耗时:   %v\n\n", totalDuration))

	sb.WriteString(fmt.Sprintf("单例测试统计:\n"))
	sb.WriteString(fmt.Sprintf("  测试数:   %d\n", independentPassed+independentFailed+independentSkipped))
	sb.WriteString(fmt.Sprintf("  通过数:   %d\n", independentPassed))
	sb.WriteString(fmt.Sprintf("  失败数:   %d\n", independentFailed))
	sb.WriteString(fmt.Sprintf("  跳过数:   %d\n", independentSkipped))
	if independentPassed+independentFailed > 0 {
		sb.WriteString(fmt.Sprintf("  通过率:   %.2f%%\n", float64(independentPassed)/float64(independentPassed+independentFailed)*100))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("链式测试统计:\n"))
	sb.WriteString(fmt.Sprintf("  测试数:   %d\n", chainPassed+chainFailed+chainSkipped))
	sb.WriteString(fmt.Sprintf("  通过数:   %d\n", chainPassed))
	sb.WriteString(fmt.Sprintf("  失败数:   %d\n", chainFailed))
	sb.WriteString(fmt.Sprintf("  跳过数:   %d\n", chainSkipped))
	if chainPassed+chainFailed > 0 {
		sb.WriteString(fmt.Sprintf("  通过率:   %.2f%%\n", float64(chainPassed)/float64(chainPassed+chainFailed)*100))
	}
	sb.WriteString("\n")

	if len(aggregated) > 0 {
		sb.WriteString("测试详情:\n")
		sb.WriteString("-" + strings.Repeat("-", 78) + "\n")
		sb.WriteString(fmt.Sprintf("%-15s %-40s %-10s %-15s %s\n", "ID", "描述", "状态", "耗时", "错误信息"))
		sb.WriteString("-" + strings.Repeat("-", 78) + "\n")

		for _, r := range aggregated {
			if !r.Passed && !r.TestCase.Skip {
				sb.WriteString(fmt.Sprintf("%-15s %-40s %-10s %-15v %s\n",
					r.TestCase.ID,
					r.TestCase.Desc,
					"F",
					r.Duration,
					r.Error))
			}
		}
		sb.WriteString("-" + strings.Repeat("-", 78) + "\n")
	}

	sb.WriteString("\n===== 报告结束 =====\n")
	sb.WriteString("来自 pipetGo 测试程序")

	return sb.String()
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func SendTestReportEmail(results []testcase.TestResult) error {
	if !Config.Enabled {
		log.Println("邮件发送功能已禁用，跳过邮件发送")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		log.Println("邮件配置未设置，跳过邮件发送")
		return nil
	}

	// 使用东八区时间
	subject := fmt.Sprintf("【测试报告】pipetGo - %s - %s", getDeviceName(), timeutil.FormatDateTime(timeutil.Now()))

	body := GenerateTestReportContent(results)

	log.Println("发送测试报告邮件...")
	return SendEmail(subject, body)
}

// SendErrorReportEmail 发送异常退出报告邮件
func SendErrorReportEmail(errorMessage string) error {
	if !Config.Enabled {
		log.Println("邮件发送功能已禁用，跳过邮件发送")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		log.Println("邮件配置未设置，跳过邮件发送")
		return nil
	}

	subject := fmt.Sprintf("【测试异常】pipetGo - %s - %s", getDeviceName(), timeutil.FormatDateTime(timeutil.Now()))

	var body strings.Builder
	body.WriteString("===== 测试异常报告 =====\n\n")
	body.WriteString(fmt.Sprintf("发生时间: %s\n", timeutil.FormatDateTime(timeutil.Now())))
	body.WriteString(fmt.Sprintf("测试设备: %s\n", getDeviceName()))
	body.WriteString(fmt.Sprintf("\n异常信息:\n"))
	body.WriteString(fmt.Sprintf("  %s\n", errorMessage))
	body.WriteString("\n===== 报告结束 =====\n")
	body.WriteString("来自 pipetGo 测试程序")

	log.Println("发送异常报告邮件...")
	return SendEmail(subject, body.String())
}

// SendTestStartEmail 发送测试开始通知邮件
func SendTestStartEmail(testCaseCount, chainCount, independentCount int, estimatedDuration string) error {
	if !Config.Enabled {
		log.Println("邮件发送功能已禁用，跳过邮件发送")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		log.Println("邮件配置未设置，跳过邮件发送")
		return nil
	}

	// 使用东八区时间
	subject := fmt.Sprintf("【测试开始】pipetGo - %s - %s", getDeviceName(), timeutil.FormatDateTime(timeutil.Now()))

	var body strings.Builder
	body.WriteString("===== 测试开始通知 =====\n\n")
	body.WriteString(fmt.Sprintf("执行时间: %s\n", timeutil.FormatDateTime(timeutil.Now())))
	body.WriteString(fmt.Sprintf("测试设备: %s\n", getDeviceName()))
	body.WriteString(fmt.Sprintf("\n测试用例统计:\n"))
	body.WriteString(fmt.Sprintf("  本次测试用例总数: %d\n", testCaseCount))
	body.WriteString(fmt.Sprintf("  链式测试: %d\n", chainCount))
	body.WriteString(fmt.Sprintf("  独立测试: %d\n", independentCount))
	body.WriteString(fmt.Sprintf("\n预估执行时间: %s\n", estimatedDuration))
	body.WriteString("\n===== 通知结束 =====\n")
	body.WriteString("来自 pipetGo 测试程序")

	log.Println("发送测试开始通知邮件...")
	return SendEmail(subject, body.String())
}