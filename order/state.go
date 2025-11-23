package order

// Status represents order lifecycle.
type Status string

const (
	StatusNew      Status = "NEW"
	StatusAck      Status = "ACK"
	StatusPartial  Status = "PARTIAL"
	StatusFilled   Status = "FILLED"
	StatusCanceled Status = "CANCELED"
	StatusRejected Status = "REJECTED"
	StatusExpired  Status = "EXPIRED"
)

// Order holds a simplified order view.
type Order struct {
	ID          string
	Symbol      string
	Side        string // BUY/SELL
	Type        string // LIMIT/MARKET
	Price       float64
	Quantity    float64
	Status      Status
	ClientID    string
	LastError   string
	ReduceOnly  bool
	PostOnly    bool
	TimeInForce string
}
