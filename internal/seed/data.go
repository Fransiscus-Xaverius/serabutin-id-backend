package seed

import "math/rand"

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
