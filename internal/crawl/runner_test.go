package crawl

import (
	"path/filepath"
	"testing"
	"time"

	"money-tacker/internal/db"
	"money-tacker/internal/store"
	"money-tacker/migrations"
)

type mapFetch map[string]string

func (m mapFetch) Get(url string) (string, int, error) {
	if b, ok := m[url]; ok {
		return b, 200, nil
	}
	return "", 404, nil
}

func TestUpsertSnapshotAndTwoObservations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	sqlDB, err := db.Open(path, migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	st := &store.Store{DB: sqlDB}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	page1 := `https://example.test/index.html`
	html := fixture
	r := &Runner{
		Store: st,
		Fetcher: mapFetch{
			page1: html,
		},
		BaseURL:  "https://example.test/",
		Delay:    0,
		Now:      func() time.Time { return now },
		MaxPages: 3,
	}
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	navs, err := st.ListNav("AF247494G")
	if err != nil {
		t.Fatal(err)
	}
	if len(navs) != 1 {
		t.Fatalf("snapshots=%d want 1", len(navs))
	}
	var n int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM nav_observation WHERE product_code='AF247494G'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("observations=%d want 2", n)
	}
	var runs int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM crawl_run`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 2 {
		t.Fatalf("runs=%d", runs)
	}
}
