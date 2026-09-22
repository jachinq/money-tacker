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

func TestWindowPctUsesNavOnOrBeforeStart(t *testing.T) {
	navs := []NavPoint{
		nav("2026-08-15", "1.0000"),
		nav("2026-09-18", "1.0562"),
	}
	got, ok := WindowPctE4(navs, 30)
	if !ok {
		t.Fatal("want window ok")
	}
	if got != 562 {
		t.Fatalf("pct e4=%d want 562 (5.62%%)", got)
	}
}

func TestWindowPctInsufficientHistory(t *testing.T) {
	navs := []NavPoint{nav("2026-09-18", "1.0562")}
	if _, ok := WindowPctE4(navs, 30); ok {
		t.Fatal("single point cannot fill 30-day start")
	}
}

func TestWindowPctSkipsHangZeroStartDate(t *testing.T) {
	// 2026-09-21 minus 30 = 2026-08-22 (Saturday). Use Friday 08-21.
	navs := []NavPoint{
		nav("2026-08-21", "1.0000"),
		nav("2026-09-21", "1.0100"),
	}
	got, ok := WindowPctE4(navs, 30)
	if !ok {
		t.Fatal("hang-zero start must still resolve")
	}
	if got != 100 {
		t.Fatalf("pct e4=%d want 100 (1.00%%)", got)
	}
}

func TestImpliedBuyFromCumulativeRoundNumbers(t *testing.T) {
	cash, ok := money.ParseYuanToFen("10000")
	if !ok {
		t.Fatal("cash")
	}
	cum, ok := money.ParseYuanToFen("100")
	if !ok {
		t.Fatal("cum")
	}
	latest := nav("2026-09-21", "1.0100").UnitNavE8
	shares, implied, ok := ImpliedBuy(cash, cum, latest)
	if !ok {
		t.Fatal("implied buy")
	}
	if shares != 1_000_000_000_000 {
		t.Fatalf("shares %d want 10000.00000000 e8", shares)
	}
	if implied != 100_000_000 {
		t.Fatalf("implied nav %d want 1.00000000", implied)
	}
	mv := money.CashFromShares(shares, latest)
	if mv-cash != 10_000 {
		t.Fatalf("unrealized %d want 100.00", mv-cash)
	}
}

func TestImpliedBuyRejectsNonPositiveMarketValue(t *testing.T) {
	cash, _ := money.ParseYuanToFen("100")
	cum, _ := money.ParseYuanToFen("-100")
	if _, _, ok := ImpliedBuy(cash, cum, 100_000_000); ok {
		t.Fatal("市值必须为正")
	}
}

func TestResolveNavForOccurDoesNotEstimateLatest(t *testing.T) {
	navs := []NavPoint{nav("2026-09-18", "1.0562")}
	if _, _, ok, _ := ResolveNavForOccur(navs, "2024-03-01", 0, ""); ok {
		t.Fatal("occur before all navs must not use latest as buy nav")
	}
	n, used, ok, est := ResolveNavForOccur(navs, "2026-09-21", 0, "")
	if !ok || est || used != "2026-09-18" || n != navs[0].UnitNavE8 {
		t.Fatalf("prev nav n=%d used=%s ok=%v est=%v", n, used, ok, est)
	}
}

func TestCollectionGapIsCumulativeMinusCollectedDaily(t *testing.T) {
	b := buy(1, "2024-03-01", "10000", "1.0000", "2024-03-01")
	navs := []NavPoint{
		nav("2026-09-18", "1.0500"),
		nav("2026-09-21", "1.0600"),
	}
	latest := navs[1].UnitNavE8
	st, err := Replay([]Entry{b})
	if err != nil {
		t.Fatal(err)
	}
	mv := money.CashFromShares(st.SharesE8, latest)
	cum := mv - st.CostFen
	if cum != 60_000 {
		t.Fatalf("cum %d want 600.00", cum)
	}
	collected := CollectedDailySum([]Entry{b}, navs)
	if collected != 10_000 {
		t.Fatalf("collected %d want 100.00 (only 1.05→1.06; first nav day has no prev)", collected)
	}
	if CollectionGap(cum, collected) != 50_000 {
		t.Fatalf("gap %d want 500.00", CollectionGap(cum, collected))
	}
}

func TestWindowPctZeroStartIsInsufficient(t *testing.T) {
	navs := []NavPoint{
		{Date: "2026-08-15", UnitNavE8: 0},
		nav("2026-09-18", "1.0562"),
	}
	if _, ok := WindowPctE4(navs, 30); ok {
		t.Fatal("zero start nav is 窗口历史不足")
	}
}

func TestCumulativeReturnE4IsCumOverBuy(t *testing.T) {
	got, ok := CumulativeReturnE4(100_000, 1_000_000)
	if !ok || got != 1000 {
		t.Fatalf("got %d ok=%v want 1000 (10.00%%)", got, ok)
	}
}

func TestCumulativeReturnE4MissingWhenBuyZero(t *testing.T) {
	if _, ok := CumulativeReturnE4(0, 0); ok {
		t.Fatal("no 累计收益率 when 累计买入 is 0")
	}
}

func TestAnnualizedUsesOccupancySegmentNotWholeLedger(t *testing.T) {
	wave1Buy := buy(1, "2026-01-01", "10000", "1.0000", "2026-01-01")
	wave1Redeem := redeemShares(2, "2026-01-31", wave1Buy.SharesE8, "1.1000", "2026-01-31")
	wave2Buy := buy(3, "2026-09-01", "10000", "1.0000", "2026-09-01")
	entries := []Entry{wave1Buy, wave1Redeem, wave2Buy}
	latest := nav("2026-09-11", "1.0100")

	st, err := Replay(entries)
	if err != nil {
		t.Fatal(err)
	}
	mv := money.CashFromShares(st.SharesE8, latest.UnitNavE8)
	cum := mv + st.TotalRedeemFen - st.TotalBuyFen
	whole, ok := CumulativeReturnE4(cum, st.TotalBuyFen)
	if !ok || whole != 550 {
		t.Fatalf("整本账累计收益率 e4=%d ok=%v want 550 (5.50%%)", whole, ok)
	}

	ann, ok := AnnualizedCumulativeReturnE4(entries, latest)
	if !ok {
		t.Fatal("want 年化累计收益率")
	}
	if ann != 3650 {
		t.Fatalf("年化 e4=%d want 3650 (36.50%%); must ignore wave1 profit and empty days", ann)
	}
}

func TestAnnualizedClosedSegmentEndsOnLastRedeemNotLatestNav(t *testing.T) {
	b := buy(1, "2026-01-01", "10000", "1.0000", "2026-01-01")
	r := redeemShares(2, "2026-01-31", b.SharesE8, "1.1000", "2026-01-31")
	ann, ok := AnnualizedCumulativeReturnE4([]Entry{b, r}, nav("2026-09-11", "1.2000"))
	if !ok {
		t.Fatal("closed account still has 年化累计收益率")
	}
	if ann != 12167 {
		t.Fatalf("年化 e4=%d want 12167 (10%% × 365/30, rounded)", ann)
	}
}

func TestAnnualizedMissingWhenSameDay(t *testing.T) {
	b := buy(1, "2026-09-11", "10000", "1.0000", "2026-09-11")
	if _, ok := AnnualizedCumulativeReturnE4([]Entry{b}, nav("2026-09-11", "1.0100")); ok {
		t.Fatal("N=0 must not produce 年化累计收益率")
	}
}
