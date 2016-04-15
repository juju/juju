// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog

import (
	"crypto/tls"
	"net/url"

	"github.com/juju/errors"

	"github.com/juju/juju/cert"
)

// ErrNotConfigured is returned when any of the rsyslog forwarding config
// values is not set.
var ErrNotConfigured = errors.Errorf("rsyslog forwarding not configured")

// ClientConfig holds information needed to connect to a
// rsyslog server.
type ClientConfig struct {
	URL    string
	CACert string
	Cert   string
	Key    string
}

// Validate validates the config values.
func (c ClientConfig) Validate() error {
	if c.URL == "" && c.CACert == "" && c.Cert == "" && c.Key == "" {
		return errors.Trace(ErrNotConfigured)
	}
	if c.URL == "" {
		return errors.NotValidf("URL")
	}
	u, err := url.Parse(c.URL)
	if err != nil {
		return errors.NewNotValid(err, "URL not valid")
	}
	if u.Scheme != "https" {
		return errors.Errorf("URL not valid; https required")
	}
	_, err = cert.ParseCert(c.CACert)
	if err != nil {
		return errors.NewNotValid(err, "CACert not valid")
	}
	_, err = cert.ParseCert(c.Cert)
	if err != nil {
		return errors.NewNotValid(err, "Cert not valid")
	}
	_, err = tls.X509KeyPair([]byte(c.Cert), []byte(c.Key))
	if err != nil {
		return errors.NewNotValid(err, "Cert/Key pair not valid")
	}
	return nil
}
