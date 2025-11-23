package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"market-maker-go/config"
	"market-maker-go/strategy"
)

type summary struct {
	Symbol         string
	Count          int
	Min            float64
	Max            float64
	Mean           float64
	MaxDrawdownPct float64
	FirstBid       float64
	FirstAsk       float64
}

// 支持多 symbol + 配置驱动的回测脚本。
// 用法：
//
//	go run ./cmd/backtest -config configs/config.yaml -symbols ETHUSDC:data/mids_sample.csv,BTCUSDC:data/btc.csv -out summaries.csv
func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	symbolFiles := flag.String("symbols", "ETHUSDC:data/mids_sample.csv", "symbol=csv 列表，逗号分隔")
	outPath := flag.String("out", "", "若指定则写入 CSV 汇总")
	flag.Parse()

	cfg, err := config.LoadWithEnvOverrides(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	entries := parseSymbolFiles(*symbolFiles)
	if len(entries) == 0 {
		log.Fatal("未指定任何 symbol=csv")
	}

	var summaries []summary
	for _, entry := range entries {
		sym := strings.ToUpper(entry.symbol)
		conf, ok := cfg.Symbols[sym]
		if !ok {
			log.Printf("symbol %s 不在配置中，跳过", sym)
			continue
		}
		strat := conf.Strategy
		engine, err := strategy.NewEngine(strategy.EngineConfig{
			MinSpread:      strat.MinSpread,
			TargetPosition: strat.TargetPosition,
			MaxDrift:       strat.MaxDrift,
			BaseSize:       strat.BaseSize,
		})
		if err != nil {
			log.Printf("symbol %s 初始化策略失败: %v", sym, err)
			continue
		}

		mids, err := loadMids(entry.path)
		if err != nil {
			log.Printf("symbol %s 读取 %s 失败: %v", sym, entry.path, err)
			continue
		}
		if len(mids) == 0 {
			log.Printf("symbol %s 数据为空: %s", sym, entry.path)
			continue
		}
		snaps := make([]strategy.MarketSnapshot, 0, len(mids))
		for _, m := range mids {
			snaps = append(snaps, strategy.MarketSnapshot{Mid: m})
		}
		quotes := engine.BacktestUpdate(snaps, nil)
		stats := computeStats(mids)
		sum := summary{
			Symbol:         sym,
			Count:          len(mids),
			Min:            stats.Min,
			Max:            stats.Max,
			Mean:           stats.Mean,
			MaxDrawdownPct: stats.MaxDrawdownPct,
		}
		if len(quotes) > 0 {
			sum.FirstBid = quotes[0].Bid
			sum.FirstAsk = quotes[0].Ask
		}
		log.Printf("symbol=%s mids=%d quotes=%d min=%.4f max=%.4f mean=%.4f maxDD=%.4f%%",
			sym, len(mids), len(quotes), sum.Min, sum.Max, sum.Mean, sum.MaxDrawdownPct)
		summaries = append(summaries, sum)
	}

	if *outPath != "" {
		if err := writeSummaryCSV(*outPath, summaries); err != nil {
			log.Printf("写入汇总 CSV 失败: %v", err)
		} else {
			log.Printf("已写入汇总: %s", *outPath)
		}
	}
}

type symbolFile struct {
	symbol string
	path   string
}

func parseSymbolFiles(arg string) []symbolFile {
	if strings.TrimSpace(arg) == "" {
		return nil
	}
	parts := strings.Split(arg, ",")
	var out []symbolFile
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		items := strings.SplitN(p, ":", 2)
		if len(items) != 2 {
			continue
		}
		out = append(out, symbolFile{symbol: strings.TrimSpace(items[0]), path: strings.TrimSpace(items[1])})
	}
	return out
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

func writeSummaryCSV(path string, sums []summary) error {
	if len(sums) == 0 {
		return fmt.Errorf("no summary data")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	header := []string{"symbol", "count", "min", "max", "mean", "maxDrawdownPct", "firstBid", "firstAsk"}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, s := range sums {
		record := []string{
			s.Symbol,
			fmt.Sprintf("%d", s.Count),
			fmt.Sprintf("%.6f", s.Min),
			fmt.Sprintf("%.6f", s.Max),
			fmt.Sprintf("%.6f", s.Mean),
			fmt.Sprintf("%.6f", s.MaxDrawdownPct),
			fmt.Sprintf("%.6f", s.FirstBid),
			fmt.Sprintf("%.6f", s.FirstAsk),
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	return nil
}
