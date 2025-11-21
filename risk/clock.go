package risk

import "time"

// Clock 抽象时间便于测试。
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// NowUTC 默认使用 UTC 时间。
var NowUTC Clock = realClock{}
