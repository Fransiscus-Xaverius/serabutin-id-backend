package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
)

// Ticket 12: kill-switch store — DB durable intent + Redis runtime source,
// master freeze ORs with per-module toggles, fail-open on outages.

type fakeKillSwitchRepo struct {
	rows    map[string]*entity.KillSwitch
	upserts []*entity.KillSwitch
	getErr  map[string]error
	listErr error
}

func newFakeKillSwitchRepo() *fakeKillSwitchRepo {
	return &fakeKillSwitchRepo{rows: map[string]*entity.KillSwitch{}, getErr: map[string]error{}}
}

func (f *fakeKillSwitchRepo) List(ctx context.Context) ([]*entity.KillSwitch, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]*entity.KillSwitch, 0, len(f.rows))
	for _, m := range entity.KillSwitchModules {
		if sw, ok := f.rows[m]; ok {
			out = append(out, sw)
		}
	}
	return out, nil
}

func (f *fakeKillSwitchRepo) Get(ctx context.Context, module string) (*entity.KillSwitch, error) {
	if err, ok := f.getErr[module]; ok {
		return nil, err
	}
	sw, ok := f.rows[module]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "KILL_SWITCH_NOT_FOUND", "kill switch not found")
	}
	return sw, nil
}

func (f *fakeKillSwitchRepo) Upsert(ctx context.Context, sw *entity.KillSwitch) error {
	cp := *sw
	f.rows[sw.Module] = &cp
	f.upserts = append(f.upserts, &cp)
	return nil
}

type fakeKillSwitchCache struct {
	vals   map[string]string
	getErr error
	setErr error
	sets   map[string]string
}

func newFakeKillSwitchCache() *fakeKillSwitchCache {
	return &fakeKillSwitchCache{vals: map[string]string{}, sets: map[string]string{}}
}

func (f *fakeKillSwitchCache) Get(ctx context.Context, key string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	v, ok := f.vals[key]
	if !ok {
		return "", errors.New("killswitch: cache miss")
	}
	return v, nil
}

func (f *fakeKillSwitchCache) Set(ctx context.Context, key, value string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.sets[key] = value
	f.vals[key] = value
	return nil
}

func seedAllEnabled() *fakeKillSwitchRepo {
	repo := newFakeKillSwitchRepo()
	for _, m := range entity.KillSwitchModules {
		repo.rows[m] = &entity.KillSwitch{Module: m, Enabled: true, UpdatedBy: 7}
	}
	return repo
}

func TestKillSwitch_IsEnabled_AllOn(t *testing.T) {
	ctx := context.Background()
	uc := NewKillSwitchUsecase(seedAllEnabled(), newFakeKillSwitchCache())
	for _, m := range []string{"bookings", "chat", "registration", "broadcasts"} {
		if !uc.IsEnabled(ctx, m) {
			t.Errorf("IsEnabled(%q) = false, want true", m)
		}
	}
}

func TestKillSwitch_ModuleOff_BlocksOnlyThatModule(t *testing.T) {
	ctx := context.Background()
	repo := seedAllEnabled()
	repo.rows["bookings"].Enabled = false
	uc := NewKillSwitchUsecase(repo, newFakeKillSwitchCache())

	if uc.IsEnabled(ctx, "bookings") {
		t.Error("IsEnabled(bookings) = true, want false")
	}
	for _, m := range []string{"chat", "registration", "broadcasts"} {
		if !uc.IsEnabled(ctx, m) {
			t.Errorf("IsEnabled(%q) = false, want true", m)
		}
	}
}

func TestKillSwitch_MasterOff_FreezesAll(t *testing.T) {
	ctx := context.Background()
	repo := seedAllEnabled()
	repo.rows["master"].Enabled = false
	uc := NewKillSwitchUsecase(repo, newFakeKillSwitchCache())

	for _, m := range []string{"bookings", "chat", "registration", "broadcasts"} {
		if uc.IsEnabled(ctx, m) {
			t.Errorf("IsEnabled(%q) = true under master freeze, want false", m)
		}
	}
}

func TestKillSwitch_RedisHit_WinsOverDB(t *testing.T) {
	ctx := context.Background()
	repo := seedAllEnabled()
	repo.rows["bookings"].Enabled = false // stale DB row
	cache := newFakeKillSwitchCache()
	cache.vals["killswitch:master"] = "1"
	cache.vals["killswitch:bookings"] = "1" // fresh runtime state
	uc := NewKillSwitchUsecase(repo, cache)

	if !uc.IsEnabled(ctx, "bookings") {
		t.Error("IsEnabled(bookings) = false, want true (Redis runtime wins)")
	}
}

func TestKillSwitch_RedisMiss_FallsBackToDB(t *testing.T) {
	ctx := context.Background()
	repo := seedAllEnabled()
	repo.rows["chat"].Enabled = false
	uc := NewKillSwitchUsecase(repo, newFakeKillSwitchCache()) // empty cache = all miss

	if uc.IsEnabled(ctx, "chat") {
		t.Error("IsEnabled(chat) = true, want false (DB fallback)")
	}
	if !uc.IsEnabled(ctx, "bookings") {
		t.Error("IsEnabled(bookings) = false, want true (DB fallback)")
	}
}

func TestKillSwitch_TotalOutage_FailsOpen(t *testing.T) {
	ctx := context.Background()
	repo := newFakeKillSwitchRepo()
	repo.listErr = errors.New("db down")
	repo.getErr = map[string]error{
		"master":   errors.New("db down"),
		"bookings": errors.New("db down"),
	}
	cache := newFakeKillSwitchCache()
	cache.getErr = errors.New("redis down")
	uc := NewKillSwitchUsecase(repo, cache)

	// A Redis+DB outage must never freeze the platform: fail open.
	if !uc.IsEnabled(ctx, "bookings") {
		t.Error("IsEnabled(bookings) = false during outage, want true (fail-open)")
	}
}

func TestKillSwitch_SetEnabled_RequiresReason(t *testing.T) {
	ctx := context.Background()
	uc := NewKillSwitchUsecase(seedAllEnabled(), newFakeKillSwitchCache())

	for _, reason := range []string{"", "   "} {
		_, err := uc.SetEnabled(ctx, 7, "bookings", false, reason, "")
		if err == nil {
			t.Errorf("SetEnabled with reason %q: want error, got nil", reason)
		}
	}
}

func TestKillSwitch_SetEnabled_RejectsUnknownModule(t *testing.T) {
	ctx := context.Background()
	uc := NewKillSwitchUsecase(seedAllEnabled(), newFakeKillSwitchCache())

	if _, err := uc.SetEnabled(ctx, 7, "payments", false, "fraud", ""); err == nil {
		t.Error("SetEnabled(payments): want error, got nil")
	}
}

func TestKillSwitch_SetEnabled_MasterRequiresConfirm(t *testing.T) {
	ctx := context.Background()
	uc := NewKillSwitchUsecase(seedAllEnabled(), newFakeKillSwitchCache())

	if _, err := uc.SetEnabled(ctx, 7, "master", false, "spam wave", ""); err == nil {
		t.Error("master freeze without confirm: want error, got nil")
	}
	if _, err := uc.SetEnabled(ctx, 7, "master", false, "spam wave", "UNFREEZE"); err == nil {
		t.Error("master freeze with wrong confirm: want error, got nil")
	}
	if _, err := uc.SetEnabled(ctx, 7, "master", true, "calm", "FREEZE"); err == nil {
		t.Error("master unfreeze with FREEZE: want error, got nil")
	}
	sw, err := uc.SetEnabled(ctx, 7, "master", false, "spam wave", "FREEZE")
	if err != nil {
		t.Fatalf("master freeze with FREEZE: unexpected error: %v", err)
	}
	if sw.Enabled {
		t.Error("master should be disabled after freeze")
	}
}

func TestKillSwitch_SetEnabled_WritesDBThenPublishesRedis(t *testing.T) {
	ctx := context.Background()
	repo := seedAllEnabled()
	cache := newFakeKillSwitchCache()
	uc := NewKillSwitchUsecase(repo, cache)

	sw, err := uc.SetEnabled(ctx, 7, "chat", false, "spam wave", "")
	if err != nil {
		t.Fatalf("SetEnabled: unexpected error: %v", err)
	}
	if sw.Enabled {
		t.Error("chat should be disabled")
	}
	if sw.UpdatedBy != 7 {
		t.Errorf("UpdatedBy = %d, want 7", sw.UpdatedBy)
	}
	if got := cache.sets["killswitch:chat"]; got != "0" {
		t.Errorf("Redis killswitch:chat = %q, want \"0\"", got)
	}
	if uc.IsEnabled(ctx, "chat") {
		t.Error("IsEnabled(chat) = true after disable, want false")
	}
}

func TestKillSwitch_SetEnabled_RedisPublishFailure_DoesNotFailWrite(t *testing.T) {
	ctx := context.Background()
	repo := seedAllEnabled()
	cache := newFakeKillSwitchCache()
	cache.setErr = errors.New("redis down")
	uc := NewKillSwitchUsecase(repo, cache)

	if _, err := uc.SetEnabled(ctx, 7, "chat", false, "spam wave", ""); err != nil {
		t.Fatalf("Redis publish failure should not fail the write: %v", err)
	}
	if repo.rows["chat"].Enabled {
		t.Error("DB row should be disabled despite Redis failure")
	}
}

func TestKillSwitch_SyncToRedis_ReloadsRuntimeFromDB(t *testing.T) {
	ctx := context.Background()
	repo := seedAllEnabled()
	repo.rows["registration"].Enabled = false
	cache := newFakeKillSwitchCache()
	uc := NewKillSwitchUsecase(repo, cache)

	if err := uc.SyncToRedis(ctx); err != nil {
		t.Fatalf("SyncToRedis: unexpected error: %v", err)
	}
	if got := cache.vals["killswitch:registration"]; got != "0" {
		t.Errorf("killswitch:registration = %q, want \"0\"", got)
	}
	if got := cache.vals["killswitch:master"]; got != "1" {
		t.Errorf("killswitch:master = %q, want \"1\"", got)
	}
}

func TestKillSwitch_EnsureSeeded_SeedsAllEnabled(t *testing.T) {
	ctx := context.Background()
	repo := newFakeKillSwitchRepo()
	uc := NewKillSwitchUsecase(repo, newFakeKillSwitchCache())

	if err := uc.EnsureSeeded(ctx); err != nil {
		t.Fatalf("EnsureSeeded: unexpected error: %v", err)
	}
	for _, m := range entity.KillSwitchModules {
		sw, err := repo.Get(ctx, m)
		if err != nil {
			t.Fatalf("module %q missing after seed: %v", m, err)
		}
		if !sw.Enabled {
			t.Errorf("module %q seeded disabled, want enabled", m)
		}
	}
}
