package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Rhymond/go-money"
	"gopkg.in/yaml.v3"
)

const (
	groszeInZloty   = 100
	maxMoneyDigits  = 15 // keeps zloty*100 far below int64 overflow
	moneyDecimalLen = 2
)

func init() {
	// Polish formatting: "1 234,50 zł". The library ships PLN with English separators.
	money.AddCurrency(money.PLN, "zł", "1 $", ",", " ", moneyDecimalLen)
}

// Money is an amount of Polish złoty kept exactly, in grosze. It is a comparable
// value type (use == or Equal); arithmetic and formatting go through
// github.com/Rhymond/go-money so rounding and display rules are not hand-rolled.
//
// JSON: encoded as a decimal string ("49.90"). Decoding also accepts a bare JSON
// number, which is how amounts were stored before decimals were supported (whole
// złoty, e.g. 150), so old events and cart cookies keep their meaning.
// YAML: any scalar, read as text so "49.99" is never a float.
type Money struct {
	minor int64
}

// PLN builds an amount from grosze.
func PLN(grosze int64) Money { return Money{minor: grosze} }

// Zloty builds an amount from whole złoty.
func Zloty(zloty int64) Money { return Money{minor: zloty * groszeInZloty} }

// ParsePLN reads "150", "49.90", "49,9" or "1 234,50". At most two decimals are
// accepted: anything finer would silently lose money.
func ParsePLN(text string) (Money, error) {
	clean := strings.NewReplacer(" ", "", " ", "", "zł", "", "PLN", "").Replace(strings.TrimSpace(text))
	if clean == "" {
		return Money{}, errors.New("empty amount")
	}
	negative := false
	if rest, ok := strings.CutPrefix(clean, "-"); ok {
		negative, clean = true, rest
	}
	clean = strings.Replace(clean, ",", ".", 1)
	whole, fraction, _ := strings.Cut(clean, ".")
	if whole == "" {
		whole = "0"
	}
	if !allDigits(whole) || (fraction != "" && !allDigits(fraction)) || len(fraction) > moneyDecimalLen || len(whole) > maxMoneyDigits {
		return Money{}, fmt.Errorf("invalid amount %q: use at most 2 decimal places", text)
	}
	fraction += strings.Repeat("0", moneyDecimalLen-len(fraction))

	var minor int64
	for _, digit := range whole + fraction {
		minor = minor*10 + int64(digit-'0')
	}
	if negative {
		minor = -minor
	}
	return Money{minor: minor}, nil
}

func allDigits(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// MoneyFromZloty converts a decimal złoty value, such as the read model's REAL
// columns, rounding to the nearest grosz.
func MoneyFromZloty(zloty float64) Money {
	return Money{minor: int64(math.Round(zloty * groszeInZloty))}
}

func (m Money) lib() *money.Money { return money.New(m.minor, money.PLN) }

// Minor is the amount in grosze.
func (m Money) Minor() int64 { return m.minor }

// Zloty is the amount as a decimal number, for storage in REAL columns and
// for display-only maths. Do not add up Zloty() values: use Add.
func (m Money) Zloty() float64 { return m.lib().AsMajorUnits() }

// IsZero reports whether the amount is exactly 0.
func (m Money) IsZero() bool { return m.minor == 0 }

// IsPositive reports whether the amount is greater than 0.
func (m Money) IsPositive() bool { return m.minor > 0 }

// IsNegative reports whether the amount is below 0.
func (m Money) IsNegative() bool { return m.minor < 0 }

// Equal compares amounts.
func (m Money) Equal(other Money) bool { return m.minor == other.minor }

// Less reports whether m is smaller than other.
func (m Money) Less(other Money) bool { return m.minor < other.minor }

// Add returns the sum.
func (m Money) Add(other Money) Money {
	sum, err := m.lib().Add(other.lib())
	if err != nil {
		panic(err) // unreachable: both amounts are PLN
	}
	return Money{minor: sum.Amount()}
}

// Sub returns the difference.
func (m Money) Sub(other Money) Money {
	diff, err := m.lib().Subtract(other.lib())
	if err != nil {
		panic(err) // unreachable: both amounts are PLN
	}
	return Money{minor: diff.Amount()}
}

// Mul multiplies by a quantity (days, pieces).
func (m Money) Mul(quantity int) Money {
	return Money{minor: m.lib().Multiply(int64(quantity)).Amount()}
}

// SumMoney adds amounts up.
func SumMoney(amounts ...Money) Money {
	total := Money{}
	for _, amount := range amounts {
		total = total.Add(amount)
	}
	return total
}

// Plain is the amount with a dot and two decimals, as in JSON and form fields: "49.90".
func (m Money) Plain() string {
	sign := ""
	minor := m.minor
	if minor < 0 {
		sign, minor = "-", -minor
	}
	return fmt.Sprintf("%s%d.%02d", sign, minor/groszeInZloty, minor%groszeInZloty)
}

// String is the Polish display form: "150 zł" for whole amounts, "49,90 zł" otherwise.
func (m Money) String() string {
	if m.minor%groszeInZloty == 0 {
		return withoutFraction(money.New(m.minor/groszeInZloty, money.PLN))
	}
	return m.lib().Display()
}

// withoutFraction renders major units ("1 234 zł") using the library formatter
// configured for PLN but with no decimals.
func withoutFraction(majorUnits *money.Money) string {
	formatter := *majorUnits.Currency().Formatter()
	formatter.Fraction = 0
	return formatter.Format(majorUnits.Amount())
}

// MarshalJSON encodes the amount as a decimal string.
func (m Money) MarshalJSON() ([]byte, error) { return json.Marshal(m.Plain()) }

// UnmarshalJSON accepts "49.90" or a bare number (legacy: whole or decimal złoty).
func (m *Money) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*m = Money{}
		return nil
	}
	var text string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
	} else {
		text = string(data)
	}
	parsed, err := ParsePLN(text)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// UnmarshalYAML reads the scalar as text so decimals are exact.
func (m *Money) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: price must be a number", node.Line)
	}
	parsed, err := ParsePLN(node.Value)
	if err != nil {
		return fmt.Errorf("line %d: %w", node.Line, err)
	}
	*m = parsed
	return nil
}
