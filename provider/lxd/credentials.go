// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"net"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/tools/lxdclient"
)

const (
	credAttrServerCert = "server-cert"
	credAttrClientCert = "client-cert"
	credAttrClientKey  = "client-key"
)

type environProviderCredentials struct {
	generateMemCert     func(bool) ([]byte, []byte, error)
	newLocalRawProvider func() (*rawProvider, error)
	lookupHost          func(string) ([]string, error)
	interfaceAddrs      func() ([]net.Addr, error)
}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.CertificateAuthType: {{
			credAttrServerCert,
			cloud.CredentialAttr{
				Description: "The LXD server certificate, PEM-encoded.",
			},
		}, {
			credAttrClientCert,
			cloud.CredentialAttr{
				Description: "The LXD client certificate, PEM-encoded.",
			},
		}, {
			credAttrClientKey,
			cloud.CredentialAttr{
				Description: "The LXD client key, PEM-encoded.",
			},
		}},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	return cloud.NewEmptyCloudCredential(), nil
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (p environProviderCredentials) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	if args.Credential.AuthType() != cloud.EmptyAuthType {
		return &args.Credential, nil
	}

	isLocalEndpoint, err := p.isLocalEndpoint(args.CloudEndpoint)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !isLocalEndpoint {
		// The endpoint is not local, so we cannot generate a
		// certificate credential.
		return &args.Credential, nil
	}
	raw, err := p.newLocalRawProvider()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Generate a certificate pair, upload to the server, and then create a
	// new "certificate" credential with them and the server's certificate.
	//
	// We must upload here to cater for the esoteric case of automatic
	// credential generation in "juju add-model". We *also* upload during
	// bootstrap, in case the user does a "rebootstrap" with the *same*
	// credentials (e.g. juju restore-backup).
	certPEM, keyPEM, err := p.generateMemCert(true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := raw.AddCert(lxdclient.Cert{
		Name:    "juju",
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}); err != nil {
		return nil, errors.Annotate(err, "adding certificate")
	}
	serverState, err := raw.ServerStatus()
	if err != nil {
		return nil, errors.Annotate(err, "getting server status")
	}

	out := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		credAttrServerCert: serverState.Environment.Certificate,
		credAttrClientCert: string(certPEM),
		credAttrClientKey:  string(keyPEM),
	})
	out.Label = args.Credential.Label
	return &out, nil
}

func (p environProviderCredentials) isLocalEndpoint(endpoint string) (bool, error) {
	endpointAddrs, err := p.lookupHost(endpoint)
	if err != nil {
		return false, errors.Trace(err)
	}
	localAddrs, err := p.interfaceAddrs()
	if err != nil {
		return false, errors.Trace(err)
	}
	return addrsContainsAny(localAddrs, endpointAddrs), nil
}

func addrsContainsAny(haystack []net.Addr, needles []string) bool {
	for _, needle := range needles {
		if addrsContains(haystack, needle) {
			return true
		}
	}
	return false
}

func addrsContains(haystack []net.Addr, needle string) bool {
	ip := net.ParseIP(needle)
	if ip == nil {
		return false
	}
	for _, addr := range haystack {
		if addr, ok := addr.(*net.IPNet); ok && addr.IP.Equal(ip) {
			return true
		}
	}
	return false
}
