package money

import "testing"

func TestSharesFromCash_specExample(t *testing.T) {
	cash, ok := ParseYuanToFen("10000")
	if !ok {
		t.Fatal("parse cash")
	}
	nav, ok := ParseNavToE8("1.0562")
	if !ok {
		t.Fatal("parse nav")
	}
	got := SharesFromCash(cash, nav)
	if got != 946790380609 {
		t.Fatalf("shares_e8=%d want 946790380609", got)
	}
}

func TestCashFromShares_defaultProductDoesNotLoseFen(t *testing.T) {
	cash, _ := ParseYuanToFen("10000")
	nav, _ := ParseNavToE8("1.0562")
	shares := SharesFromCash(cash, nav)
	if shares != 946790380609 {
		t.Fatalf("shares_e8=%d", shares)
	}
	mv := CashFromShares(shares, nav)
	if mv != cash {
		t.Fatalf("market fen=%d want %d (truncation would yield 999999)", mv, cash)
	}
}

func TestMarketValueRoundTripSameNav(t *testing.T) {
	cash, _ := ParseYuanToFen("10000")
	nav, _ := ParseNavToE8("1.0000")
	shares := SharesFromCash(cash, nav)
	if shares != 1_000_000_000_000 {
		t.Fatalf("shares %d", shares)
	}
	mv := CashFromShares(shares, nav)
	if mv != 1_000_000 {
		t.Fatalf("market fen=%d", mv)
	}
}

func TestParseFormatFen(t *testing.T) {
	v, ok := ParseYuanToFen("12.50")
	if !ok || v != 1250 {
		t.Fatalf("got %d ok=%v", v, ok)
	}
	if FormatFen(1250) != "12.50" {
		t.Fatalf("format %s", FormatFen(1250))
	}
}

func TestAmortizeHalf(t *testing.T) {
	got := Amortize(1_000_000, 100, 200)
	if got != 500_000 {
		t.Fatalf("got %d", got)
	}
}

func TestFormatSignedPctE4(t *testing.T) {
	if FormatSignedPctE4(562) != "+5.62" {
		t.Fatalf("got %s", FormatSignedPctE4(562))
	}
	if FormatSignedPctE4(-100) != "-1.00" {
		t.Fatalf("got %s", FormatSignedPctE4(-100))
	}
	if FormatSignedPctE4(0) != "0.00" {
		t.Fatalf("got %s", FormatSignedPctE4(0))
	}
}
