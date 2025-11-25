package strategy

import (
	"testing"
	"market-maker-go/strategy/asmm"
)

func TestStrategyFactory_CreateGridStrategy(t *testing.T) {
	factory := NewStrategyFactory()
	
	config := EngineConfig{
		MinSpread:      0.001,
		TargetPosition: 0,
		MaxDrift:       1,
		BaseSize:       1,
	}
	
	strategy, err := factory.CreateStrategy("grid", config)
	if err != nil {
		t.Errorf("Failed to create grid strategy: %v", err)
	}
	
	if strategy == nil {
		t.Error("Grid strategy should not be nil")
	}
	
	engine, ok := strategy.(*Engine)
	if !ok {
		t.Error("Strategy should be of type *Engine")
	}
	
	if engine == nil {
		t.Error("Engine should not be nil")
	}
}

func TestStrategyFactory_CreateASMMStrategy(t *testing.T) {
	factory := NewStrategyFactory()
	
	config := asmm.DefaultASMMConfig()
	
	strategy, err := factory.CreateStrategy("asmm", config)
	if err != nil {
		t.Errorf("Failed to create ASMM strategy: %v", err)
	}
	
	if strategy == nil {
		t.Error("ASMM strategy should not be nil")
	}
	
	asmmStrategy, ok := strategy.(*asmm.ASMMStrategy)
	if !ok {
		t.Error("Strategy should be of type *asmm.ASMMStrategy")
	}
	
	if asmmStrategy == nil {
		t.Error("ASMM strategy should not be nil")
	}
}

func TestStrategyFactory_CreateInvalidStrategy(t *testing.T) {
	factory := NewStrategyFactory()
	
	_, err := factory.CreateStrategy("invalid", nil)
	if err == nil {
		t.Error("Expected error for invalid strategy type")
	}
}