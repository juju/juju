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

	raw, err := newRawClient(cfg.Remote)
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

// newRawClient connects to the LXD host that is defined in Config.
func newRawClient(remote Remote) (*lxd.Client, error) {
	host := remote.Host
	logger.Debugf("connecting to LXD remote %q: %q", remote.ID(), host)

	if remote.ID() == remoteIDForLocal || host == "" {
		host = "unix://" + lxdshared.VarPath("unix.socket")
	} else {
		_, _, err := net.SplitHostPort(host)
		if err != nil {
			// There is no port here
			host = net.JoinHostPort(host, lxdshared.DefaultPort)
		}
	}

	clientCert := ""
	if remote.Cert != nil && remote.Cert.CertPEM != nil {
		clientCert = string(remote.Cert.CertPEM)
	}

	clientKey := ""
	if remote.Cert != nil && remote.Cert.KeyPEM != nil {
		clientKey = string(remote.Cert.KeyPEM)
	}

	static := false
	public := false
	if remote.Protocol == SimplestreamsProtocol {
		static = true
		public = true
	}

	client, err := lxdNewClientFromInfo(lxd.ConnectInfo{
		Name: remote.ID(),
		RemoteConfig: lxd.RemoteConfig{
			Addr:     host,
			Static:   static,
			Public:   public,
			Protocol: string(remote.Protocol),
		},
		ClientPEMCert: clientCert,
		ClientPEMKey:  clientKey,
		ServerPEMCert: remote.ServerPEMCert,
	})
	if err != nil {
		if remote.ID() == remoteIDForLocal {
			return nil, errors.Annotate(err, "can't connect to the local LXD server")
		}
		return nil, errors.Trace(err)
	}
	return client, nil
}
