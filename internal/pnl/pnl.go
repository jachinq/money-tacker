package pnl

import (
	"errors"
	"time"

	"money-tacker/internal/money"
)

var (
	ErrInsufficientShares = errors.New("INSUFFICIENT_SHARES")
	ErrEmptyHolding       = errors.New("EMPTY_HOLDING")
)

type Kind string

const (
	KindBuy    Kind = "buy"
	KindRedeem Kind = "redeem"
)

type Entry struct {
	ID          int64
	Kind        Kind
	OccurDate   string // YYYY-MM-DD
	CashFen     int64
	SharesE8    int64
	UnitNavE8   int64
	NavDateUsed string
	Voided      bool
}

type NavPoint struct {
	Date      string
	UnitNavE8 int64
}

type State struct {
	SharesE8       int64
	CostFen        int64
	RealizedFen    int64
	TotalBuyFen    int64
	TotalRedeemFen int64
}

type DayPnl struct {
	Date      string
	PnlFen    int64
	HangZero  bool
	HasNav    bool
	UnitNavE8 int64
}

func Active(entries []Entry) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if !e.Voided {
			out = append(out, e)
		}
	}
	return out
}

func Replay(entries []Entry) (State, error) {
	var st State
	for _, e := range Active(entries) {
		switch e.Kind {
		case KindBuy:
			st.SharesE8 += e.SharesE8
			st.CostFen += e.CashFen
			st.TotalBuyFen += e.CashFen
		case KindRedeem:
			if e.SharesE8 > st.SharesE8 {
				return State{}, ErrInsufficientShares
			}
			if st.SharesE8 == 0 {
				return State{}, ErrEmptyHolding
			}
			amort := money.Amortize(st.CostFen, e.SharesE8, st.SharesE8)
			if e.SharesE8 == st.SharesE8 {
				amort = st.CostFen
			}
			st.RealizedFen += e.CashFen - amort
			st.SharesE8 -= e.SharesE8
			st.CostFen -= amort
			st.TotalRedeemFen += e.CashFen
			if st.SharesE8 == 0 {
				st.CostFen = 0
			}
		}
	}
	return st, nil
}

func SharesAtOpen(entries []Entry, day string) (int64, error) {
	var before []Entry
	for _, e := range Active(entries) {
		if e.OccurDate < day {
			before = append(before, e)
		}
	}
	st, err := Replay(before)
	if err != nil {
		return 0, err
	}
	return st.SharesE8, nil
}

func NavOn(navs []NavPoint, date string) (NavPoint, bool) {
	for _, n := range navs {
		if n.Date == date && n.UnitNavE8 > 0 {
			return n, true
		}
	}
	return NavPoint{}, false
}

func PrevNav(navs []NavPoint, date string) (NavPoint, bool) {
	var best NavPoint
	ok := false
	for _, n := range navs {
		if n.Date < date && n.UnitNavE8 > 0 {
			if !ok || n.Date > best.Date {
				best = n
				ok = true
			}
		}
	}
	return best, ok
}

func NavOnOrBefore(navs []NavPoint, date string) (NavPoint, bool) {
	var best NavPoint
	ok := false
	for _, n := range navs {
		if n.UnitNavE8 <= 0 {
			continue
		}
		if n.Date <= date && (!ok || n.Date > best.Date) {
			best = n
			ok = true
		}
	}
	return best, ok
}

func LatestNav(navs []NavPoint) (NavPoint, bool) {
	var best NavPoint
	ok := false
	for _, n := range navs {
		if n.UnitNavE8 <= 0 {
			continue
		}
		if !ok || n.Date > best.Date {
			best = n
			ok = true
		}
	}
	return best, ok
}

// DisplayDate is the latest nav date <= today - lagDays (default lag 1).
func DisplayDate(navs []NavPoint, today string, lagDays int) (string, bool) {
	if lagDays < 0 {
		lagDays = 0
	}
	cutoff, err := addDays(today, -lagDays)
	if err != nil {
		return "", false
	}
	var best string
	ok := false
	for _, n := range navs {
		if n.UnitNavE8 <= 0 {
			continue
		}
		if n.Date <= cutoff && (!ok || n.Date > best) {
			best = n.Date
			ok = true
		}
	}
	return best, ok
}

func DailyPnl(entries []Entry, navs []NavPoint, day string) int64 {
	navD, has := NavOn(navs, day)
	if !has {
		return 0
	}
	sod, err := SharesAtOpen(entries, day)
	if err != nil {
		return 0
	}
	var hold int64
	if prev, ok := PrevNav(navs, day); ok {
		hold = money.MulDiv(sod, navD.UnitNavE8-prev.UnitNavE8, money.FenScale)
	}
	var buyD int64
	for _, e := range Active(entries) {
		if e.Kind != KindBuy || e.OccurDate != day {
			continue
		}
		buyD += money.MulDiv(e.SharesE8, navD.UnitNavE8-e.UnitNavE8, money.FenScale)
	}
	return hold + buyD
}

func CalendarPnl(entries []Entry, navs []NavPoint, from, to string) []DayPnl {
	if from == "" || to == "" || from > to {
		return nil
	}
	var out []DayPnl
	for d := from; ; {
		nav, has := NavOn(navs, d)
		row := DayPnl{Date: d, HangZero: !has, HasNav: has}
		if has {
			row.UnitNavE8 = nav.UnitNavE8
			row.PnlFen = DailyPnl(entries, navs, d)
		} else {
			row.PnlFen = 0
		}
		out = append(out, row)
		if d == to {
			break
		}
		next, err := addDays(d, 1)
		if err != nil {
			break
		}
		d = next
	}
	return out
}

func ResolveNavForOccur(navs []NavPoint, occurDate string, manualE8 int64, manualDate string) (navE8 int64, navDate string, ok bool, estimated bool) {
	if manualE8 > 0 {
		d := manualDate
		if d == "" {
			d = occurDate
		}
		return manualE8, d, true, false
	}
	if n, has := NavOn(navs, occurDate); has {
		return n.UnitNavE8, n.Date, true, false
	}
	if n, has := PrevNav(navs, occurDate); has {
		return n.UnitNavE8, n.Date, true, false
	}
	return 0, "", false, false
}

// ImpliedBuy derives 份额 and 隐含买入净值 from 购入金额, 申报累计收益, and 最新净值.
func ImpliedBuy(cashFen, cumulativeFen, latestNavE8 int64) (sharesE8, navE8 int64, ok bool) {
	mv := cashFen + cumulativeFen
	if cashFen <= 0 || mv <= 0 || latestNavE8 <= 0 {
		return 0, 0, false
	}
	sharesE8 = money.SharesFromCash(mv, latestNavE8)
	if sharesE8 <= 0 {
		return 0, 0, false
	}
	navE8 = money.MulDiv(cashFen, money.FenScale, sharesE8)
	if navE8 <= 0 {
		return 0, 0, false
	}
	return sharesE8, navE8, true
}

func CollectedDailySum(entries []Entry, navs []NavPoint) int64 {
	var sum int64
	for _, n := range navs {
		if n.UnitNavE8 <= 0 {
			continue
		}
		sum += DailyPnl(entries, navs, n.Date)
	}
	return sum
}

func CollectionGap(cumulativeFen, collectedFen int64) int64 {
	return cumulativeFen - collectedFen
}

// CatalogWindows is 30 / 90 / 180 / 360 / 730 (两年) natural days.
var CatalogWindows = []int{30, 90, 180, 360, 730}

// WindowPctE4 is 窗口净值涨跌幅 in hundredths of a percent (5.62% => 562).
func WindowPctE4(navs []NavPoint, days int) (int64, bool) {
	if days <= 0 {
		return 0, false
	}
	end, ok := LatestNav(navs)
	if !ok {
		return 0, false
	}
	startCal, err := addDays(end.Date, -days)
	if err != nil {
		return 0, false
	}
	start, ok := NavOnOrBefore(navs, startCal)
	if !ok || start.UnitNavE8 <= 0 {
		return 0, false
	}
	return money.MulDivRound(end.UnitNavE8-start.UnitNavE8, 10000, start.UnitNavE8), true
}

func addDays(iso string, n int) (string, error) {
	t, err := time.ParseInLocation("2006-01-02", iso, time.UTC)
	if err != nil {
		return "", err
	}
	return t.AddDate(0, 0, n).Format("2006-01-02"), nil
}

func DaysBetween(from, to string) int {
	a, err1 := time.ParseInLocation("2006-01-02", from, time.UTC)
	b, err2 := time.ParseInLocation("2006-01-02", to, time.UTC)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(b.Sub(a).Hours() / 24)
}
