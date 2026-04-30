package services

import (
	"os"
	"strconv"
)

// PremiumPackage mirrors backend/services/premium_packages.go. Star prices may
// be overridden via env (PREMIUM_STARS_PRICE_1M / _3M / _6M / _12M); keep both
// services in sync so invoices match what the backend will accept.
type PremiumPackage struct {
	ID             string
	Label          string
	DurationMonths int
	StarsPrice     int
}

var defaultPremiumPackages = []PremiumPackage{
	{ID: "1m", Label: "1 oy", DurationMonths: 1, StarsPrice: 100},
	{ID: "3m", Label: "3 oy", DurationMonths: 3, StarsPrice: 270},
	{ID: "6m", Label: "6 oy", DurationMonths: 6, StarsPrice: 500},
	{ID: "12m", Label: "12 oy", DurationMonths: 12, StarsPrice: 900},
}

func PremiumPackageByID(id string) (PremiumPackage, bool) {
	for _, p := range defaultPremiumPackages {
		envKey := "PREMIUM_STARS_PRICE_" + envSuffix(p.ID)
		if v := os.Getenv(envKey); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				p.StarsPrice = n
			}
		}
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
