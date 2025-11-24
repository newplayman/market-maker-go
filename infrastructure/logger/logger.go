package logger

import (
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger 封装zap日志器，提供结构化日志功能
type Logger struct {
	*zap.Logger
	config Config
}

// Config 日志配置
type Config struct {
	Level      string   `yaml:"level"`       // debug, info, warn, error
	Outputs    []string `yaml:"outputs"`     // stdout, file
	OutputFile string   `yaml:"output_file"` // 日志文件路径
	ErrorFile  string   `yaml:"error_file"`  // 错误日志单独文件
	Format     string   `yaml:"format"`      // json 或 console
	MaxSize    int      `yaml:"max_size"`    // 单个日志文件最大MB
	MaxBackups int      `yaml:"max_backups"` // 保留的旧日志文件数
	MaxAge     int      `yaml:"max_age"`     // 保留天数
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Level:      "info",
		Outputs:    []string{"stdout"},
		Format:     "json",
		MaxSize:    100,
		MaxBackups: 3,
		MaxAge:     7,
	}
}

// New 创建新的Logger实例
func New(cfg Config) (*Logger, error) {
	// 解析日志级别
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level %s: %w", cfg.Level, err)
	}

	// 配置编码器
	var encoderConfig zapcore.EncoderConfig
	if cfg.Format == "console" {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		encoderConfig = zap.NewProductionEncoderConfig()
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	// 构建核心
	cores := []zapcore.Core{}

	// 标准输出
	if contains(cfg.Outputs, "stdout") {
		var encoder zapcore.Encoder
		if cfg.Format == "console" {
			encoder = zapcore.NewConsoleEncoder(encoderConfig)
		} else {
			encoder = zapcore.NewJSONEncoder(encoderConfig)
		}
		cores = append(cores, zapcore.NewCore(
			encoder,
			zapcore.AddSync(os.Stdout),
			level,
		))
	}

	// 文件输出
	if contains(cfg.Outputs, "file") && cfg.OutputFile != "" {
		fileWriter, err := os.OpenFile(cfg.OutputFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("open log file failed: %w", err)
		}
		
		encoder := zapcore.NewJSONEncoder(encoderConfig)
		cores = append(cores, zapcore.NewCore(
			encoder,
			zapcore.AddSync(fileWriter),
			level,
		))
	}

	// 错误日志单独文件
	if cfg.ErrorFile != "" {
		errorWriter, err := os.OpenFile(cfg.ErrorFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("open error log file failed: %w", err)
		}
		
		encoder := zapcore.NewJSONEncoder(encoderConfig)
		cores = append(cores, zapcore.NewCore(
			encoder,
			zapcore.AddSync(errorWriter),
			zapcore.ErrorLevel, // 只记录error及以上级别
		))
	}

	core := zapcore.NewTee(cores...)
	zapLogger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	return &Logger{
		Logger: zapLogger,
		config: cfg,
	}, nil
}

// WithFields 添加字段返回新的logger
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	zapFields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	return &Logger{
		Logger: l.Logger.With(zapFields...),
		config: l.config,
	}
}

// LogOrder 记录订单相关事件
func (l *Logger) LogOrder(event string, orderID string, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["event"] = event
	fields["order_id"] = orderID
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)

	zapFields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	l.Info("order_event", zapFields...)
}

// LogTrade 记录交易相关事件
func (l *Logger) LogTrade(event string, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["event"] = event
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)

	zapFields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	l.Info("trade_event", zapFields...)
}

// LogError 记录错误并附带上下文
func (l *Logger) LogError(err error, context map[string]interface{}) {
	if context == nil {
		context = make(map[string]interface{})
	}
	context["error"] = err.Error()
	context["ts"] = time.Now().UTC().Format(time.RFC3339Nano)

	zapFields := make([]zap.Field, 0, len(context))
	for k, v := range context {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	l.Error("error_event", zapFields...)
}

// LogRisk 记录风控事件
func (l *Logger) LogRisk(event string, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["event"] = event
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)

	zapFields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	l.Warn("risk_event", zapFields...)
}

// Close 关闭日志器
func (l *Logger) Close() error {
	return l.Sync()
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
