package config

import (
	"errors"
	"flag"
)

var Flags = &Config{}

func init() {
	flag.StringVar(&Flags.SubscriptionId, "subscription", "", "subscription")
	flag.StringVar(&Flags.TenantId, "tenant", "", "tenant")
}

type Config struct {
	SubscriptionId string
	TenantId       string
}

func (c *Config) Validate() error {
	if c.SubscriptionId == "" {
		return errors.New("--subscription is required")
	}

	if c.TenantId == "" {
		return errors.New("--tenant is required")
	}

	return nil
}
