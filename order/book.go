package order

import "sync"

// Book 记录订单和状态，支持查询。
type Book struct {
	mu     sync.RWMutex
	orders map[string]Order
}

func NewBook() *Book {
	return &Book{orders: make(map[string]Order)}
}

func (b *Book) Set(o Order) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.orders[o.ID] = o
}

func (b *Book) Get(id string) (Order, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	o, ok := b.orders[id]
	return o, ok
}

// List 返回全部订单（拷贝）。
func (b *Book) List() []Order {
	b.mu.RLock()
	defer b.mu.RUnlock()
	res := make([]Order, 0, len(b.orders))
	for _, o := range b.orders {
		res = append(res, o)
	}
	return res
}
