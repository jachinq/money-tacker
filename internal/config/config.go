package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr           string
	Env            string
	TZ             *time.Location
	SQLitePath     string
	SessionHours   int
	CrawlCron      string
	CrawlDelayMS   int
	DisplayLagDays int
	RegisterOpen   bool
	BcryptCost     int
	AdminToken     string
	CrawlBaseURL   string
	CookieSecure   bool
}

func Load() Config {
	loc, err := time.LoadLocation(env("APP_TZ", "Asia/Shanghai"))
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return Config{
		Addr:           env("APP_ADDR", ":8080"),
		Env:            env("APP_ENV", "dev"),
		TZ:             loc,
		SQLitePath:     env("SQLITE_PATH", "data/app.db"),
		SessionHours:   envInt("SESSION_HOURS", 168),
		CrawlCron:      env("CRAWL_CRON", "0 10,16,21 * * *"),
		CrawlDelayMS:   envInt("CRAWL_DELAY_MS", 500),
		DisplayLagDays: envInt("DISPLAY_LAG_DAYS", 1),
		RegisterOpen:   envBool("REGISTER_OPEN", false),
		BcryptCost:     envInt("BCRYPT_COST", 12),
		AdminToken:     env("APP_ADMIN_TOKEN", ""),
		CrawlBaseURL:   env("CRAWL_BASE_URL", "https://www.bankofchina.com/sourcedb/srfd6_2024/"),
		CookieSecure:   envBool("COOKIE_SECURE", false),
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		return v == "1" || v == "true" || v == "TRUE"
	}
	return def
}
