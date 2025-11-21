package risk

import "log/slog"

// AlertClient 抽象告警发送。
type AlertClient interface {
	Send(typ, msg string)
}

type Notifier struct {
	alert AlertClient
}

func NewNotifier(alert AlertClient) *Notifier {
	return &Notifier{alert: alert}
}

func (n *Notifier) NotifyLimitExceeded(symbol string, err error) {
	msg := "RiskLimitExceeded symbol=" + symbol
	if err != nil {
		msg += " err=" + err.Error()
	}
	slog.Warn(msg)
	if n.alert != nil {
		n.alert.Send("RiskLimit", msg)
	}
}

func (n *Notifier) NotifyCircuitTrip(span string, tickPrice float64) {
	msg := "CircuitBreakerTriggered span=" + span
	slog.Warn(msg, "price", tickPrice)
	if n.alert != nil {
		n.alert.Send("CircuitBreaker", msg)
	}
}
