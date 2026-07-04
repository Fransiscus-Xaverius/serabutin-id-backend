### Task 2: User Generator

**Files:**
- Create: `internal/seed/users.go`
- Create: `internal/seed/users_test.go`

**Interfaces:**
- Consumes: `randomItem`, `randomInt`, data slices from `data.go`; `model.UserModel` from `internal/repository/model`
- Produces: `func generateUsers(rng *rand.Rand, bcryptCost int) ([]model.UserModel, error)` — returns 1000 users (200 providers + 800 customers) with unique emails, whatsapp numbers, and bcrypt-hashed passwords

- [ ] **Step 1: Write the failing test**

Create `internal/seed/users_test.go`:

```go
package seed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateUsers_ReturnsCorrectCount(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	assert.Len(t, users, 1000)
}

func TestGenerateUsers_UniqueEmails(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	emails := make(map[string]bool)
	for _, u := range users {
		assert.False(t, emails[u.Email], "duplicate email: %s", u.Email)
		emails[u.Email] = true
	}
}

func TestGenerateUsers_UniqueWhatsApp(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	phones := make(map[string]bool)
	for _, u := range users {
		assert.False(t, phones[u.WhatsApp], "duplicate whatsapp: %s", u.WhatsApp)
		phones[u.WhatsApp] = true
	}
}

func TestGenerateUsers_HashedPasswords(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	for _, u := range users {
		assert.NotEqual(t, "testing123", u.Password)
		assert.True(t, len(u.Password) > 20, "password looks unhashed: %s", u.Password)
	}
}

func TestGenerateUsers_ProviderCount(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	count := 0
	for _, u := range users {
		if u.Role == "provider" {
			count++
		}
	}
	assert.Equal(t, 200, count)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/seed/ -v -count=1 -run TestGenerateUsers`
Expected: compilation error (generateUsers not defined)

- [ ] **Step 3: Write minimal implementation**

Create `internal/seed/users.go`:

```go
package seed

import (
	"fmt"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"golang.org/x/crypto/bcrypt"
)

func generateUsers(bcryptCost int) ([]model.UserModel, error) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	users := make([]model.UserModel, 0, 1000)

	for i := 0; i < 1000; i++ {
		firstName := randomItem(rng, firstNames)
		lastName := randomItem(rng, lastNames)
		name := firstName + " " + lastName

		role := "customer"
		if i < 200 {
			role = "provider"
		}

		email := fmt.Sprintf("%s.%s.%d@email.com", firstName, lastName, i)
		whatsapp := fmt.Sprintf("+628%d%d", randomInt(rng, 100000000, 999999999), randomInt(rng, 10000000, 99999999))[:16]

		hash, err := bcrypt.GenerateFromPassword([]byte("testing123"), bcryptCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password for user %d: %w", i, err)
		}

		city := randomItem(rng, cities)
		province := randomItem(rng, provinces)

		user := model.UserModel{
			Email:     email,
			Password:  string(hash),
			Name:      name,
			WhatsApp:  whatsapp,
			Role:      role,
			Bio:       fmt.Sprintf("Halo, saya %s. Siap membantu Anda!", name),
			Address:   fmt.Sprintf("Jl. %s No. %d", randomItem(rng, firstNames), randomInt(rng, 1, 200)),
			City:      city,
			Province:  province,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		users = append(users, user)
	}

	return users, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/seed/ -v -count=1 -run TestGenerateUsers`
Expected: PASS

- [ ] **Step 5: Add imports and fix — ensure `time` and `math/rand` are imported**

Fix the import block in `users.go` if needed:

```go
import (
	"fmt"
	"math/rand"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"golang.org/x/crypto/bcrypt"
)
```

- [ ] **Step 6: Run all tests again**

Run: `go test ./internal/seed/ -v -count=1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/seed/users.go internal/seed/users_test.go
git commit -m "feat(seed): add user generator (200 providers + 800 customers)"
```
