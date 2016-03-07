// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"net"

	"github.com/lxc/lxd"
	lxdshared "github.com/lxc/lxd/shared"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.tools.lxdclient")

// Client is a high-level wrapper around the LXD API client.
type Client struct {
	*serverConfigClient
	*certClient
	*profileClient
	*instanceClient
	*imageClient
}

// Connect opens an API connection to LXD and returns a high-level
// Client wrapper around that connection.
func Connect(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	remote := cfg.Remote.ID()

	raw, err := newRawClient(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	conn := &Client{
		serverConfigClient: &serverConfigClient{raw},
		certClient:         &certClient{raw},
		profileClient:      &profileClient{raw},
		instanceClient:     &instanceClient{raw, remote},
		imageClient:        &imageClient{raw},
	}
	return conn, nil
}

var lxdNewClientFromInfo = lxd.NewClientFromInfo
var lxdLoadConfig = lxd.LoadConfig

// TODO(jam) 
func newRawClient(cfg Config) (*lxd.Client, error) {
	logger.Debugf("using LXD remote %q", cfg.Remote.ID())
	remote := cfg.Remote.ID()
	host := cfg.Remote.Host
	if remote == remoteIDForLocal || host == "" {
		host = "unix://" + lxdshared.VarPath("unix.socket")
	} else {
		_, _, err := net.SplitHostPort(host)
		if err != nil {
			// There is no port here
			host = net.JoinHostPort(host, lxdshared.DefaultPort)
		}
	}

	clientCert := ""
	if cfg.Remote.Cert != nil && cfg.Remote.Cert.CertPEM != nil {
		clientCert = string(cfg.Remote.Cert.CertPEM)
	}

	clientKey := ""
	if cfg.Remote.Cert != nil && cfg.Remote.Cert.KeyPEM != nil {
		clientKey = string(cfg.Remote.Cert.KeyPEM)
	}

	client, err := lxdNewClientFromInfo(lxd.ConnectInfo{
		Name:          cfg.Remote.ID(),
		RemoteConfig:  lxd.RemoteConfig{
			Addr: host,
			Static: false,
			Public: false,
			Protocol: "",
			},
		ClientPEMCert: clientCert,
		ClientPEMKey:  clientKey,
		ServerPEMCert: cfg.Remote.ServerPEMCert,
	})
	if err != nil {
		if remote == remoteIDForLocal {
			return nil, errors.Annotate(err, "can't connect to the local LXD server")
		}
		return nil, errors.Trace(err)
	}
	return client, nil
}

// newRawClientToDefault connects to one of the known DefaultRemotes that are
// pre-set inside of the lxd bindings.
func newRawClientToDefault(cfg Config, remote string) (*lxd.Client, error) {
	logger.Debugf("connecting to LXD static remote: %q", remote)

	clientCert := ""
	if cfg.Remote.Cert != nil && cfg.Remote.Cert.CertPEM != nil {
		clientCert = string(cfg.Remote.Cert.CertPEM)
	}

	clientKey := ""
	if cfg.Remote.Cert != nil && cfg.Remote.Cert.KeyPEM != nil {
		clientKey = string(cfg.Remote.Cert.KeyPEM)
	}

	remoteConf, ok := lxd.DefaultRemotes[remote]
	if !ok {
		knownRemotes := make([]string, 0, len(lxd.DefaultRemotes))
		for name, _ := range lxd.DefaultRemotes {
			knownRemotes = append(knownRemotes, name)
		}
		return nil, errors.Errorf("remote %q is not a known DefaultRemote: %v", knownRemotes)
	}

	client, err := lxdNewClientFromInfo(lxd.ConnectInfo{
		Name:          cfg.Remote.ID(),
		RemoteConfig:  remoteConf,
		ClientPEMCert: clientCert,
		ClientPEMKey:  clientKey,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}
