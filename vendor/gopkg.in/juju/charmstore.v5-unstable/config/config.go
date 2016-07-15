// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The config package defines configuration parameters for
// the charm store.
package config // import "gopkg.in/juju/charmstore.v5-unstable/config"

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/yaml.v2"
)

type Config struct {
	// TODO(rog) rename this to MongoAddr - it's not a URL.
	MongoURL          string            `yaml:"mongo-url,omitempty"`
	AuditLogFile      string            `yaml:"audit-log-file,omitempty"`
	AuditLogMaxSize   int               `yaml:"audit-log-max-size,omitempty"`
	AuditLogMaxAge    int               `yaml:"audit-log-max-age,omitempty"`
	APIAddr           string            `yaml:"api-addr,omitempty"`
	AuthUsername      string            `yaml:"auth-username,omitempty"`
	AuthPassword      string            `yaml:"auth-password,omitempty"`
	ESAddr            string            `yaml:"elasticsearch-addr,omitempty"` // elasticsearch is optional
	IdentityPublicKey *bakery.PublicKey `yaml:"identity-public-key,omitempty"`
	IdentityLocation  string            `yaml:"identity-location"`
	TermsPublicKey    *bakery.PublicKey `yaml:"terms-public-key,omitempty"`
	TermsLocation     string            `yaml:"terms-location,omitempty"`
	// The identity API is optional
	IdentityAPIURL    string          `yaml:"identity-api-url,omitempty"`
	AgentUsername     string          `yaml:"agent-username,omitempty"`
	AgentKey          *bakery.KeyPair `yaml:"agent-key,omitempty"`
	MaxMgoSessions    int             `yaml:"max-mgo-sessions,omitempty"`
	RequestTimeout    DurationString  `yaml:"request-timeout,omitempty"`
	StatsCacheMaxAge  DurationString  `yaml:"stats-cache-max-age,omitempty"`
	SearchCacheMaxAge DurationString  `yaml:"search-cache-max-age,omitempty"`
	Database          string          `yaml:"database,omitempty"`
}

func (c *Config) validate() error {
	var missing []string
	if c.MongoURL == "" {
		missing = append(missing, "mongo-url")
	}
	if c.APIAddr == "" {
		missing = append(missing, "api-addr")
	}
	if c.AuthUsername == "" {
		missing = append(missing, "auth-username")
	}
	if strings.Contains(c.AuthUsername, ":") {
		return fmt.Errorf("invalid user name %q (contains ':')", c.AuthUsername)
	}
	if c.AuthPassword == "" {
		missing = append(missing, "auth-password")
	}
	if len(missing) != 0 {
		return fmt.Errorf("missing fields %s in config file", strings.Join(missing, ", "))
	}
	return nil
}

// Read reads a charm store configuration file from the
// given path.
func Read(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errgo.Notef(err, "cannot open config file")
	}
	defer f.Close()
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, errgo.Notef(err, "cannot read %q", path)
	}
	var conf Config
	err = yaml.Unmarshal(data, &conf)
	if err != nil {
		return nil, errgo.Notef(err, "cannot parse %q", path)
	}
	if err := conf.validate(); err != nil {
		return nil, errgo.Mask(err)
	}
	return &conf, nil
}

// DurationString holds a duration that marshals and
// unmarshals as a friendly string.
type DurationString struct {
	time.Duration
}

func (dp *DurationString) UnmarshalText(data []byte) error {
	d, err := time.ParseDuration(string(data))
	if err != nil {
		return errgo.Mask(err)
	}
	dp.Duration = d
	return nil
}
