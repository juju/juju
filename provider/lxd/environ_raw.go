// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	lxdclient "github.com/lxc/lxd/client"
	lxdapi "github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	jujulxdclient "github.com/juju/juju/tools/lxdclient"
)

// TODO (manadart 2018-05-09) This is really nothing but an LXD server and does
// not need its own type.
//
// Side-note on terms: what used to be called a client will be our new "server".
// This is for congruence with the LXD package, which presents things like
// "ContainerServer" and "ImageServer" for interaction with LXD.
//
// As the LXD facility is refactored, this will be removed altogether.
// As an interim measure, the new and old client implementations will be have
// interface shims.
// After the old client is removed, provider tests can be rewritten using
// GoMock, at which point rawProvider is replaced with the new server.
type rawProvider struct {
	newServer
}

type newServer interface {
	FindImage(string, string, []lxd.RemoteServer, bool, environs.StatusCallbackFunc) (lxd.SourcedImage, error)
	GetServer() (server *lxdapi.Server, ETag string, err error)
	GetConnectionInfo() (info *lxdclient.ConnectionInfo, err error)
	UpdateServerConfig(map[string]string) error
	UpdateContainerConfig(string, map[string]string) error
	GetCertificate(fingerprint string) (certificate *lxdapi.Certificate, ETag string, err error)
	DeleteCertificate(fingerprint string) (err error)
	CreateClientCertificate(certificate *lxd.Certificate) error
	LocalBridgeName() string
	AliveContainers(prefix string) ([]lxd.Container, error)
	ContainerAddresses(name string) ([]network.Address, error)
	RemoveContainer(name string) error
	RemoveContainers(names []string) error
	FilterContainers(prefix string, statuses ...string) ([]lxd.Container, error)
	CreateContainerFromSpec(spec lxd.ContainerSpec) (*lxd.Container, error)
	WriteContainer(*lxd.Container) error
	CreateProfileWithConfig(string, map[string]string) error
	HasProfile(string) (bool, error)
	StorageSupported() bool
	GetStoragePool(name string) (pool *lxdapi.StoragePool, ETag string, err error)
	GetStoragePools() (pools []lxdapi.StoragePool, err error)
	CreatePool(name, driver string, attrs map[string]string) error
	GetStoragePoolVolume(pool string, volType string, name string) (*lxdapi.StorageVolume, string, error)
	GetStoragePoolVolumes(pool string) (volumes []lxdapi.StorageVolume, err error)
	CreateVolume(pool, name string, config map[string]string) error
	UpdateStoragePoolVolume(pool string, volType string, name string, volume lxdapi.StorageVolumePut, ETag string) error
	DeleteStoragePoolVolume(pool string, volType string, name string) (err error)
}

func newRawProvider(spec environs.CloudSpec, local bool) (*rawProvider, error) {
	if local {
		prov, err := newLocalRawProvider()
		return prov, errors.Trace(err)
	}
	clientCert, serverCert, ok := getCertificates(spec)
	if !ok {
		return nil, errors.NotValidf("credentials")
	}
	prov, err := lxd.NewRemoteServer(lxd.RemoteServer{
		Host: spec.Endpoint,
		ConnectionArgs: lxdclient.ConnectionArgs{
			TLSServerCert: serverCert,
			TLSClientCert: string(clientCert.CertPEM),
			TLSClientKey:  string(clientCert.KeyPEM),
		},
	})
	return &rawProvider{newServer: prov}, errors.Trace(err)
}

func newLocalRawProvider() (*rawProvider, error) {
	config := jujulxdclient.Config{Remote: jujulxdclient.Local}
	raw, err := newRawProviderFromConfig(config)
	return raw, errors.Trace(err)
}

func newRawProviderFromConfig(config jujulxdclient.Config) (*rawProvider, error) {
	client, err := jujulxdclient.Connect(config, true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &rawProvider{
		newServer: client,
	}, nil
}

func getCertificates(spec environs.CloudSpec) (client *lxd.Certificate, server string, ok bool) {
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
	clientCert := &lxd.Certificate{
		Name:    "juju",
		CertPEM: []byte(clientCertPEM),
		KeyPEM:  []byte(clientKeyPEM),
	}
	return clientCert, serverCertPEM, true
}
