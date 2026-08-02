package usecase

// shouldAutoAccept returns true when the bid price matches the Mitra's
// auto-accept rule (spec 002.auto-accept-penawaran-mitra):
//
//   - Profile must have a budget_min set (nil = auto-accept off).
//   - bid.Price must be <= profile.BudgetMin (inclusive per user decision).
//   - budget_max, if set, does not gate the rule — the lower bound is the
//     trigger; offers above the floor stay manual.
//
// Diagram reference (Ojol node): "range ku adalah 10000 - 20000 tapi
// kalau ada yang offer 10000 langsung acc".
func shouldAutoAccept(bidPrice int64, budgetMin, budgetMax *int64) bool {
	if budgetMin == nil || budgetMax == nil {
		return false
	}
	return bidPrice <= *budgetMin
}