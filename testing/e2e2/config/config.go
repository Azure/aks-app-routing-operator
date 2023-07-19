package config

import (
	"errors"
	"flag"
)

var Flags = &Config{}

func init() {
	flag.StringVar(&Flags.SubscriptionId, "subscription", "", "subscription")
}

type Config struct {
	SubscriptionId string
}

func (c *Config) Validate() error {
	if c.SubscriptionId == "" {
		return errors.New("--subscription is required")
	}

	return nil
}
