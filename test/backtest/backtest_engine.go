package backtest

import (
	"fmt"
	"sort"
	"time"

	"market-maker-go/internal/strategy"
	"market-maker-go/inventory"
)

// PriceData 历史价格数据
type PriceData struct {
	Timestamp time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
}

// Trade 回测交易记录
type Trade struct {
	Timestamp time.Time
	Side      string // "BUY" or "SELL"
	Price     float64
	Size      float64
	PnL       float64 // 已实现盈亏
}

// BacktestResult 回测结果
type BacktestResult struct {
	StartTime      time.Time
	EndTime        time.Time
	InitialBalance float64
	FinalBalance   float64
	TotalPnL       float64
	TotalReturn    float64 // 总收益率

	TotalTrades   int
	WinningTrades int
	LosingTrades  int
	WinRate       float64

	MaxDrawdown float64
	SharpeRatio float64

	Trades      []Trade
	EquityCurve []float64
	Timestamps  []time.Time
}

// BacktestConfig 回测配置
type BacktestConfig struct {
	InitialBalance float64         // 初始资金
	TakerFee       float64         // 手续费率（如0.001 = 0.1%）
	SlippageRate   float64         // 滑点率（如0.0001 = 0.01%）
	StrategyConfig strategy.Config // 策略配置
}

// BacktestEngine 回测引擎
type BacktestEngine struct {
	config    BacktestConfig
	strategy  *strategy.BasicMarketMaking
	inventory *inventory.Tracker

	balance     float64
	trades      []Trade
	equityCurve []float64
	timestamps  []time.Time
	peakEquity  float64
	maxDrawdown float64
}

// NewBacktestEngine 创建回测引擎
func NewBacktestEngine(config BacktestConfig) *BacktestEngine {
	// 设置默认值
	if config.InitialBalance <= 0 {
		config.InitialBalance = 10000.0
	}
	if config.TakerFee <= 0 {
		config.TakerFee = 0.001 // 0.1%
	}
	if config.SlippageRate < 0 {
		config.SlippageRate = 0.0001 // 0.01%
	}

	return &BacktestEngine{
		config:      config,
		strategy:    strategy.NewBasicMarketMaking(config.StrategyConfig),
		inventory:   &inventory.Tracker{},
		balance:     config.InitialBalance,
		trades:      make([]Trade, 0),
		equityCurve: make([]float64, 0),
		timestamps:  make([]time.Time, 0),
		peakEquity:  config.InitialBalance,
		maxDrawdown: 0,
	}
}

// Run 运行回测
func (e *BacktestEngine) Run(priceData []PriceData) (*BacktestResult, error) {
	if len(priceData) == 0 {
		return nil, fmt.Errorf("no price data provided")
	}

	// 确保数据按时间排序
	sort.Slice(priceData, func(i, j int) bool {
		return priceData[i].Timestamp.Before(priceData[j].Timestamp)
	})

	startTime := priceData[0].Timestamp
	endTime := priceData[len(priceData)-1].Timestamp

	// 逐个时间点执行策略
	for _, data := range priceData {
		e.processBar(data)
	}

	// 计算最终结果
	result := e.calculateResult(startTime, endTime)

	return result, nil
}

// processBar 处理单个K线数据
func (e *BacktestEngine) processBar(data PriceData) {
	mid := (data.High + data.Low) / 2.0
	position := e.inventory.NetExposure()

	// 生成报价
	ctx := strategy.Context{
		Symbol:       "ETHUSDC",
		Mid:          mid,
		Inventory:    position,
		MaxInventory: e.config.StrategyConfig.MaxInventory,
	}

	quotes, err := e.strategy.GenerateQuotes(ctx)
	if err != nil {
		return
	}

	// 模拟订单成交
	// 简化处理：假设买单在low附近成交，卖单在high附近成交
	for _, quote := range quotes {
		if e.shouldFill(quote, data) {
			e.executeTrade(quote, data)
		}
	}

	// 记录权益曲线
	currentEquity := e.calculateEquity(mid)
	e.equityCurve = append(e.equityCurve, currentEquity)
	e.timestamps = append(e.timestamps, data.Timestamp)

	// 更新回撤
	if currentEquity > e.peakEquity {
		e.peakEquity = currentEquity
	}
	drawdown := (e.peakEquity - currentEquity) / e.peakEquity
	if drawdown > e.maxDrawdown {
		e.maxDrawdown = drawdown
	}
}

// shouldFill 判断订单是否成交
func (e *BacktestEngine) shouldFill(quote strategy.Quote, data PriceData) bool {
	// 简化的成交逻辑：
	// 买单价格 >= Low，则可能成交
	// 卖单价格 <= High，则可能成交
	if quote.Side == "BUY" {
		return quote.Price >= data.Low
	} else if quote.Side == "SELL" {
		return quote.Price <= data.High
	}
	return false
}

// executeTrade 执行交易
func (e *BacktestEngine) executeTrade(quote strategy.Quote, data PriceData) {
	// 计算成交价格（考虑滑点）
	fillPrice := quote.Price
	if quote.Side == "BUY" {
		fillPrice *= (1 + e.config.SlippageRate)
	} else {
		fillPrice *= (1 - e.config.SlippageRate)
	}

	// 计算手续费
	fee := fillPrice * quote.Size * e.config.TakerFee

	// 更新余额
	if quote.Side == "BUY" {
		e.balance -= (fillPrice*quote.Size + fee)
	} else {
		e.balance += (fillPrice*quote.Size - fee)
	}

	// 计算已实现盈亏
	realizedPnL := 0.0
	oldPosition := e.inventory.NetExposure()

	// 更新库存
	if quote.Side == "BUY" {
		e.inventory.Update(quote.Size, fillPrice)
	} else {
		e.inventory.Update(-quote.Size, fillPrice)
	}

	// 如果是平仓交易，计算盈亏
	if (oldPosition > 0 && quote.Side == "SELL") || (oldPosition < 0 && quote.Side == "BUY") {
		// 计算平仓部分的盈亏
		avgCost := e.inventory.AvgCost()
		if avgCost > 0 {
			if quote.Side == "SELL" {
				realizedPnL = (fillPrice - avgCost) * quote.Size
			} else {
				realizedPnL = (avgCost - fillPrice) * quote.Size
			}
		}
	}

	// 记录交易
	trade := Trade{
		Timestamp: data.Timestamp,
		Side:      quote.Side,
		Price:     fillPrice,
		Size:      quote.Size,
		PnL:       realizedPnL - fee,
	}
	e.trades = append(e.trades, trade)

	// 通知策略
	e.strategy.OnFill(strategy.Fill{
		Side:  quote.Side,
		Price: fillPrice,
		Size:  quote.Size,
	})
}

// calculateEquity 计算当前权益
func (e *BacktestEngine) calculateEquity(currentPrice float64) float64 {
	position := e.inventory.NetExposure()
	positionValue := position * currentPrice
	return e.balance + positionValue
}

// calculateResult 计算回测结果
func (e *BacktestEngine) calculateResult(startTime, endTime time.Time) *BacktestResult {
	finalEquity := e.balance
	if len(e.equityCurve) > 0 {
		finalEquity = e.equityCurve[len(e.equityCurve)-1]
	}

	totalPnL := finalEquity - e.config.InitialBalance
	totalReturn := totalPnL / e.config.InitialBalance

	// 计算胜率
	winningTrades := 0
	losingTrades := 0
	for _, trade := range e.trades {
		if trade.PnL > 0 {
			winningTrades++
		} else if trade.PnL < 0 {
			losingTrades++
		}
	}

	winRate := 0.0
	if len(e.trades) > 0 {
		winRate = float64(winningTrades) / float64(len(e.trades))
	}

	// 计算夏普比率
	sharpeRatio := e.calculateSharpeRatio()

	return &BacktestResult{
		StartTime:      startTime,
		EndTime:        endTime,
		InitialBalance: e.config.InitialBalance,
		FinalBalance:   finalEquity,
		TotalPnL:       totalPnL,
		TotalReturn:    totalReturn,
		TotalTrades:    len(e.trades),
		WinningTrades:  winningTrades,
		LosingTrades:   losingTrades,
		WinRate:        winRate,
		MaxDrawdown:    e.maxDrawdown,
		SharpeRatio:    sharpeRatio,
		Trades:         e.trades,
		EquityCurve:    e.equityCurve,
		Timestamps:     e.timestamps,
	}
}

// calculateSharpeRatio 计算夏普比率
func (e *BacktestEngine) calculateSharpeRatio() float64 {
	if len(e.equityCurve) < 2 {
		return 0
	}

	// 计算收益率序列
	returns := make([]float64, len(e.equityCurve)-1)
	for i := 1; i < len(e.equityCurve); i++ {
		returns[i-1] = (e.equityCurve[i] - e.equityCurve[i-1]) / e.equityCurve[i-1]
	}

	// 计算平均收益率
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))

	// 计算标准差
	variance := 0.0
	for _, r := range returns {
		diff := r - mean
		variance += diff * diff
	}
	variance /= float64(len(returns))
	stdDev := 0.0
	if variance > 0 {
		stdDev = 1.0
		for i := 0; i < 10; i++ {
			stdDev = (stdDev + variance/stdDev) / 2
		}
	}

	// 夏普比率 = 平均收益率 / 标准差
	// 假设无风险利率为0
	if stdDev > 0 {
		// 年化夏普比率（假设每天一个数据点）
		annualizedReturn := mean * 365
		annualizedStdDev := stdDev * 15.87 // sqrt(252) 约等于 15.87
		return annualizedReturn / annualizedStdDev
	}

	return 0
}

// PrintResult 打印回测结果
func (r *BacktestResult) PrintResult() {
	fmt.Println("=== 回测结果 ===")
	fmt.Printf("时间范围: %s - %s\n", r.StartTime.Format("2006-01-02"), r.EndTime.Format("2006-01-02"))
	fmt.Printf("初始资金: %.2f USDC\n", r.InitialBalance)
	fmt.Printf("最终资金: %.2f USDC\n", r.FinalBalance)
	fmt.Printf("总盈亏: %.2f USDC (%.2f%%)\n", r.TotalPnL, r.TotalReturn*100)
	fmt.Printf("\n")
	fmt.Printf("总交易次数: %d\n", r.TotalTrades)
	fmt.Printf("盈利交易: %d\n", r.WinningTrades)
	fmt.Printf("亏损交易: %d\n", r.LosingTrades)
	fmt.Printf("胜率: %.2f%%\n", r.WinRate*100)
	fmt.Printf("\n")
	fmt.Printf("最大回撤: %.2f%%\n", r.MaxDrawdown*100)
	fmt.Printf("夏普比率: %.2f\n", r.SharpeRatio)
	fmt.Println("================")
}
