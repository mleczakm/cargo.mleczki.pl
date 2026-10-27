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
	Code        string       `json:"code"                  yaml:"code"`
	Name        string       `json:"name"                  yaml:"name"`
	Description string       `json:"description,omitempty" yaml:"description"`
	Kind        Kind         `json:"kind"                  yaml:"kind"`
	Carrier     string       `json:"carrier,omitempty"     yaml:"carrier"` // Polkurier service code, e.g. INPOST_PACZKOMAT
	Price       domain.Money `json:"-"                     yaml:"price"`
	FreeFrom    domain.Money `json:"-"                     yaml:"freeFrom"` // shop total from which the delivery is free; zero = never
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
		option := &c.Options[i]
		option.Code = strings.TrimSpace(option.Code)
		option.Name = strings.TrimSpace(option.Name)
		option.Carrier = strings.ToUpper(strings.TrimSpace(option.Carrier))
		if option.Kind == "" {
			option.Kind = KindPickup
		}
		if problem := validateOption(*option, i, seen); problem != nil {
			problems = append(problems, problem)
		}
		seen[option.Code] = true
		hasPickup = hasPickup || option.Kind == KindPickup
	}
	if !hasPickup {
		// Rentals and goods can always be collected, so the checkout never ends up without a way to deliver.
		c.Options = append([]Option{{Code: PickupCode, Name: "Odbiór osobisty w Radzyminie", Kind: KindPickup}}, c.Options...)
	}
	c.normalizeParcel()
	return errors.Join(problems...)
}

func validateOption(option Option, index int, seen map[string]bool) error {
	switch {
	case !codePattern.MatchString(option.Code):
		return fmt.Errorf("option %d: code %q must be lowercase letters, digits and dashes", index+1, option.Code)
	case seen[option.Code]:
		return fmt.Errorf("option %q is repeated", option.Code)
	case option.Name == "":
		return fmt.Errorf("option %q needs a name", option.Code)
	case option.Kind != KindPickup && option.Kind != KindLocker && option.Kind != KindCourier:
		return fmt.Errorf("option %q: kind %q must be pickup, locker or courier", option.Code, option.Kind)
	case option.NeedsPolkurier() && option.Carrier == "":
		return fmt.Errorf("option %q needs a carrier (Polkurier service code)", option.Code)
	case option.Price.IsNegative() || option.FreeFrom.IsNegative():
		return fmt.Errorf("option %q: price and freeFrom must not be negative", option.Code)
	}
	return nil
}

func (c *Config) normalizeParcel() {
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
