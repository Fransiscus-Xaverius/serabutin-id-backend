package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

// TDD guard: spec 003.history-penawaran-pekerjaan-mitra requires
// `updated_at` in BidResponse so the mitra history "Penawaran" tab
// can render the diagram's "last update" timestamp. If this field
// disappears, the UI shows stale `created_at` instead.
func TestBidResponse_JSONIncludesUpdatedAt(t *testing.T) {
	resp := BidResponse{
		ID:        42,
		ServiceID: 7,
		Price:     15000,
		Status:    "pending",
		CreatedAt: "2026-07-27T10:00:00Z",
		UpdatedAt: "2026-07-27T10:05:00Z",
	}

	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"updated_at":"2026-07-27T10:05:00Z"`) {
		t.Errorf("expected updated_at in JSON, got: %s", out)
	}
}