package dto

// KillSwitchResponse is one durable kill-switch row: the module key, its
// intended state, and who set it last with why.
type KillSwitchResponse struct {
	Module    string `json:"module"`
	Enabled   bool   `json:"enabled"`
	UpdatedBy uint   `json:"updated_by"`
	UpdatedAt string `json:"updated_at"`
	Reason    string `json:"reason"`
}

type KillSwitchListResponse struct {
	Data []KillSwitchResponse `json:"data"`
}

// KillSwitchUpdateRequest flips one module. `enabled` is a pointer so an
// omitted field is a 400 (never an accidental disable); `reason` is
// mandatory and audit-logged; toggling `master` additionally requires
// `confirm` to be 'FREEZE' (disabling) or 'UNFREEZE' (enabling).
type KillSwitchUpdateRequest struct {
	Enabled *bool  `json:"enabled"`
	Reason  string `json:"reason"`
	Confirm string `json:"confirm"`
}
