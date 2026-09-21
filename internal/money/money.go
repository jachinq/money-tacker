package money

import "math/bits"

// Fen is CNY * 100.
// NavE8 is unit NAV * 1e8.
// ShareE8 is shares * 1e8.
//
// yuan = fen/100 = shares_e8/1e8 * nav_e8/1e8
// fen  = shares_e8 * nav_e8 / 1e14

const (
	YuanFen   int64 = 100
	ScaleE8   int64 = 100_000_000
	FenScale  int64 = 100_000_000_000_000 // 1e14
)

func MulDiv(a, b, div int64) int64 {
	if div == 0 {
		return 0
	}
	neg := (a < 0) != (b < 0)
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	if div < 0 {
		neg = !neg
		div = -div
	}
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	q, _ := bits.Div64(hi, lo, uint64(div))
	out := int64(q)
	if neg {
		return -out
	}
	return out
}

func SharesFromCash(cashFen, navE8 int64) int64 {
	if cashFen <= 0 || navE8 <= 0 {
		return 0
	}
	return MulDiv(cashFen, FenScale, navE8)
}

func CashFromShares(sharesE8, navE8 int64) int64 {
	if sharesE8 <= 0 || navE8 <= 0 {
		return 0
	}
	return MulDiv(sharesE8, navE8, FenScale)
}

func Amortize(costFen, redeemSharesE8, sharesBeforeE8 int64) int64 {
	if sharesBeforeE8 <= 0 || redeemSharesE8 <= 0 {
		return 0
	}
	return MulDiv(costFen, redeemSharesE8, sharesBeforeE8)
}

func ParseYuanToFen(s string) (int64, bool) {
	return parseFixed(s, 2)
}

func ParseNavToE8(s string) (int64, bool) {
	return parseFixed(s, 8)
}

func FormatFen(fen int64) string {
	return formatFixed(fen, 2)
}

func FormatNav(navE8 int64) string {
	return formatFixed(navE8, 8)
}

func FormatShares(sharesE8 int64) string {
	return formatFixed(sharesE8, 8)
}

func parseFixed(s string, scale int) (int64, bool) {
	if s == "" {
		return 0, false
	}
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
		if s == "" {
			return 0, false
		}
	}
	var intPart, frac int64
	seenDot := false
	fracDigits := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '.' {
			if seenDot {
				return 0, false
			}
			seenDot = true
			continue
		}
		if c < '0' || c > '9' {
			return 0, false
		}
		d := int64(c - '0')
		if !seenDot {
			intPart = intPart*10 + d
		} else {
			if fracDigits >= scale {
				continue
			}
			frac = frac*10 + d
			fracDigits++
		}
	}
	for fracDigits < scale {
		frac *= 10
		fracDigits++
	}
	var factor int64 = 1
	for i := 0; i < scale; i++ {
		factor *= 10
	}
	v := intPart*factor + frac
	if neg {
		v = -v
	}
	return v, true
}

func formatFixed(v int64, scale int) string {
	neg := v < 0
	if neg {
		v = -v
	}
	var factor int64 = 1
	for i := 0; i < scale; i++ {
		factor *= 10
	}
	intPart := v / factor
	frac := v % factor
	b := make([]byte, 0, 24)
	if neg {
		b = append(b, '-')
	}
	b = append(b, itoa(intPart)...)
	b = append(b, '.')
	fracStr := itoa(frac)
	for len(fracStr) < scale {
		fracStr = "0" + fracStr
	}
	b = append(b, fracStr...)
	return string(b)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
