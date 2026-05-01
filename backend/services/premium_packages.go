package services

import (
	"os"
	"strconv"
)

// PremiumPackage describes a Telegram Stars premium package.
// StarsPrice is the price in XTR (Telegram Stars). DurationDays is the
// subscription length granted on successful payment.
type PremiumPackage struct {
	ID             string
	Label          string
	DurationMonths int
	DurationDays   int
	StarsPrice     int
	Discount       string
}

// defaultPremiumPackages — keep IDs short so the bot deep link stays under
// Telegram's 64-char start payload limit.
var defaultPremiumPackages = []PremiumPackage{
	{ID: "1m", Label: "1 oy", DurationMonths: 1, DurationDays: 30, StarsPrice: 100},
	{ID: "3m", Label: "3 oy", DurationMonths: 3, DurationDays: 90, StarsPrice: 270, Discount: "-10%"},
	{ID: "6m", Label: "6 oy", DurationMonths: 6, DurationDays: 180, StarsPrice: 500, Discount: "-17%"},
	{ID: "12m", Label: "12 oy", DurationMonths: 12, DurationDays: 365, StarsPrice: 1000, Discount: "-17%"},
}

// PremiumPackages returns the configured packages, allowing per-package star
// price overrides via env vars: PREMIUM_STARS_PRICE_1M, _3M, _6M, _12M.
func PremiumPackages() []PremiumPackage {
	out := make([]PremiumPackage, len(defaultPremiumPackages))
	for i, p := range defaultPremiumPackages {
		envKey := "PREMIUM_STARS_PRICE_" + envSuffix(p.ID)
		if v := os.Getenv(envKey); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				p.StarsPrice = n
			}
		}
		out[i] = p
	}
	return out
}

// PremiumPackageByID returns the package and ok=true if the id matches.
func PremiumPackageByID(id string) (PremiumPackage, bool) {
	for _, p := range PremiumPackages() {
		if p.ID == id {
			return p, true
		}
	}
	return PremiumPackage{}, false
}

func envSuffix(id string) string {
	switch id {
	case "1m":
		return "1M"
	case "3m":
		return "3M"
	case "6m":
		return "6M"
	case "12m":
		return "12M"
	}
	return id
}
