package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"market-maker-go/strategy"
)

// 简易回测脚本：读取 CSV (mid 价格为第一列，可选 CSV 路径参数)，输出报价曲线长度。
// 用法：
//
//	go run ./cmd/backtest mids.csv
//
// CSV 示例：每行一个 mid 价格数值（不含表头）
func main() {
	engine, err := strategy.NewEngine(strategy.EngineConfig{
		MinSpread:      0.001,
		TargetPosition: 0,
		MaxDrift:       1,
		BaseSize:       0.5,
	})
	if err != nil {
		panic(err)
	}

	path := "mids.csv"
	if len(os.Args) > 1 && os.Args[1] != "" {
		path = os.Args[1]
	}
	mids, err := loadMids(path)
	if err != nil {
		fmt.Printf("failed to read %s: %v\n", path, err)
		return
	}
	snaps := make([]strategy.MarketSnapshot, 0, len(mids))
	for _, m := range mids {
		snaps = append(snaps, strategy.MarketSnapshot{Mid: m})
	}
	quotes := engine.BacktestUpdate(snaps, nil)
	fmt.Printf("loaded mids=%d, generated quotes=%d\n", len(mids), len(quotes))
	if len(quotes) > 0 {
		fmt.Printf("first quote bid=%.4f ask=%.4f size=%.4f\n", quotes[0].Bid, quotes[0].Ask, quotes[0].Size)
	}

	// 简单统计：价差均值/最大回撤估算（基于 mid 演进的虚拟持仓）
	stats := computeStats(mids)
	fmt.Printf("stats: min=%.4f max=%.4f mean=%.4f maxDrawdown=%.4f%%\n", stats.Min, stats.Max, stats.Mean, stats.MaxDrawdownPct)
}

type statsResult struct {
	Min            float64
	Max            float64
	Mean           float64
	MaxDrawdownPct float64
}

func computeStats(series []float64) statsResult {
	if len(series) == 0 {
		return statsResult{}
	}
	min, max := series[0], series[0]
	sum := 0.0
	peak := series[0]
	maxDD := 0.0
	for _, v := range series {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		sum += v
		if v > peak {
			peak = v
		}
		if peak != 0 {
			dd := (peak - v) / peak * 100
			if dd > maxDD {
				maxDD = dd
			}
		}
	}
	return statsResult{
		Min:            min,
		Max:            max,
		Mean:           sum / float64(len(series)),
		MaxDrawdownPct: maxDD,
	}
}

func loadMids(path string) ([]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	out := make([]float64, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(row[0], 64)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}
