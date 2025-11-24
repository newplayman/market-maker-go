package container

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"market-maker-go/infrastructure/logger"
)

// Lifecycle 生命周期接口
type Lifecycle interface {
	Start(ctx context.Context) error
	Stop() error
	Health() error
}

// LifecycleManager 生命周期管理器
type LifecycleManager struct {
	components []Lifecycle
	mu         sync.RWMutex
}

// NewLifecycleManager 创建新的生命周期管理器
func NewLifecycleManager() *LifecycleManager {
	return &LifecycleManager{
		components: make([]Lifecycle, 0),
	}
}

// Register 注册组件
func (m *LifecycleManager) Register(component Lifecycle) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.components = append(m.components, component)
}

// StartAll 按顺序启动所有组件
func (m *LifecycleManager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i, component := range m.components {
		if err := component.Start(ctx); err != nil {
			// 启动失败，回滚已启动的组件
			for j := i - 1; j >= 0; j-- {
				m.components[j].Stop()
			}
			return fmt.Errorf("start component %d failed: %w", i, err)
		}
	}
	return nil
}

// StopAll 逆序停止所有组件
func (m *LifecycleManager) StopAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var lastErr error
	// 逆序停止
	for i := len(m.components) - 1; i >= 0; i-- {
		if err := m.components[i].Stop(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// CheckHealth 检查所有组件健康状态
func (m *LifecycleManager) CheckHealth() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i, component := range m.components {
		if err := component.Health(); err != nil {
			return fmt.Errorf("component %d unhealthy: %w", i, err)
		}
	}
	return nil
}

// httpServerComponent HTTP服务器组件
type httpServerComponent struct {
	name    string
	handler http.Handler
	addr    string
	logger  *logger.Logger
	server  **http.Server
	started bool
	mu      sync.Mutex
}

func (h *httpServerComponent) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.started {
		return nil
	}

	srv := &http.Server{
		Addr:    h.addr,
		Handler: h.handler,
	}
	*h.server = srv

	// 在后台启动服务器
	go func() {
		h.logger.Logger.Info(fmt.Sprintf("%s listening on %s", h.name, h.addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			h.logger.LogError(err, map[string]interface{}{
				"component": h.name,
				"action":    "listen",
			})
		}
	}()

	h.started = true
	return nil
}

func (h *httpServerComponent) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.started || *h.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := (*h.server).Shutdown(ctx); err != nil {
		return fmt.Errorf("%s shutdown failed: %w", h.name, err)
	}

	h.logger.Logger.Info(fmt.Sprintf("%s stopped", h.name))
	h.started = false
	return nil
}

func (h *httpServerComponent) Health() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.started {
		return fmt.Errorf("%s not started", h.name)
	}
	return nil
}
