// Package email 提供邮件发送功能
package email

import (
	"crypto/tls"
	"fmt"
	"mime"
	"net/smtp"
	"strings"

	"go.uber.org/zap"

	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
	"gwatch/internal/util"
)

// EmailConfig 邮件配置结构体
type EmailConfig struct {
	Enabled         bool
	FromEmail       string
	ToEmail         []string
	AuthCode        string
	SMTPServer      string
	SMTPPort        int
	DeviceName      string
	ErrorSubject    string
	ScraperCooldown int
	APICooldown     int
	SystemCooldown  int
}

var Config EmailConfig

// InitEmail 初始化邮件模块配置。
func InitEmail(cfg EmailConfig) {
	Config = cfg
}

// formatSubject 格式化邮件主题，对包含非 ASCII 字符的主题进行 RFC 2047 编码。
func formatSubject(subject string) string {
	hasNonASCII := false
	for _, r := range subject {
		if r > 127 {
			hasNonASCII = true
			break
		}
	}
	if hasNonASCII {
		return mime.QEncoding.Encode("UTF-8", subject)
	}
	return subject
}

// formatBody 格式化邮件正文（预留扩展点）。
func formatBody(body string) string {
	return body
}

// SendEmail 通过 TLS 加密连接 SMTP 服务器发送纯文本邮件。
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

	logger.Info("正在连接 SMTP 服务器", zap.String("addr", addr))

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         Config.SMTPServer,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		logger.Warn("TLS 连接失败", zap.Error(err))
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, Config.SMTPServer)
	if err != nil {
		logger.Warn("SMTP 客户端创建失败", zap.Error(err))
		return err
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		logger.Warn("SMTP 认证失败", zap.Error(err))
		return err
	}

	if err := client.Mail(Config.FromEmail); err != nil {
		logger.Warn("设置发件人失败", zap.Error(err))
		return err
	}

	for _, to := range Config.ToEmail {
		if err := client.Rcpt(to); err != nil {
			logger.Warn("设置收件人失败", zap.String("to", to), zap.Error(err))
			return err
		}
	}

	w, err := client.Data()
	if err != nil {
		logger.Warn("获取数据写入器失败", zap.Error(err))
		return err
	}

	_, err = w.Write(msg)
	if err != nil {
		logger.Warn("写入邮件内容失败", zap.Error(err))
		return err
	}

	err = w.Close()
	if err != nil {
		logger.Warn("关闭数据写入器失败", zap.Error(err))
		return err
	}

	logger.Info("邮件发送成功")
	return nil
}

// buildErrorSubject 构建异常报告邮件标题
func buildErrorSubject() string {
	if Config.ErrorSubject != "" {
		subject := strings.ReplaceAll(Config.ErrorSubject, "{{device}}", util.GetDeviceName())
		subject = strings.ReplaceAll(subject, "{{time}}", timeutil.FormatDateTime(timeutil.Now()))
		return subject
	}
	return fmt.Sprintf("【测试异常】gwatch - %s - %s", util.GetDeviceName(), timeutil.FormatDateTime(timeutil.Now()))
}

// SendErrorReportEmail 发送异常退出报告邮件
func SendErrorReportEmail(errorMessage string) error {
	if !Config.Enabled {
		logger.Info("邮件发送已禁用，跳过")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		logger.Info("邮件配置未设置，跳过")
		return nil
	}

	subject := buildErrorSubject()

	var body strings.Builder
	body.WriteString("===== 测试异常报告 =====\n\n")
	body.WriteString(fmt.Sprintf("发生时间: %s\n", timeutil.FormatDateTime(timeutil.Now())))
	body.WriteString(fmt.Sprintf("测试设备: %s\n", util.GetDeviceName()))
	body.WriteString(fmt.Sprintf("\n异常信息:\n"))
	body.WriteString(fmt.Sprintf("  %s\n", errorMessage))
	body.WriteString("\n===== 报告结束 =====\n")
	body.WriteString("来自 gwatch 监控系统")

	logger.Info("正在发送错误报告邮件...")
	return SendEmail(subject, body.String())
}

// SendCustomEmail 发送自定义邮件
func SendCustomEmail(subject, body string) error {
	if !Config.Enabled {
		logger.Info("邮件发送已禁用，跳过")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		logger.Info("邮件配置未设置，跳过")
		return nil
	}

	logger.Info("正在发送自定义邮件...")
	return SendEmail(subject, body)
}