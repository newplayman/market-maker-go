package inventory

// Sync 懒实现：外部可定期调用以获取当前仓位快照。
type Sync struct {
	Tracker *Tracker
}

func (s *Sync) Snapshot(mid float64) (net float64, pnl float64) {
	if s.Tracker == nil {
		return 0, 0
	}
	return s.Tracker.Valuation(mid)
}
