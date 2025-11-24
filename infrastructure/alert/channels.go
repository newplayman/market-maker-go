package alert

import (
	"fmt"
	"log"
	"os"
)

// LogChannel 日志告警通道
type LogChannel struct {
	logger *log.Logger
	name   string
}

// NewLogChannel 创建日志告警通道
func NewLogChannel(name string, output *os.File) *LogChannel {
	if output == nil {
		output = os.Stdout
	}

	return &LogChannel{
		logger: log.New(output, "[ALERT] ", log.LstdFlags),
		name:   name,
	}
}

// Send 发送告警到日志
func (c *LogChannel) Send(alert Alert) error {
	// 格式化告警信息
	msg := fmt.Sprintf("[%s] %s", alert.Level, alert.Message)

	// 添加附加字段
	if len(alert.Fields) > 0 {
		msg += " | Fields: "
		for k, v := range alert.Fields {
			msg += fmt.Sprintf("%s=%v ", k, v)
		}
	}

	// 记录日志
	c.logger.Println(msg)
	return nil
}

// Name 返回通道名称
func (c *LogChannel) Name() string {
	return c.name
}

// ConsoleChannel 控制台告警通道（彩色输出）
type ConsoleChannel struct {
	name string
}

// NewConsoleChannel 创建控制台告警通道
func NewConsoleChannel(name string) *ConsoleChannel {
	return &ConsoleChannel{
		name: name,
	}
}

// Send 发送告警到控制台（带颜色）
func (c *ConsoleChannel) Send(alert Alert) error {
	// ANSI颜色代码
	colorReset := "\033[0m"
	colorCode := ""

	switch alert.Level {
	case "INFO":
		colorCode = "\033[32m" // 绿色
	case "WARNING":
		colorCode = "\033[33m" // 黄色
	case "ERROR":
		colorCode = "\033[31m" // 红色
	case "CRITICAL":
		colorCode = "\033[35m" // 紫色
	default:
		colorCode = colorReset
	}

	// 格式化消息
	msg := fmt.Sprintf("%s[%s]%s %s - %s",
		colorCode,
		alert.Level,
		colorReset,
		alert.Timestamp.Format("2006-01-02 15:04:05"),
		alert.Message,
	)

	// 添加字段
	if len(alert.Fields) > 0 {
		msg += " | "
		for k, v := range alert.Fields {
			msg += fmt.Sprintf("%s=%v ", k, v)
		}
	}

	fmt.Println(msg)
	return nil
}

// Name 返回通道名称
func (c *ConsoleChannel) Name() string {
	return c.name
}

// MockChannel 模拟告警通道（用于测试）
type MockChannel struct {
	name      string
	alerts    []Alert
	shouldErr bool
}

// NewMockChannel 创建模拟告警通道
func NewMockChannel(name string) *MockChannel {
	return &MockChannel{
		name:   name,
		alerts: make([]Alert, 0),
	}
}

// Send 记录告警（用于测试验证）
func (c *MockChannel) Send(alert Alert) error {
	if c.shouldErr {
		return fmt.Errorf("mock error")
	}
	c.alerts = append(c.alerts, alert)
	return nil
}

// Name 返回通道名称
func (c *MockChannel) Name() string {
	return c.name
}

// GetAlerts 获取所有接收到的告警
func (c *MockChannel) GetAlerts() []Alert {
	return c.alerts
}

// SetShouldError 设置是否返回错误
func (c *MockChannel) SetShouldError(shouldErr bool) {
	c.shouldErr = shouldErr
}

// Clear 清空告警记录
func (c *MockChannel) Clear() {
	c.alerts = make([]Alert, 0)
}

// Count 返回接收到的告警数量
func (c *MockChannel) Count() int {
	return len(c.alerts)
}
