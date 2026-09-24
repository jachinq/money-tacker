package crawl

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"money-tacker/internal/db"
	"money-tacker/internal/store"
	"money-tacker/migrations"
)

func TestRefreshProductWritesLatestNavWithoutCrawl(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	sqlDB, err := db.Open(path, migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	st := &store.Store{DB: sqlDB}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	if err := st.UpsertProduct("AF247494G", "旧名称", "旧机构", 2, now); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSnapshot("AF247494G", "2026-09-20", 110000000, 110000000, 0, "", now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`UPDATE product SET listed=0 WHERE code='AF247494G'`); err != nil {
		t.Fatal(err)
	}
	html := `
<html><body><table>
<tr><th>产品代码</th><th>产品名称</th><th>发行机构</th><th>单位净值</th><th>累计净值</th><th>日净值增长率</th><th>截止日期</th></tr>
<tr><td>OTHER</td><td>别的</td><td>别的机构</td><td>1.0000</td><td>1.0000</td><td>0.01%</td><td>2026-09-18</td></tr>
<tr><td>AF247494G</td><td>新名称</td><td>新机构</td><td>1.0562</td><td>1.0700</td><td>0.05%</td><td>2026-09-18</td></tr>
</table></body></html>`
	r := &Runner{
		Store:   st,
		Fetcher: mapFetch{"https://example.test/index_1.html": html},
		BaseURL: "https://example.test/",
		Now:     func() time.Time { return now },
	}
	if err := r.RefreshProduct("af247494g"); err != nil {
		t.Fatal(err)
	}
	p, err := st.Product("AF247494G")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "新名称" || p.Issuer != "新机构" || !p.Listed || p.LastSeenPage != 2 {
		t.Fatalf("product %+v", p)
	}
	navs, err := st.ListNav("AF247494G")
	if err != nil {
		t.Fatal(err)
	}
	if len(navs) != 2 {
		t.Fatalf("navs %+v", navs)
	}
	var got store.NavSnap
	for _, n := range navs {
		if n.NavDate == "2026-09-18" {
			got = n
		}
	}
	if got.UnitNavE8 != 105620000 || !got.AccNavE8.Valid || got.AccNavE8.Int64 != 107000000 || !got.DailyReturnBP.Valid || got.DailyReturnBP.Int64 != 5 {
		t.Fatalf("snapshot %+v", got)
	}
	var runs, obs int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM crawl_run`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM nav_observation`).Scan(&obs); err != nil {
		t.Fatal(err)
	}
	if runs != 0 || obs != 0 {
		t.Fatalf("runs=%d obs=%d", runs, obs)
	}
}

func TestRefreshProductLeavesProductAloneWhenMissingOrUnread(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	sqlDB, err := db.Open(path, migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	st := &store.Store{DB: sqlDB}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	if err := st.UpsertProduct("AF247494G", "旧名称", "旧机构", 1, now); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSnapshot("AF247494G", "2026-09-18", 105620000, 0, 0, "", now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	page := `<html><body><table>
<tr><th>产品代码</th><th>产品名称</th><th>单位净值</th><th>截止日期</th></tr>
<tr><td>OTHER</td><td>别的</td><td>1.0000</td><td>2026-09-18</td></tr>
</table></body></html>`
	r := &Runner{
		Store:   st,
		Fetcher: mapFetch{"https://example.test/index.html": page},
		BaseURL: "https://example.test/",
		Now:     func() time.Time { return now },
	}
	if err := r.RefreshProduct("AF247494G"); !errors.Is(err, ErrProductNotOnPage) {
		t.Fatalf("missing product: %v", err)
	}
	r.Fetcher = mapFetch{}
	if err := r.RefreshProduct("AF247494G"); !errors.Is(err, ErrPageUnread) {
		t.Fatalf("unread: %v", err)
	}
	r.Fetcher = errFetch{}
	if err := r.RefreshProduct("AF247494G"); !errors.Is(err, ErrPageUnread) {
		t.Fatalf("network: %v", err)
	}
	if _, err := sqlDB.Exec(`UPDATE product SET last_seen_page=NULL WHERE code='AF247494G'`); err != nil {
		t.Fatal(err)
	}
	if err := r.RefreshProduct("AF247494G"); !errors.Is(err, ErrNoCatalogPage) {
		t.Fatalf("no page: %v", err)
	}
	p, err := st.Product("AF247494G")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "旧名称" || !p.Listed {
		t.Fatalf("product changed %+v", p)
	}
	navs, err := st.ListNav("AF247494G")
	if err != nil || len(navs) != 1 || navs[0].UnitNavE8 != 105620000 {
		t.Fatalf("navs %+v %v", navs, err)
	}
}

func TestRefreshProductNoUnitNavStillLists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	sqlDB, err := db.Open(path, migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	st := &store.Store{DB: sqlDB}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	if err := st.UpsertProduct("CASH1", "旧现金", "旧机构", 1, now); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`UPDATE product SET listed=0 WHERE code='CASH1'`); err != nil {
		t.Fatal(err)
	}
	html := `<html><body><table>
<tr><th>产品代码</th><th>产品名称</th><th>发行机构</th><th>每万份基金单位收益</th><th>截止日期</th></tr>
<tr><td>CASH1</td><td>新现金</td><td>新机构</td><td>0.35</td><td>2026-09-18</td></tr>
</table></body></html>`
	r := &Runner{
		Store:   st,
		Fetcher: mapFetch{"https://example.test/index.html": html},
		BaseURL: "https://example.test/",
		Now:     func() time.Time { return now },
	}
	if err := r.RefreshProduct("CASH1"); !errors.Is(err, ErrNoUnitNav) {
		t.Fatal(err)
	}
	p, err := st.Product("CASH1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "新现金" || p.Issuer != "新机构" || !p.Listed {
		t.Fatalf("product %+v", p)
	}
	navs, err := st.ListNav("CASH1")
	if err != nil || len(navs) != 0 {
		t.Fatalf("navs %+v %v", navs, err)
	}
}

func TestRefreshProductPrefersLaterUnitNavOnSamePage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	sqlDB, err := db.Open(path, migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	st := &store.Store{DB: sqlDB}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	if err := st.UpsertProduct("AF247494G", "旧名称", "旧机构", 1, now); err != nil {
		t.Fatal(err)
	}
	html := `<html><body><table>
<tr><th>产品代码</th><th>产品名称</th><th>发行机构</th><th>单位净值</th><th>截止日期</th></tr>
<tr><td>AF247494G</td><td>第一次</td><td>甲</td><td>1.0100</td><td>2026-09-18</td></tr>
<tr><td>AF247494G</td><td>缺净值</td><td>乙</td><td></td><td>2026-09-19</td></tr>
<tr><td>AF247494G</td><td>较后</td><td>丙</td><td>1.0200</td><td>2026-09-19</td></tr>
<tr><td>AF247494G</td><td>再缺</td><td>丁</td><td></td><td>2026-09-20</td></tr>
</table></body></html>`
	r := &Runner{
		Store:   st,
		Fetcher: mapFetch{"https://example.test/index.html": html},
		BaseURL: "https://example.test/",
		Now:     func() time.Time { return now },
	}
	if err := r.RefreshProduct("AF247494G"); err != nil {
		t.Fatal(err)
	}
	p, err := st.Product("AF247494G")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "较后" || p.Issuer != "丙" {
		t.Fatalf("product %+v", p)
	}
	navs, err := st.ListNav("AF247494G")
	if err != nil || len(navs) != 1 || navs[0].NavDate != "2026-09-19" || navs[0].UnitNavE8 != 102000000 {
		t.Fatalf("navs %+v %v", navs, err)
	}
}
