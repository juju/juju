// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/tools/lxdclient"
)

const (
	credAttrServerCert = "server-cert"
	credAttrClientCert = "client-cert"
	credAttrClientKey  = "client-key"
)

// environProviderCredentials implements environs.ProviderCredentials.
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
func (p environProviderCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	certPEM, keyPEM, err := p.readOrGenerateCert()
	if err != nil {
		return nil, errors.Trace(err)
	}
	certCredential := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		credAttrClientCert: string(certPEM),
		credAttrClientKey:  string(keyPEM),
	})
	return &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{"default": certCredential},
	}, nil
}

func (p environProviderCredentials) readOrGenerateCert() (certPEM, keyPEM []byte, _ error) {
	// First look in the Juju XDG_DATA dir. This allows the user
	// to explicitly override the certificates used by the lxc
	// client if they wish.
	jujuLXDDir := osenv.JujuXDGDataHomePath("lxd")
	certPEM, keyPEM, err := readCert(jujuLXDDir)
	if err == nil {
		return certPEM, keyPEM, nil
	} else if !os.IsNotExist(err) {
		return nil, nil, errors.Trace(err)
	}

	// Next we look in the LXD config dir, in case the user has
	// a client certificate/key pair for use with the "lxc" client
	// application.
	lxdConfigDir := filepath.Join(utils.Home(), ".config", "lxc")
	certPEM, keyPEM, err = readCert(lxdConfigDir)
	if err == nil {
		return certPEM, keyPEM, nil
	} else if !os.IsNotExist(err) {
		return nil, nil, errors.Trace(err)
	}

	// No certs were found, so generate one and cache it in the
	// Juju XDG_DATA dir. We cache the certificate so that we
	// avoid uploading a new certificate each time we bootstrap.
	certPEM, keyPEM, err = p.generateMemCert(true)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if err := writeCert(jujuLXDDir, certPEM, keyPEM); err != nil {
		return nil, nil, errors.Trace(err)
	}
	return certPEM, keyPEM, nil
}

func readCert(dir string) (certPEM, keyPEM []byte, _ error) {
	clientCertPath := filepath.Join(dir, "client.crt")
	clientKeyPath := filepath.Join(dir, "client.key")
	certPEM, err := ioutil.ReadFile(clientCertPath)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err = ioutil.ReadFile(clientKeyPath)
	if err != nil {
		return nil, nil, err
	}
	return certPEM, keyPEM, nil
}

func writeCert(dir string, certPEM, keyPEM []byte) error {
	clientCertPath := filepath.Join(dir, "client.crt")
	clientKeyPath := filepath.Join(dir, "client.key")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.Trace(err)
	}
	if err := ioutil.WriteFile(clientCertPath, certPEM, 0600); err != nil {
		return err
	}
	if err := ioutil.WriteFile(clientKeyPath, keyPEM, 0600); err != nil {
		return err
	}
	return nil
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (p environProviderCredentials) FinalizeCredential(ctx environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	// Credential detection yields a partial certificate containing just
	// the client certificate and key. We check if we have a partial
	// credential, and fill in the server certificate if we can.

	if args.Credential.AuthType() != cloud.CertificateAuthType {
		return &args.Credential, nil
	}

	credAttrs := args.Credential.Attributes()
	if credAttrs[credAttrServerCert] != "" {
		// The credential is fully formed, so we assume the client
		// certificate is uploaded to the server already.
		return &args.Credential, nil
	}
	certPEM := credAttrs[credAttrClientCert]
	keyPEM := credAttrs[credAttrClientKey]
	if certPEM == "" {
		return nil, errors.NotValidf("missing or empty %q attribute", credAttrClientCert)
	}
	if keyPEM == "" {
		return nil, errors.NotValidf("missing or empty %q attribute", credAttrClientKey)
	}

	isLocalEndpoint, err := p.isLocalEndpoint(args.CloudEndpoint)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !isLocalEndpoint {
		// The endpoint is not local, so we cannot generate a
		// certificate credential.
		//
		// TODO(axw) we could look in the $HOME/.config/lxc/servercerts
		// directory for a server certificate. We would need to read
		// $HOME/.config/lxc/config.yml to identify the remote by its
		// endpoint.
		//
		// We may later want to add an "interactive" auth-type
		// that users can select when adding credentials to
		// go through the interactive server certificate fingerprint
		// verification flow.
		return &args.Credential, nil
	}
	raw, err := p.newLocalRawProvider()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Upload the certificate to the server if necessary.
	clientCert := lxdclient.Cert{
		Name:    "juju",
		CertPEM: []byte(certPEM),
		KeyPEM:  []byte(keyPEM),
	}
	fingerprint, err := clientCert.Fingerprint()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, err := raw.CertByFingerprint(fingerprint); errors.IsNotFound(err) {
		if addCertErr := raw.AddCert(clientCert); addCertErr != nil {
			// There is no specific error code returned when
			// attempting to add a certificate that already
			// exists in the database. We can just check
			// again to see if another process added the
			// certificate concurrently with us checking the
			// first time.
			if _, err := raw.CertByFingerprint(fingerprint); errors.IsNotFound(err) {
				// The cert still isn't there, so report the AddCert error.
				return nil, errors.Annotatef(
					addCertErr, "adding certificate %q", clientCert.Name,
				)
			} else if err != nil {
				return nil, errors.Annotate(err, "querying certificates")
			}
			// The certificate is there now, which implies
			// there was a concurrent AddCert by another
			// process. Carry on.
		}
	} else if err != nil {
		return nil, errors.Annotate(err, "querying certificates")
	}

	// Store the server's certificate in the credential.
	serverState, err := raw.ServerStatus()
	if err != nil {
		return nil, errors.Annotate(err, "getting server status")
	}
	credAttrs[credAttrServerCert] = serverState.Environment.Certificate
	out := cloud.NewCredential(cloud.CertificateAuthType, credAttrs)
	out.Label = args.Credential.Label
	return &out, nil
}

func (p environProviderCredentials) isLocalEndpoint(endpoint string) (bool, error) {
	if endpoint == "" {
		// No endpoint specified, so assume we're local. This
		// will happen, for example, when destroying a 2.0
		// LXD controller.
		return true, nil
	}
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
