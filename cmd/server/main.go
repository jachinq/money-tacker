package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/robfig/cron/v3"

	"money-tacker/internal/config"
	"money-tacker/internal/crawl"
	"money-tacker/internal/db"
	"money-tacker/internal/server"
	"money-tacker/internal/store"
	"money-tacker/migrations"
)

func main() {
	cfg := config.Load()
	sqlDB, err := db.Open(cfg.SQLitePath, migrations.FS, ".")
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()
	st := &store.Store{DB: sqlDB}
	runner := &crawl.Runner{
		Store:   st,
		Fetcher: crawl.HTTPFetcher{Client: &http.Client{Timeout: 30 * time.Second}},
		BaseURL: cfg.CrawlBaseURL,
		Delay:   time.Duration(cfg.CrawlDelayMS) * time.Millisecond,
		Now:     func() time.Time { return time.Now().In(cfg.TZ) },
	}
	srv := server.New(cfg, st)
	srv.Crawl = runner.Go
	srv.CrawlBusy = runner.Busy
	srv.RefreshProduct = runner.RefreshProduct
	if _, err := os.Stat("web/dist"); err == nil {
		srv.Static = "web/dist"
	}

	c := cron.New(cron.WithLocation(cfg.TZ))
	if _, err := c.AddFunc(cfg.CrawlCron, func() {
		if err := runner.Run(); err != nil {
			log.Println("crawl:", err)
		}
	}); err != nil {
		log.Println("cron:", err)
	} else {
		c.Start()
		defer c.Stop()
	}
	go func() {
		if err := runner.Run(); err != nil {
			log.Println("startup crawl:", err)
		}
	}()

	log.Println("listen", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, srv.Router()); err != nil {
		log.Fatal(err)
	}
}
