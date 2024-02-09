// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog

import (
	"crypto/tls"
	"crypto/x509"
	"net"

	"github.com/juju/errors"
	"github.com/juju/utils/v4/cert"
)

// RawConfig holds the raw configuration data for a connection to a
// syslog forwarding target.
type RawConfig struct {
	// Enabled is true if the log forwarding feature is enabled.
	Enabled bool

	// Host is the host-port of the syslog host. The format is:
	//
	//   [domain-or-ip-addr] or [domain-or-ip-addr][:port]
	//
	// If the port is not set then the default TLS port (6514) will
	// be used.
	Host string

	// CACert is the TLS CA certificate (x.509, PEM-encoded) to use
	// for validating the server certificate when connecting.
	CACert string

	// ClientCert is the TLS certificate (x.509, PEM-encoded) to use
	// when connecting.
	ClientCert string

	// ClientKey is the TLS private key (x.509, PEM-encoded) to use
	// when connecting.
	ClientKey string
}

// Validate ensures that the config is currently valid.
func (cfg RawConfig) Validate() error {
	if err := cfg.validateHost(); err != nil {
		return errors.Trace(err)
	}

	if cfg.Enabled || cfg.ClientKey != "" || cfg.ClientCert != "" || cfg.CACert != "" {
		if _, err := cfg.tlsConfig(); err != nil {
			return errors.Annotate(err, "validating TLS config")
		}
	}
	return nil
}

func (cfg RawConfig) validateHost() error {
	host, _, err := net.SplitHostPort(cfg.Host)
	if err != nil {
		host = cfg.Host
	}
	if host == "" && cfg.Enabled {
		return errors.NotValidf("Host %q", cfg.Host)
	}
	return nil
}

func (cfg RawConfig) tlsConfig() (*tls.Config, error) {
	clientCert, err := tls.X509KeyPair([]byte(cfg.ClientCert), []byte(cfg.ClientKey))
	if err != nil {
		return nil, errors.Annotate(err, "parsing client key pair")
	}

	caCert, err := cert.ParseCert(cfg.CACert)
	if err != nil {
		return nil, errors.Annotate(err, "parsing CA certificate")
	}
	rootCAs := x509.NewCertPool()
	rootCAs.AddCert(caCert)

	return &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      rootCAs,
	}, nil
}
