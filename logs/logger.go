package logs

import "log/slog"

// Logger 提供统一的结构化日志入口，默认使用 slog。
type Logger interface {
	Warn(msg string, args ...any)
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}

type slogWrapper struct{}

func (s slogWrapper) Warn(msg string, args ...any)  { slog.Warn(msg, args...) }
func (s slogWrapper) Info(msg string, args ...any)  { slog.Info(msg, args...) }
func (s slogWrapper) Error(msg string, args ...any) { slog.Error(msg, args...) }

// DefaultLogger 可在不同模块注入，便于替换。
var DefaultLogger Logger = slogWrapper{}
