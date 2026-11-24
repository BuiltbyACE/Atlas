package invoicepdf

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// LineAmountMinor is quantity x unit price in minor units, rounded to the
// nearest minor unit. The sign follows the quantity, so a negative quantity is a
// discount.
func LineAmountMinor(quantity float64, unitPriceMinor int64) int64 {
	return int64(math.Round(quantity * float64(unitPriceMinor)))
}

// LineTaxMinor is the tax on an amount at a basis-point rate, rounded half away
// from zero.
func LineTaxMinor(amountMinor int64, taxRateBP int) int64 {
	return int64(math.Round(float64(amountMinor) * float64(taxRateBP) / 10000))
}

// Totals are the invoice sums in minor units.
type Totals struct {
	SubtotalMinor int64
	TaxMinor      int64
	TotalMinor    int64
	Quantity      float64
}

// Sum totals the lines.
func Sum(lines []Line) Totals {
	var t Totals
	for _, l := range lines {
		amount := LineAmountMinor(l.Quantity, l.UnitPriceMinor)
		t.SubtotalMinor += amount
		t.TaxMinor += LineTaxMinor(amount, l.TaxRateBP)
		t.Quantity += l.Quantity
	}
	t.TotalMinor = t.SubtotalMinor + t.TaxMinor
	return t
}

// Money formats minor units with thousands separators: 2210000 -> "22,100",
// 2210050 -> "22,100.50", negative -> "-2,600".
func Money(minor int64) string {
	neg := minor < 0
	if neg {
		minor = -minor
	}
	whole, frac := minor/100, minor%100
	s := groupThousands(whole)
	if frac != 0 {
		s += fmt.Sprintf(".%02d", frac)
	}
	if neg {
		return "-" + s
	}
	return s
}

// Symbol is the short currency prefix: KES prints "KSh", others print the code.
func Symbol(currency string) string {
	switch strings.ToUpper(currency) {
	case "", "KES":
		return "KSh"
	default:
		return strings.ToUpper(currency)
	}
}

// Quantity formats a quantity without trailing zeros: 19 -> "19", 1.5 -> "1.5".
func Quantity(q float64) string {
	if q == math.Trunc(q) {
		return groupThousands(int64(q))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", q), "0"), ".")
}

func groupThousands(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

var (
	ones = []string{"", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine",
		"Ten", "Eleven", "Twelve", "Thirteen", "Fourteen", "Fifteen", "Sixteen", "Seventeen", "Eighteen", "Nineteen"}
	tens = []string{"", "", "Twenty", "Thirty", "Forty", "Fifty", "Sixty", "Seventy", "Eighty", "Ninety"}
)

func below1000(n int64) string {
	var parts []string
	if n >= 100 {
		parts = append(parts, ones[n/100]+" Hundred")
		n %= 100
		if n > 0 {
			parts = append(parts, "and")
		}
	}
	switch {
	case n >= 20:
		w := tens[n/10]
		if n%10 != 0 {
			w += "-" + ones[n%10]
		}
		parts = append(parts, w)
	case n > 0:
		parts = append(parts, ones[n])
	}
	return strings.Join(parts, " ")
}

// words spells a non-negative whole number in British English.
func words(n int64) string {
	if n == 0 {
		return "Zero"
	}
	scales := []struct {
		size int64
		name string
	}{{1_000_000_000, "Billion"}, {1_000_000, "Million"}, {1_000, "Thousand"}}
	var parts []string
	for _, sc := range scales {
		if n >= sc.size {
			parts = append(parts, below1000(n/sc.size)+" "+sc.name)
			n %= sc.size
		}
	}
	if n > 0 {
		// "Twenty-Two Thousand One Hundred" reads without "and"; add it only when
		// the last group is below a hundred and something larger came before it.
		last := below1000(n)
		if len(parts) > 0 && n < 100 {
			last = "and " + last
		}
		parts = append(parts, last)
	}
	return strings.Join(parts, " ")
}

// AmountInWords spells a total the way a cheque would: "Twenty-Two Thousand One
// Hundred Kenyan Shillings Only", with cents as "and Fifty Cents".
func AmountInWords(minor int64, currency string) string {
	if minor < 0 {
		minor = -minor
	}
	major, cents := minor/100, minor%100
	unit, subunit := "Kenyan Shillings", "Cents"
	switch strings.ToUpper(currency) {
	case "", "KES":
	case "USD":
		unit, subunit = "US Dollars", "Cents"
	default:
		unit = strings.ToUpper(currency)
	}
	out := words(major) + " " + unit
	if cents != 0 {
		out += " and " + words(cents) + " " + subunit
	}
	return out + " Only"
}

// StatusFor derives the label shown on the invoice from where it is in its life.
// today is passed in so the result is deterministic.
func StatusFor(status string, due *time.Time, today time.Time) (string, Tone) {
	switch status {
	case "paid":
		return "Paid", ToneGood
	case "draft", "scheduled":
		return "Draft", ToneNeutral
	case "approved":
		return "Approved", ToneNeutral
	case "disputed":
		return "Disputed", ToneDanger
	case "written_off":
		return "Written off", ToneNeutral
	case "partially_paid":
		if due != nil && dayBefore(*due, today) {
			return "Overdue", ToneDanger
		}
		return "Part paid", ToneNeutral
	case "overdue":
		return "Overdue", ToneDanger
	default: // issued
		if due != nil && dayBefore(*due, today) {
			return "Overdue", ToneDanger
		}
		return "Issued", ToneNeutral
	}
}

// NoticeFor is the payment notice for an unpaid invoice, or "" when none applies.
func NoticeFor(label string, totalMinor int64, currency string, due *time.Time) string {
	amount := Symbol(currency) + " " + Money(totalMinor)
	switch label {
	case "Overdue":
		return "Payment notice: Payment of " + amount + " is now overdue and is due in full."
	case "Issued", "Part paid":
		if due != nil {
			return "Payment of " + amount + " is due by " + due.Format("02/01/2006") + "."
		}
	}
	return ""
}

func dayBefore(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC).Before(time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC))
}
