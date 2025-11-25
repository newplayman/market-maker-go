package strategy

import (
	"errors"
	"market-maker-go/strategy/asmm"
)

// StrategyFactory creates strategy instances based on configuration.
type StrategyFactory struct{}

// NewStrategyFactory creates a new StrategyFactory.
func NewStrategyFactory() *StrategyFactory {
	return &StrategyFactory{}
}

// CreateStrategy creates a strategy instance based on the type and configuration.
func (f *StrategyFactory) CreateStrategy(strategyType string, config interface{}) (interface{}, error) {
	switch StrategyType(strategyType) {
	case GridStrategy:
		if cfg, ok := config.(EngineConfig); ok {
			return NewEngine(cfg)
		}
		return nil, errors.New("invalid grid strategy config")
	case ASMMStrategy:
		if cfg, ok := config.(asmm.ASMMConfig); ok {
			if !cfg.Validate() {
				return nil, errors.New("invalid ASMM strategy config")
			}
			return asmm.NewASMMStrategy(cfg), nil
		}
		return nil, errors.New("invalid ASMM strategy config")
	default:
		return nil, errors.New("unknown strategy type: " + strategyType)
	}
}