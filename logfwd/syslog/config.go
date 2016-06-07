// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/cert"
	"github.com/juju/juju/network"
)

const (
	// TLSPort is the TCP port used for syslog-over-TLS.
	TLSPort = 6514
)

// RawConfig holds the raw configuration data for a connection to a
// syslog forwarding target.
type RawConfig struct {
	// Host is the host-port of the syslog host. The format is:
	//
	//   [domain-or-ip-addr] or [domain-or-ip-addr][:port]
	//
	// If the port is not set then the default TLS port (6514) will
	// be used.
	Host string

	// ExpectedServerCert is the TLS certificate that the server must
	// use when the client connects.
	ExpectedServerCert string

	// CACert is the CA cert PEM to use for the client cert.
	ClientCACert string

	// ClientCert is the TLS certificate (x.509, PEM-encoded) to use
	// when connecting.
	ClientCert string

	// ClientCert is the TLS private key (x.509, PEM-encoded) to use
	// when connecting.
	ClientKey string
}

// Validate ensures that the config is currently valid.
func (cfg RawConfig) Validate() error {
	if err := cfg.validateHost(); err != nil {
		return errors.Trace(err)
	}

	if err := cfg.validateSSL(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (cfg RawConfig) validateHost() error {
	if cfg.Host == "" {
		return errors.NewNotValid(nil, "empty Host")
	}

	hostport, err := parseHost(cfg.Host)
	if err != nil {
		return errors.NewNotValid(err, "bad Host")
	}
	if hostport.Type == network.HostName && hostport.Value == "" {
		return errors.NewNotValid(nil, "empty hostname in Host")
	}

	return nil
}

// TODO(ericsnow) network.ParseHostPort() should do this for us...

var hostRE = regexp.MustCompile(`^.*:\d+$`)

func parseHost(host string) (*network.HostPort, error) {
	if _, _, err := net.SplitHostPort(host); err != nil {
		if hostRE.MatchString(host) {
			return nil, errors.Trace(err)
		}
		host = fmt.Sprintf("%s:%d", host, TLSPort)
	}

	hostport, err := network.ParseHostPort(host)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if hostport.Type == network.HostName {
		// network.ParseHostPort() *should* do this for us, but currently does not.
		// TODO(ericsnow) This needs better criteria.
		if strings.ContainsAny(hostport.Value, "#") {
			return nil, errors.Errorf("invalid domain name %q", hostport.Value)
		}
	}

	return hostport, nil
}

// TODO(ericsnow) Split up validateSSL() to make it more follow-able?

func (cfg RawConfig) validateSSL() error {
	if cfg.ExpectedServerCert == "" {
		return errors.NewNotValid(nil, "empty ExpectedServerCert")
	}
	if _, err := cert.ParseCert(cfg.ExpectedServerCert); err != nil {
		err = errors.NewNotValid(err, "")
		return errors.Annotate(err, "invalid ExpectedServerCert")
	}

	if cfg.ClientCert == "" {
		return errors.NewNotValid(nil, "empty ClientCert")
	}

	if cfg.ClientKey == "" {
		return errors.NewNotValid(nil, "empty ClientKey")
	}

	if _, _, err := cert.ParseCertAndKey(cfg.ClientCert, cfg.ClientKey); err != nil {
		if _, err := cert.ParseCert(cfg.ClientCert); err != nil {
			err = errors.NewNotValid(err, "")
			return errors.Annotate(err, "invalid ClientCert")
		}
		err = errors.NewNotValid(err, "bad key or key does not match certificate")
		return errors.Annotate(err, "invalid ClientKey")
	}

	if cfg.ClientCACert == "" {
		return errors.NewNotValid(nil, "empty ClientCACert")
	}
	if _, err := cert.ParseCert(cfg.ClientCACert); err != nil {
		err = errors.NewNotValid(err, "")
		return errors.Annotate(err, "invalid ClientCACert")
	}

	// TODO(ericsnow) Also call cert.Verify() to ensure the CA cert matches?

	return nil
}
