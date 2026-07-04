### Task 1: Static Data Pool

**Files:**
- Create: `internal/seed/data.go`
- Create: `internal/seed/data_test.go`

**Interfaces:**
- Produces: Package-level slices and helper functions used by all seeder files

- [ ] **Step 1: Create `internal/seed/data.go` with static data pools**

```go
package seed

var firstNames = []string{
	"Ahmad", "Budi", "Citra", "Dewi", "Eko", "Fitri", "Gunawan", "Hendra",
	"Indah", "Joko", "Kartika", "Lukman", "Mega", "Nugroho", "Putri",
	"Ratna", "Sigit", "Tika", "Utami", "Wahyu", "Agus", "Bayu", "Cahyo",
	"Dian", "Eka", "Fajar", "Gita", "Hadi", "Intan", "Jaya", "Kurniawan",
	"Lestari", "Mulyadi", "Nina", "Oki", "Pramono", "Rini", "Sari", "Teguh",
	"Umar", "Vina", "Wulan", "Yoga", "Zainal", "Adi", "Bambang", "Candra",
	"Desi", "Edi", "Fani",
}

var lastNames = []string{
	"Pratama", "Wijaya", "Kusuma", "Saputra", "Santoso", "Hidayat", "Nugraha",
	"Wibowo", "Setiawan", "Susanti", "Utami", "Sari", "Wati", "Rahayu",
	"Anggraini", "Hartono", "Gunarto", "Purnomo", "Suryadi", "Ramadhani",
	"Maulana", "Syahrini", "Pertiwi", "Lestari", "Haryanto", "Maryati",
}

var cities = []string{
	"Jakarta", "Surabaya", "Bandung", "Medan", "Semarang", "Makassar", "Palembang",
	"Tangerang", "Bekasi", "Depok", "Bogor", "Yogyakarta", "Solo", "Malang",
	"Denpasar", "Banjarmasin", "Pontianak", "Padang", "Pekanbaru", "Manado",
	"Balikpapan", "Samarinda", "Aceh", "Mataram", "Kupang",
}

var provinces = []string{
	"DKI Jakarta", "Jawa Barat", "Jawa Tengah", "Jawa Timur", "Sumatera Utara",
	"Sulawesi Selatan", "Sumatera Selatan", "Banten", "DI Yogyakarta",
	"Kalimantan Timur", "Bali", "Lampung", "Riau", "Kalimantan Selatan",
	"Sumatera Barat", "Papua", "Nusa Tenggara Timur", "Kalimantan Barat",
}

var categories = []string{
	"House Cleaning", "AC Service", "Plumbing", "Electrical", "Painting",
	"Gardening", "Catering", "Photography", "Massage", "Tutoring",
}

func randomItem[T any](rng *rand.Rand, items []T) T {
	return items[rng.Intn(len(items))]
}

func randomInt(rng *rand.Rand, min, max int) int {
	return rng.Intn(max-min+1) + min
}
```

- [ ] **Step 2: Create `internal/seed/data_test.go`**

```go
package seed

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomItemReturnsItemFromSlice(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	items := []string{"a", "b", "c"}
	got := randomItem[string](rng, items)
	assert.Contains(t, items, got)
}

func TestRandomIntWithinRange(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 100; i++ {
		got := randomInt(rng, 5, 10)
		assert.GreaterOrEqual(t, got, 5)
		assert.LessOrEqual(t, got, 10)
	}
}
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `go test ./internal/seed/ -v -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/seed/data.go internal/seed/data_test.go
git commit -m "feat(seed): add static data pools for Indonesian locale data"
```
