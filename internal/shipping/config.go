// Package shipping sells shop items with delivery: the delivery options and
// the sender are described in data/shipping.yml, and parcels are ordered at
// Polkurier (github.com/mleczakm/polkurier-go): parcel locker / pickup point
// search for the checkout, parcels with labels for the admin, and tracking.
package shipping

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"cargo.mleczki.pl/internal/domain"
)

// Kind tells how an option delivers the order.
type Kind string

const (
	// KindPickup is collection in person; it never needs Polkurier.
	KindPickup Kind = "pickup"
	// KindLocker delivers to a parcel locker or pickup point chosen by the customer.
	KindLocker Kind = "locker"
	// KindCourier delivers to the customer's address.
	KindCourier Kind = "courier"
)

// PickupCode is the code of the always-available collection in person.
const PickupCode = "pickup"

var codePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Option is a way of getting an order to the customer.
type Option struct {
	Code        string       `yaml:"code" json:"code"`
	Name        string       `yaml:"name" json:"name"`
	Description string       `yaml:"description" json:"description,omitempty"`
	Kind        Kind         `yaml:"kind" json:"kind"`
	Carrier     string       `yaml:"carrier" json:"carrier,omitempty"` // Polkurier service code, e.g. INPOST_PACZKOMAT
	Price       domain.Money `yaml:"price" json:"-"`
	FreeFrom    domain.Money `yaml:"freeFrom" json:"-"` // shop total from which the delivery is free; zero = never
}

// NeedsPolkurier reports whether the option can only be offered with Polkurier configured.
func (o Option) NeedsPolkurier() bool { return o.Kind != KindPickup }

// NeedsLocker reports whether the customer has to choose a locker or point.
func (o Option) NeedsLocker() bool { return o.Kind == KindLocker }

// NeedsAddress reports whether the parcel goes to the customer's address.
func (o Option) NeedsAddress() bool { return o.Kind == KindCourier }

// Cost is what the customer pays for the delivery of goods worth goods.
func (o Option) Cost(goods domain.Money) domain.Money {
	if o.FreeFrom.IsPositive() && !goods.Less(o.FreeFrom) {
		return domain.Money{}
	}
	return o.Price
}

// Sender is the address printed on parcel labels.
type Sender struct {
	Company     string `yaml:"company"`
	Person      string `yaml:"person"`
	Street      string `yaml:"street"`
	HouseNumber string `yaml:"houseNumber"`
	FlatNumber  string `yaml:"flatNumber"`
	Postcode    string `yaml:"postcode"`
	City        string `yaml:"city"`
	Email       string `yaml:"email"`
	Phone       string `yaml:"phone"`
}

// Parcel holds the defaults used to size a parcel from the ordered goods.
type Parcel struct {
	LengthCm           int    `yaml:"lengthCm"`
	WidthCm            int    `yaml:"widthCm"`
	HeightCm           int    `yaml:"heightCm"`
	PackagingWeightG   int    `yaml:"packagingWeightG"`
	DefaultItemWeightG int    `yaml:"defaultItemWeightG"`
	Description        string `yaml:"description"` // content printed on the label, max 30 characters
	Insure             bool   `yaml:"insure"`      // insure parcels for the value of the goods
}

// Config is data/shipping.yml.
type Config struct {
	Sender  Sender   `yaml:"sender"`
	Parcel  Parcel   `yaml:"parcel"`
	Options []Option `yaml:"options"`
}

// DefaultConfig is used when there is no data/shipping.yml: collection in person only.
func DefaultConfig() Config {
	return Config{
		Parcel:  Parcel{LengthCm: 30, WidthCm: 20, HeightCm: 10, PackagingWeightG: 300, DefaultItemWeightG: 500, Description: "Akcesoria rowerowe"},
		Options: []Option{{Code: PickupCode, Name: "Odbiór osobisty w Radzyminie", Kind: KindPickup}},
	}
}

// Option finds an option by code.
func (c Config) Option(code string) (Option, bool) {
	for _, o := range c.Options {
		if o.Code == code {
			return o, true
		}
	}
	return Option{}, false
}

// Available lists the options the shop can honour now: the ones that need
// Polkurier are left out until it is configured.
func (c Config) Available(polkurier bool) []Option {
	var out []Option
	for _, o := range c.Options {
		if polkurier || !o.NeedsPolkurier() {
			out = append(out, o)
		}
	}
	return out
}

// ParseConfig reads and validates a shipping.yml document.
func ParseConfig(raw []byte) (Config, error) {
	cfg := DefaultConfig()
	cfg.Options = nil
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse shipping config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	var problems []error
	seen := map[string]bool{}
	hasPickup := false
	for i := range c.Options {
		o := &c.Options[i]
		o.Code = strings.TrimSpace(o.Code)
		o.Name = strings.TrimSpace(o.Name)
		o.Carrier = strings.ToUpper(strings.TrimSpace(o.Carrier))
		if o.Kind == "" {
			o.Kind = KindPickup
		}
		switch {
		case !codePattern.MatchString(o.Code):
			problems = append(problems, fmt.Errorf("option %d: code %q must be lowercase letters, digits and dashes", i+1, o.Code))
		case seen[o.Code]:
			problems = append(problems, fmt.Errorf("option %q is repeated", o.Code))
		case o.Name == "":
			problems = append(problems, fmt.Errorf("option %q needs a name", o.Code))
		case o.Kind != KindPickup && o.Kind != KindLocker && o.Kind != KindCourier:
			problems = append(problems, fmt.Errorf("option %q: kind %q must be pickup, locker or courier", o.Code, o.Kind))
		case o.NeedsPolkurier() && o.Carrier == "":
			problems = append(problems, fmt.Errorf("option %q needs a carrier (Polkurier service code)", o.Code))
		case o.Price.IsNegative() || o.FreeFrom.IsNegative():
			problems = append(problems, fmt.Errorf("option %q: price and freeFrom must not be negative", o.Code))
		}
		seen[o.Code] = true
		hasPickup = hasPickup || o.Kind == KindPickup
	}
	if !hasPickup {
		// Rentals and goods can always be collected, so the checkout never ends up without a way to deliver.
		c.Options = append([]Option{{Code: PickupCode, Name: "Odbiór osobisty w Radzyminie", Kind: KindPickup}}, c.Options...)
	}
	if c.Parcel.LengthCm <= 0 || c.Parcel.WidthCm <= 0 || c.Parcel.HeightCm <= 0 {
		def := DefaultConfig().Parcel
		c.Parcel.LengthCm, c.Parcel.WidthCm, c.Parcel.HeightCm = def.LengthCm, def.WidthCm, def.HeightCm
	}
	if c.Parcel.PackagingWeightG < 0 || c.Parcel.DefaultItemWeightG <= 0 {
		def := DefaultConfig().Parcel
		c.Parcel.PackagingWeightG, c.Parcel.DefaultItemWeightG = def.PackagingWeightG, def.DefaultItemWeightG
	}
	if c.Parcel.Description == "" {
		c.Parcel.Description = DefaultConfig().Parcel.Description
	}
	if r := []rune(c.Parcel.Description); len(r) > 30 {
		c.Parcel.Description = string(r[:30])
	}
	return errors.Join(problems...)
}

// Complete reports what is missing for parcels to be ordered, or "" when the sender is filled in.
func (s Sender) Missing() string {
	var missing []string
	for name, value := range map[string]string{
		"person": s.Person, "street": s.Street, "houseNumber": s.HouseNumber,
		"postcode": s.Postcode, "city": s.City, "email": s.Email, "phone": s.Phone,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return "uzupełnij nadawcę w data/shipping.yml (sender): " + strings.Join(sortedStrings(missing), ", ")
}

// ConfigFile loads a Config from disk and reloads it when the file changes.
type ConfigFile struct {
	Path string

	mu      sync.Mutex
	loaded  time.Time
	modTime time.Time
	cfg     Config
	err     error
}

// Load returns the current configuration. A broken file is reported through the
// error while the last good configuration (or the default) keeps serving.
func (f *ConfigFile) Load() (Config, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	info, statErr := os.Stat(f.Path)
	switch {
	case statErr != nil && errors.Is(statErr, os.ErrNotExist):
		if f.cfg.Options == nil || !f.modTime.IsZero() {
			f.cfg, f.err, f.modTime = DefaultConfig(), nil, time.Time{}
		}
		return f.cfg, nil
	case statErr != nil:
		return f.fallback(statErr)
	case f.cfg.Options != nil && info.ModTime().Equal(f.modTime):
		return f.cfg, f.err
	}

	raw, err := os.ReadFile(f.Path) // #nosec G304 - path is configured by the operator
	if err != nil {
		return f.fallback(err)
	}
	cfg, err := ParseConfig(raw)
	f.modTime = info.ModTime()
	if err != nil {
		f.err = fmt.Errorf("%s: %w", f.Path, err)
		if f.cfg.Options == nil {
			f.cfg = DefaultConfig()
		}
		return f.cfg, f.err
	}
	f.cfg, f.err = cfg, nil
	return f.cfg, nil
}

func (f *ConfigFile) fallback(err error) (Config, error) {
	if f.cfg.Options == nil {
		f.cfg = DefaultConfig()
	}
	return f.cfg, err
}
