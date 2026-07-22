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

	"gwatch/internal/timeutil"
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

	subject := fmt.Sprintf("【测试异常】gwatch - %s - %s", getDeviceName(), timeutil.FormatDateTime(timeutil.Now()))

	var body strings.Builder
	body.WriteString("===== 测试异常报告 =====\n\n")
	body.WriteString(fmt.Sprintf("发生时间: %s\n", timeutil.FormatDateTime(timeutil.Now())))
	body.WriteString(fmt.Sprintf("测试设备: %s\n", getDeviceName()))
	body.WriteString(fmt.Sprintf("\n异常信息:\n"))
	body.WriteString(fmt.Sprintf("  %s\n", errorMessage))
	body.WriteString("\n===== 报告结束 =====\n")
	body.WriteString("来自 gwatch 监控系统")

	log.Println("发送异常报告邮件...")
	return SendEmail(subject, body.String())
}

// SendCustomEmail 发送自定义邮件
func SendCustomEmail(subject, body string) error {
	if !Config.Enabled {
		log.Println("邮件发送功能已禁用，跳过邮件发送")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		log.Println("邮件配置未设置，跳过邮件发送")
		return nil
	}

	log.Println("发送自定义邮件...")
	return SendEmail(subject, body)
}