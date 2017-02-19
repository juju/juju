// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"
	lxdapi "github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/tools/lxdclient"
)

type rawProvider struct {
	lxdCerts
	lxdConfig
	lxdInstances
	lxdProfiles
	lxdImages
	common.Firewaller

	remote lxdclient.Remote
}

type lxdCerts interface {
	AddCert(lxdclient.Cert) error
	CertByFingerprint(string) (lxdapi.Certificate, error)
	RemoveCertByFingerprint(string) error
}

type lxdConfig interface {
	ServerAddresses() ([]string, error)
	ServerStatus() (*lxdapi.Server, error)
	SetServerConfig(k, v string) error
	SetContainerConfig(container, key, value string) error
}

type lxdInstances interface {
	Instances(string, ...string) ([]lxdclient.Instance, error)
	AddInstance(lxdclient.InstanceSpec) (*lxdclient.Instance, error)
	RemoveInstances(string, ...string) error
	Addresses(string) ([]network.Address, error)
}

type lxdProfiles interface {
	DefaultProfileBridgeName() string
	CreateProfile(string, map[string]string) error
	HasProfile(string) (bool, error)
}

type lxdImages interface {
	EnsureImageExists(series, arch string, sources []lxdclient.Remote, copyProgressHandler func(string)) (string, error)
}

func newRawProvider(spec environs.CloudSpec, local bool) (*rawProvider, error) {
	if local {
		return newLocalRawProvider()
	}
	return newRemoteRawProvider(spec)
}

func newLocalRawProvider() (*rawProvider, error) {
	config := lxdclient.Config{Remote: lxdclient.Local}
	return newRawProviderFromConfig(config)
}

func newRemoteRawProvider(spec environs.CloudSpec) (*rawProvider, error) {
	config, err := getRemoteConfig(spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newRawProviderFromConfig(*config)
}

func newRawProviderFromConfig(config lxdclient.Config) (*rawProvider, error) {
	client, err := lxdclient.Connect(config, true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &rawProvider{
		lxdCerts:     client,
		lxdConfig:    client,
		lxdInstances: client,
		lxdProfiles:  client,
		lxdImages:    client,
		Firewaller:   common.NewFirewaller(),
		remote:       config.Remote,
	}, nil
}

// getRemoteConfig returns a lxdclient.Config using a TCP-based remote.
func getRemoteConfig(spec environs.CloudSpec) (*lxdclient.Config, error) {
	clientCert, serverCert, ok := getCerts(spec)
	if !ok {
		return nil, errors.NotValidf("credentials")
	}
	return &lxdclient.Config{
		lxdclient.Remote{
			Name:          "remote",
			Host:          spec.Endpoint,
			Protocol:      lxdclient.LXDProtocol,
			Cert:          clientCert,
			ServerPEMCert: serverCert,
		},
	}, nil
}

func getCerts(spec environs.CloudSpec) (client *lxdclient.Cert, server string, ok bool) {
	if spec.Credential == nil {
		return nil, "", false
	}
	credAttrs := spec.Credential.Attributes()
	clientCertPEM, ok := credAttrs[credAttrClientCert]
	if !ok {
		return nil, "", false
	}
	clientKeyPEM, ok := credAttrs[credAttrClientKey]
	if !ok {
		return nil, "", false
	}
	serverCertPEM, ok := credAttrs[credAttrServerCert]
	if !ok {
		return nil, "", false
	}
	clientCert := &lxdclient.Cert{
		Name:    "juju",
		CertPEM: []byte(clientCertPEM),
		KeyPEM:  []byte(clientKeyPEM),
	}
	return clientCert, serverCertPEM, true
}
