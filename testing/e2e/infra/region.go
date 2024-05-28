package infra

import "math/rand"

const (
	regions = []string{"North Central US", "South Central US", "East US", "East US 2", "West US", "West US 2", "West US 3"}
)

// getLocation returns a location that should be used for the test infrastructure
func getLocation() string {
	// use a random strategy for now to reduce the chances of selecting an overconstrained region. Our test subscription limits
	// the number of resources that can be used for a single region.
	idx := rand.Intn(len(regions))
	return regions[idx]
}
