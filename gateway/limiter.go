package gateway

import (
	"sync"
	"time"
)

// RateLimiter 控制请求速率，避免触发交易所限流。
type RateLimiter interface {
	Wait()
}

// TokenBucketLimiter 是一个简单的令牌桶实现。
type TokenBucketLimiter struct {
	rate   float64
	burst  int
	tokens float64
	last   time.Time
	mu     sync.Mutex
}

func NewTokenBucketLimiter(rate float64, burst int) *TokenBucketLimiter {
	if rate <= 0 {
		rate = 1
	}
	if burst <= 0 {
		burst = 1
	}
	return &TokenBucketLimiter{
		rate:   rate,
		burst:  burst,
		tokens: float64(burst),
		last:   time.Now(),
	}
}

func (l *TokenBucketLimiter) Wait() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(l.last).Seconds()
	l.last = now
	l.tokens += elapsed * l.rate
	if l.tokens > float64(l.burst) {
		l.tokens = float64(l.burst)
	}
	if l.tokens < 1 {
		sleep := time.Duration((1-l.tokens)/l.rate*float64(time.Second)) + time.Millisecond
		l.mu.Unlock()
		time.Sleep(sleep)
		l.mu.Lock()
		l.tokens = 0
	} else {
		l.tokens -= 1
	}
}
