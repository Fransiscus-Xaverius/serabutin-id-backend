package usecase

import (
	"context"
	"testing"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type promoFakeUserRepo struct {
	users map[string]*entity.User
}

func (f *promoFakeUserRepo) Create(ctx context.Context, u *entity.User) error { return nil }
func (f *promoFakeUserRepo) FindByID(ctx context.Context, id uint) (*entity.User, error) {
	return nil, nil
}
func (f *promoFakeUserRepo) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	u, ok := f.users[email]
	if !ok {
		return nil, nil
	}
	return u, nil
}
func (f *promoFakeUserRepo) FindByWhatsApp(ctx context.Context, w string) (*entity.User, error) {
	return nil, nil
}
func (f *promoFakeUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	_, ok := f.users[email]
	return ok, nil
}
func (f *promoFakeUserRepo) Update(ctx context.Context, u *entity.User) error {
	f.users[u.Email] = u
	return nil
}

func TestPromoteAdmins_PromotesExistingSkipsMissing(t *testing.T) {
	repo := &promoFakeUserRepo{users: map[string]*entity.User{
		"a@x.com": {ID: 1, Email: "a@x.com", Role: "customer"},
		"b@x.com": {ID: 2, Email: "b@x.com", Role: "admin"},
	}}
	got := PromoteAdmins(context.Background(), repo, []string{"a@x.com", "b@x.com", "ghost@x.com", "a@x.com"})
	if len(got) != 1 || got[0] != "a@x.com" {
		t.Fatalf("promoted = %v, want [a@x.com]", got)
	}
	if repo.users["a@x.com"].Role != "admin" {
		t.Errorf("a@x.com role = %q, want admin", repo.users["a@x.com"].Role)
	}
}
