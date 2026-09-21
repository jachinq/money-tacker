package crawl

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"money-tacker/internal/store"
)

type Fetcher interface {
	Get(url string) (body string, status int, err error)
}

type HTTPFetcher struct {
	Client *http.Client
}

func (f HTTPFetcher) Get(url string) (string, int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", "money-tacker/0.1 (personal nav tracker)")
	resp, err := f.Client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(b), resp.StatusCode, nil
}

type Runner struct {
	Store    *store.Store
	Fetcher  Fetcher
	BaseURL  string
	Delay    time.Duration
	Now      func() time.Time
	MaxPages int
}

func (r *Runner) Run() error {
	now := r.Now()
	runID, err := r.Store.StartCrawl(now)
	if err != nil {
		return err
	}
	base := strings.TrimRight(r.BaseURL, "/") + "/"
	max := r.MaxPages
	if max <= 0 {
		max = 250
	}
	seen := map[string]struct{}{}
	pagesOK := 0
	productsOK := 0
	var errs []string
	for i := 0; i < max; i++ {
		u := base + "index.html"
		if i > 0 {
			u = fmt.Sprintf("%sindex_%d.html", base, i)
		}
		body, status, err := r.Fetcher.Get(u)
		if err != nil {
			errs = append(errs, fmt.Sprintf("page %d: %v", i+1, err))
			break
		}
		if status == 404 || strings.TrimSpace(body) == "" {
			break
		}
		if status >= 400 {
			errs = append(errs, fmt.Sprintf("page %d status %d", i+1, status))
			break
		}
		rows, err := ParsePage(body)
		if err != nil {
			errs = append(errs, fmt.Sprintf("page %d parse: %v", i+1, err))
			break
		}
		if len(rows) == 0 && i > 0 {
			break
		}
		pagesOK++
		for _, row := range rows {
			if row.Code == "" || row.NavDate == "" {
				continue
			}
			if err := r.Store.UpsertProduct(row.Code, row.Name, row.Issuer, i+1, now); err != nil {
				errs = append(errs, err.Error())
				continue
			}
			if row.HasUnitNav {
				if err := r.Store.UpsertSnapshot(row.Code, row.NavDate, row.UnitNavE8, row.AccNavE8, row.DailyReturnBP, "", now.Format(time.RFC3339)); err != nil {
					errs = append(errs, err.Error())
					continue
				}
			}
			if err := r.Store.InsertObservation(runID, row.Code, row.NavDate, row.UnitNavE8); err != nil {
				errs = append(errs, err.Error())
				continue
			}
			seen[row.Code] = struct{}{}
			productsOK++
		}
		if r.Delay > 0 && i+1 < max {
			time.Sleep(r.Delay)
		}
	}
	status := "success"
	if pagesOK == 0 {
		status = "fail"
	} else if len(errs) > 0 {
		status = "partial"
	}
	if status == "success" {
		_ = r.Store.MarkMissingUnlisted(seen, now)
	}
	sum := strings.Join(errs, "; ")
	if len(sum) > 2000 {
		sum = sum[:2000]
	}
	return r.Store.FinishCrawl(runID, status, pagesOK, productsOK, sum, r.Now())
}
