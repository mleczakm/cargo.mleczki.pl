package shipping

import (
	"regexp"
	"sort"
	"strings"
)

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

var streetRe = regexp.MustCompile(`^(.*?[^\d\s])\s*(\d+[A-Za-z]?(?:[-/]\d+[A-Za-z]?)?)(?:\s*(?:/|m\.?|lok\.?)\s*(\w+))?$`)

// SplitStreet splits "Poznańska 12/3" into street "Poznańska", house number "12" and flat "3".
func SplitStreet(s string) (street, house, flat string) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(s), "ul. "), "ul."))
	m := streetRe.FindStringSubmatch(s)
	if m == nil {
		return s, "", ""
	}
	street, house, flat = strings.TrimSpace(strings.TrimRight(m[1], ", ")), m[2], m[3]
	if flat == "" {
		if h, f, ok := strings.Cut(house, "/"); ok {
			house, flat = h, f
		}
	}
	return street, house, flat
}

// SplitPostcodeCity splits "05-250 Radzymin" into postcode and city.
func SplitPostcodeCity(s string) (postcode, city string) {
	s = strings.TrimSpace(s)
	if m := postcodeRe.FindString(s); m != "" {
		return m, strings.TrimSpace(strings.TrimLeft(strings.Replace(s, m, "", 1), " ,"))
	}
	return "", s
}

var postcodeRe = regexp.MustCompile(`\b\d{2}-\d{3}\b`)

// ValidPostcode reports whether s looks like a Polish postcode (00-000).
func ValidPostcode(s string) bool {
	return postcodeRe.MatchString(s) && len(strings.TrimSpace(s)) == 6
}
