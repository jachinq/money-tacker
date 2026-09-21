package pnl

import (
	"testing"

	"money-tacker/internal/money"
)

func nav(date, v string) NavPoint {
	n, ok := money.ParseNavToE8(v)
	if !ok {
		panic(v)
	}
	return NavPoint{Date: date, UnitNavE8: n}
}

func buy(id int64, date, cash, navS, navDate string) Entry {
	c, _ := money.ParseYuanToFen(cash)
	n, _ := money.ParseNavToE8(navS)
	return Entry{
		ID: id, Kind: KindBuy, OccurDate: date,
		CashFen: c, SharesE8: money.SharesFromCash(c, n),
		UnitNavE8: n, NavDateUsed: navDate,
	}
}

func redeemShares(id int64, date string, sharesE8 int64, navS, navDate string) Entry {
	n, _ := money.ParseNavToE8(navS)
	return Entry{
		ID: id, Kind: KindRedeem, OccurDate: date,
		SharesE8: sharesE8, UnitNavE8: n,
		CashFen: money.CashFromShares(sharesE8, n), NavDateUsed: navDate,
	}
}

func TestHangZeroDayDailyPnlIsZero(t *testing.T) {
	entries := []Entry{buy(1, "2026-09-18", "10000", "1.0562", "2026-09-18")}
	navs := []NavPoint{nav("2026-09-18", "1.0562")}
	if DailyPnl(entries, navs, "2026-09-21") != 0 {
		t.Fatal("hang-zero day must be 0")
	}
}

func TestDailyPnlAfterNavArrives(t *testing.T) {
	entries := []Entry{buy(1, "2026-09-18", "10000", "1.0000", "2026-09-18")}
	navs := []NavPoint{
		nav("2026-09-18", "1.0000"),
		nav("2026-09-21", "1.0100"),
	}
	got := DailyPnl(entries, navs, "2026-09-21")
	if got != 10000 {
		t.Fatalf("got %d want 10000 fen", got)
	}
}

func TestBuyThenRedeemHalfCostAndCumulative(t *testing.T) {
	b := buy(1, "2026-09-18", "10000", "1.0000", "2026-09-18")
	half := b.SharesE8 / 2
	r := redeemShares(2, "2026-09-21", half, "1.1000", "2026-09-21")
	st, err := Replay([]Entry{b, r})
	if err != nil {
		t.Fatal(err)
	}
	if b.SharesE8 != 1_000_000_000_000 {
		t.Fatalf("buy shares %d", b.SharesE8)
	}
	if st.SharesE8 != 500_000_000_000 {
		t.Fatalf("shares %d", st.SharesE8)
	}
	if st.CostFen != 500_000 {
		t.Fatalf("remaining cost %d", st.CostFen)
	}
	if st.RealizedFen != 50_000 {
		t.Fatalf("realized %d want 500.00", st.RealizedFen)
	}
	mv := money.CashFromShares(st.SharesE8, nav("2026-09-21", "1.1000").UnitNavE8)
	if mv != 550_000 {
		t.Fatalf("mv %d", mv)
	}
	cum := mv + st.TotalRedeemFen - st.TotalBuyFen
	if cum != 100_000 {
		t.Fatalf("cumulative %d", cum)
	}
	if cum != st.RealizedFen+(mv-st.CostFen) {
		t.Fatalf("identity failed")
	}
}

func TestReplayRejectsOverRedeem(t *testing.T) {
	b := buy(1, "2026-09-18", "10000", "1.0562", "2026-09-18")
	r := redeemShares(2, "2026-09-21", b.SharesE8+1, "1.0562", "2026-09-18")
	_, err := Replay([]Entry{b, r})
	if err != ErrInsufficientShares {
		t.Fatalf("err=%v", err)
	}
}

func TestDisplayDateSkipsTodayAndGaps(t *testing.T) {
	navs := []NavPoint{
		nav("2026-09-18", "1.0562"),
		nav("2026-09-21", "1.0571"),
	}
	d, ok := DisplayDate(navs, "2026-09-21", 1)
	if !ok || d != "2026-09-18" {
		t.Fatalf("display=%s ok=%v want 2026-09-18", d, ok)
	}
	d2, ok2 := DisplayDate(navs, "2026-09-22", 1)
	if !ok2 || d2 != "2026-09-21" {
		t.Fatalf("display=%s ok=%v want 2026-09-21", d2, ok2)
	}
}

func TestCalendarFillsHangZeroZero(t *testing.T) {
	entries := []Entry{buy(1, "2026-09-18", "10000", "1.0562", "2026-09-18")}
	navs := []NavPoint{nav("2026-09-18", "1.0562")}
	rows := CalendarPnl(entries, navs, "2026-09-18", "2026-09-20")
	if len(rows) != 3 {
		t.Fatalf("len %d", len(rows))
	}
	if rows[0].HangZero || rows[0].PnlFen != 0 {
		t.Fatalf("first day %+v", rows[0])
	}
	if !rows[1].HangZero || rows[1].PnlFen != 0 {
		t.Fatalf("gap %+v", rows[1])
	}
}

func TestSameDayBuyIncludedInDailyPnl(t *testing.T) {
	entries := []Entry{buy(1, "2026-09-21", "10000", "1.0500", "2026-09-21")}
	navs := []NavPoint{
		nav("2026-09-18", "1.0400"),
		nav("2026-09-21", "1.0600"),
	}
	got := DailyPnl(entries, navs, "2026-09-21")
	if got != 9523 {
		t.Fatalf("got %d want 9523 fen", got)
	}
}

func TestVoidedEntryIgnored(t *testing.T) {
	b := buy(1, "2026-09-18", "10000", "1.0562", "2026-09-18")
	b.Voided = true
	st, err := Replay([]Entry{b})
	if err != nil || st.SharesE8 != 0 {
		t.Fatalf("st=%+v err=%v", st, err)
	}
}
