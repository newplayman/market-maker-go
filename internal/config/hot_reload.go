package config

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// HotReloadConfig 热更新配置
type HotReloadConfig struct {
	Enabled       bool          // 是否启用热更新
	WatchInterval time.Duration // 监听间隔
	CooldownTime  time.Duration // 冷却时间，避免频繁更新
}

// DefaultHotReloadConfig 默认热更新配置
func DefaultHotReloadConfig() HotReloadConfig {
	return HotReloadConfig{
		Enabled:       true,
		WatchInterval: 1 * time.Second,
		CooldownTime:  5 * time.Second,
	}
}

// ParameterValidator 参数验证器接口
type ParameterValidator interface {
	Validate(params map[string]interface{}) error
}

// ParameterApplier 参数应用器接口
type ParameterApplier interface {
	ApplyParameters(params map[string]interface{}) error
}

// HotReloader 配置热更新器
type HotReloader struct {
	config        HotReloadConfig
	configPath    string
	watcher       *fsnotify.Watcher
	validators    map[string]ParameterValidator
	appliers      map[string]ParameterApplier
	lastReload    time.Time
	mu            sync.RWMutex
	stopChan      chan struct{}
	doneChan      chan struct{}
	reloadHandler func(newConfig interface{}) error
}

// NewHotReloader 创建热更新器
func NewHotReloader(configPath string, cfg HotReloadConfig) (*HotReloader, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	return &HotReloader{
		config:     cfg,
		configPath: configPath,
		watcher:    watcher,
		validators: make(map[string]ParameterValidator),
		appliers:   make(map[string]ParameterApplier),
		stopChan:   make(chan struct{}),
		doneChan:   make(chan struct{}),
	}, nil
}

// RegisterValidator 注册参数验证器
func (h *HotReloader) RegisterValidator(name string, validator ParameterValidator) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.validators[name] = validator
}

// RegisterApplier 注册参数应用器
func (h *HotReloader) RegisterApplier(name string, applier ParameterApplier) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.appliers[name] = applier
}

// SetReloadHandler 设置重载处理函数
func (h *HotReloader) SetReloadHandler(handler func(newConfig interface{}) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reloadHandler = handler
}

// Start 启动热更新监听
func (h *HotReloader) Start(ctx context.Context) error {
	if !h.config.Enabled {
		return nil
	}

	// 添加配置文件到监听
	if err := h.watcher.Add(h.configPath); err != nil {
		return fmt.Errorf("failed to watch config file: %w", err)
	}

	go h.watch(ctx)

	return nil
}

// Stop 停止热更新
func (h *HotReloader) Stop() error {
	if !h.config.Enabled {
		// 如果没有启用，直接关闭 watcher
		if h.watcher != nil {
			return h.watcher.Close()
		}
		return nil
	}

	// 发送停止信号
	select {
	case <-h.stopChan:
		// 已经停止
	default:
		close(h.stopChan)
	}

	// 等待 goroutine 结束（带超时）
	select {
	case <-h.doneChan:
		// 正常结束
	case <-time.After(1 * time.Second):
		// 超时，可能 watch goroutine 没有启动
	}

	if h.watcher != nil {
		return h.watcher.Close()
	}

	return nil
}

// watch 监听文件变化
func (h *HotReloader) watch(ctx context.Context) {
	defer close(h.doneChan)

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopChan:
			return
		case event, ok := <-h.watcher.Events:
			if !ok {
				return
			}

			// 只处理写入和创建事件
			if event.Op&fsnotify.Write == fsnotify.Write ||
				event.Op&fsnotify.Create == fsnotify.Create {
				h.handleConfigChange()
			}

		case err, ok := <-h.watcher.Errors:
			if !ok {
				return
			}
			// 记录错误但继续监听
			fmt.Printf("Watcher error: %v\n", err)
		}
	}
}

// handleConfigChange 处理配置变化
func (h *HotReloader) handleConfigChange() {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 检查冷却时间
	if time.Since(h.lastReload) < h.config.CooldownTime {
		return
	}

	// 重新加载配置
	if h.reloadHandler != nil {
		if err := h.reloadHandler(nil); err != nil {
			fmt.Printf("Failed to reload config: %v\n", err)
			return
		}
	}

	h.lastReload = time.Now()
}

// ValidateParameters 验证参数
func (h *HotReloader) ValidateParameters(category string, params map[string]interface{}) error {
	h.mu.RLock()
	validator, ok := h.validators[category]
	h.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no validator registered for category: %s", category)
	}

	return validator.Validate(params)
}

// ApplyParameters 应用参数
func (h *HotReloader) ApplyParameters(category string, params map[string]interface{}) error {
	// 先验证
	if err := h.ValidateParameters(category, params); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// 再应用
	h.mu.RLock()
	applier, ok := h.appliers[category]
	h.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no applier registered for category: %s", category)
	}

	return applier.ApplyParameters(params)
}

// GetLastReloadTime 获取最后重载时间
func (h *HotReloader) GetLastReloadTime() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastReload
}

// StrategyParameterValidator 策略参数验证器
type StrategyParameterValidator struct{}

func (v *StrategyParameterValidator) Validate(params map[string]interface{}) error {
	// 验证 base_spread
	if spread, ok := params["base_spread"].(float64); ok {
		if spread <= 0 || spread > 0.1 {
			return fmt.Errorf("base_spread must be between 0 and 0.1, got %f", spread)
		}
	}

	// 验证 base_size
	if size, ok := params["base_size"].(float64); ok {
		if size <= 0 || size > 10.0 {
			return fmt.Errorf("base_size must be between 0 and 10, got %f", size)
		}
	}

	// 验证 max_inventory
	if maxInv, ok := params["max_inventory"].(float64); ok {
		if maxInv <= 0 || maxInv > 100.0 {
			return fmt.Errorf("max_inventory must be between 0 and 100, got %f", maxInv)
		}
	}

	// 验证 skew_factor
	if skew, ok := params["skew_factor"].(float64); ok {
		if skew < 0 || skew > 1.0 {
			return fmt.Errorf("skew_factor must be between 0 and 1, got %f", skew)
		}
	}

	return nil
}

// RiskParameterValidator 风控参数验证器
type RiskParameterValidator struct{}

func (v *RiskParameterValidator) Validate(params map[string]interface{}) error {
	// 验证 daily_loss_limit
	if limit, ok := params["daily_loss_limit"].(float64); ok {
		if limit <= 0 {
			return fmt.Errorf("daily_loss_limit must be positive, got %f", limit)
		}
	}

	// 验证 max_drawdown_limit
	if limit, ok := params["max_drawdown_limit"].(float64); ok {
		if limit <= 0 || limit > 1.0 {
			return fmt.Errorf("max_drawdown_limit must be between 0 and 1, got %f", limit)
		}
	}

	// 验证 circuit_breaker_threshold
	if threshold, ok := params["circuit_breaker_threshold"].(int); ok {
		if threshold <= 0 || threshold > 100 {
			return fmt.Errorf("circuit_breaker_threshold must be between 0 and 100, got %d", threshold)
		}
	}

	return nil
}

// AlertParameterValidator 告警参数验证器
type AlertParameterValidator struct{}

func (v *AlertParameterValidator) Validate(params map[string]interface{}) error {
	// 验证 throttle_interval
	if interval, ok := params["throttle_interval"].(string); ok {
		if _, err := time.ParseDuration(interval); err != nil {
			return fmt.Errorf("invalid throttle_interval: %w", err)
		}
	}

	return nil
}
