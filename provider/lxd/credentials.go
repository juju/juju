// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/provider/lxd/lxdnames"
)

const (
	credAttrServerCert    = "server-cert"
	credAttrClientCert    = "client-cert"
	credAttrClientKey     = "client-key"
	credAttrTrustPassword = "trust-password"
)

// CertificateReadWriter groups methods that is required to read and write
// certificates at a given path.
//go:generate mockgen -package lxd -destination credentials_mock_test.go github.com/juju/juju/provider/lxd CertificateReadWriter,CertificateGenerator,NetLookup
type CertificateReadWriter interface {
	// Read takes a path and returns both a cert and key PEM.
	// Returns an error if there was an issue reading the certs.
	Read(path string) (certPEM, keyPEM []byte, err error)

	// Write takes a path and cert, key PEM and stores them.
	// Returns an error if there was an issue writing the certs.
	Write(path string, certPEM, keyPEM []byte) error
}

// CertificateGenerator groups methods for generating a new certificate
type CertificateGenerator interface {
	// Generate creates client or server certificate and key pair,
	// returning them as byte arrays in memory.
	Generate(client bool, addHosts bool) (certPEM, keyPEM []byte, err error)
}

// NetLookup groups methods for looking up hosts and interface addresses.
type NetLookup interface {
	// LookupHost looks up the given host using the local resolver.
	// It returns a slice of that host's addresses.
	LookupHost(string) ([]string, error)

	// InterfaceAddrs returns a list of the system's unicast interface
	// addresses.
	InterfaceAddrs() ([]net.Addr, error)
}

// environProviderCredentials implements environs.ProviderCredentials.
type environProviderCredentials struct {
	certReadWriter  CertificateReadWriter
	certGenerator   CertificateGenerator
	lookup          NetLookup
	serverFactory   ServerFactory
	lxcConfigReader LXCConfigReader
}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.CertificateAuthType: {
			{
				Name: credAttrServerCert,
				CredentialAttr: cloud.CredentialAttr{
					Description:    "the path to the PEM-encoded LXD server certificate file",
					ExpandFilePath: true,
					Hidden:         true,
				},
			}, {
				Name: credAttrClientCert,
				CredentialAttr: cloud.CredentialAttr{
					Description:    "the path to the PEM-encoded LXD client certificate file",
					ExpandFilePath: true,
					Hidden:         true,
				},
			}, {
				Name: credAttrClientKey,
				CredentialAttr: cloud.CredentialAttr{
					Description:    "the path to the PEM-encoded LXD client key file",
					ExpandFilePath: true,
					Hidden:         true,
				},
			},
		},
		cloud.InteractiveAuthType: {
			{
				Name: credAttrTrustPassword,
				CredentialAttr: cloud.CredentialAttr{
					Description: "the LXD server trust password",
					Hidden:      true,
				},
			},
		},
	}
}

// RegisterCredentials is part of the environs.ProviderCredentialsRegister interface.
func (p environProviderCredentials) RegisterCredentials(cld cloud.Cloud) (map[string]*cloud.CloudCredential, error) {
	// only register credentials if the operator is attempting to access "lxd"
	// or "localhost"
	cloudName := cld.Name
	if cloudName != lxdnames.DefaultCloud && cloudName != lxdnames.DefaultCloudAltName {
		return make(map[string]*cloud.CloudCredential), nil
	}

	nopLogf := func(msg string, args ...interface{}) {}
	certPEM, keyPEM, err := p.readOrGenerateCert(nopLogf)
	if err != nil {
		return nil, errors.Trace(err)
	}

	localCertCredential, err := p.detectLocalCredentials(certPEM, keyPEM)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return map[string]*cloud.CloudCredential{
		cloudName: {
			DefaultCredential: cloudName,
			AuthCredentials: map[string]cloud.Credential{
				cloudName: *localCertCredential,
			},
		},
	}, nil
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (p environProviderCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	nopLogf := func(msg string, args ...interface{}) {}
	certPEM, keyPEM, err := p.readOrGenerateCert(nopLogf)
	if err != nil {
		return nil, errors.Trace(err)
	}

	remoteCertCredentials, err := p.detectRemoteCredentials(certPEM, keyPEM)
	if err != nil {
		logger.Errorf("unable to detect remote LXC credentials: %s", err)
	}

	localCertCredentials, err := p.detectLocalCredentials(certPEM, keyPEM)
	if err != nil {
		logger.Errorf("unable to detect local LXC credentials: %s", err)
	}

	authCredentials := make(map[string]cloud.Credential)
	for k, v := range remoteCertCredentials {
		authCredentials[k] = v
	}
	if localCertCredentials != nil {
		authCredentials["localhost"] = *localCertCredentials
	}
	return &cloud.CloudCredential{
		AuthCredentials: authCredentials,
	}, nil
}

// detectLocalCredentials will use the local server to read and finalize the
// cloud credentials.
func (p environProviderCredentials) detectLocalCredentials(certPEM, keyPEM []byte) (*cloud.Credential, error) {
	svr, err := p.serverFactory.LocalServer()
	if err != nil {
		return nil, errors.NewNotFound(err, "failed to connect to local LXD")
	}

	label := fmt.Sprintf("LXD credential %q", lxdnames.DefaultCloud)
	certCredential, err := p.finalizeLocalCredential(
		ioutil.Discard, svr, string(certPEM), string(keyPEM), label,
	)
	return certCredential, errors.Trace(err)
}

// detectRemoteCredentials will attempt to gather all the potential existing
// remote lxc configurations found in `$HOME/.config/lxc/.config` file.
// Any setups found in the configuration will then be returned as a credential
// that can be automatically loaded into juju.
func (p environProviderCredentials) detectRemoteCredentials(certPEM, keyPEM []byte) (map[string]cloud.Credential, error) {
	configDir := filepath.Join(utils.Home(), ".config", "lxc")
	configPath := filepath.Join(configDir, "config.yml")
	config, err := p.lxcConfigReader.ReadConfig(configPath)
	if err != nil {
		return nil, errors.Trace(err)
	}

	credentials := make(map[string]cloud.Credential)
	for name, remote := range config.Remotes {
		if remote.Protocol == lxdnames.ProviderType {
			certPath := filepath.Join(configDir, "servercerts", fmt.Sprintf("%s.crt", name))
			serverCert, err := p.lxcConfigReader.ReadCert(certPath)
			if err != nil {
				logger.Errorf("unable to read certificate from %s with error %s", certPath, err)
				continue
			}
			credential := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
				credAttrClientCert: string(certPEM),
				credAttrClientKey:  string(keyPEM),
				credAttrServerCert: string(serverCert),
			})
			credential.Label = fmt.Sprintf("LXD credential %q", name)
			credentials[name] = credential
		}
	}
	return credentials, nil
}

func (p environProviderCredentials) readOrGenerateCert(logf func(string, ...interface{})) (certPEM, keyPEM []byte, _ error) {
	// First look in the Juju XDG_DATA dir. This allows the user
	// to explicitly override the certificates used by the lxc
	// client if they wish.
	jujuLXDDir := osenv.JujuXDGDataHomePath("lxd")
	certPEM, keyPEM, err := p.certReadWriter.Read(jujuLXDDir)
	if err == nil {
		logf("Loaded client cert/key from %q", jujuLXDDir)
		return certPEM, keyPEM, nil
	} else if !os.IsNotExist(errors.Cause(err)) {
		return nil, nil, errors.Trace(err)
	}

	// Next we look in the LXD config dir, in case the user has
	// a client certificate/key pair for use with the "lxc" client
	// application.
	lxdConfigDir := filepath.Join(utils.Home(), ".config", "lxc")
	certPEM, keyPEM, err = p.certReadWriter.Read(lxdConfigDir)
	if err == nil {
		logf("Loaded client cert/key from %q", lxdConfigDir)
		return certPEM, keyPEM, nil
	} else if !os.IsNotExist(errors.Cause(err)) {
		return nil, nil, errors.Trace(err)
	}

	// No certs were found, so generate one and cache it in the
	// Juju XDG_DATA dir. We cache the certificate so that we
	// avoid uploading a new certificate each time we bootstrap.
	certPEM, keyPEM, err = p.certGenerator.Generate(true, true)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if err := p.certReadWriter.Write(jujuLXDDir, certPEM, keyPEM); err != nil {
		return nil, nil, errors.Trace(err)
	}
	logf("Generating client cert/key in %q", jujuLXDDir)
	return certPEM, keyPEM, nil
}

// ShouldFinalizeCredential is part of the environs.RequestFinalizeCredential
// interface.
// This is an optional interface to check if the server certificate has not
// been filled in.
func (p environProviderCredentials) ShouldFinalizeCredential(cred cloud.Credential) bool {
	// The credential is fully formed, so we assume the client
	// certificate is uploaded to the server already.
	credAttrs := cred.Attributes()
	_, ok := credAttrs[credAttrServerCert]
	return !ok
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (p environProviderCredentials) FinalizeCredential(
	ctx environs.FinalizeCredentialContext,
	args environs.FinalizeCredentialParams,
) (*cloud.Credential, error) {
	switch authType := args.Credential.AuthType(); authType {
	case cloud.InteractiveAuthType:
		credAttrs := args.Credential.Attributes()
		// We don't care if the password is empty, just that it exists. Empty
		// passwords can be valid ones...
		if _, ok := credAttrs[credAttrTrustPassword]; ok {
			// check to see if the client cert, keys exist, if they do not,
			// generate them for the user.
			if _, ok := getClientCertificates(args.Credential); !ok {
				stderr := ctx.GetStderr()
				nopLogf := func(s string, args ...interface{}) {
					fmt.Fprintf(stderr, s+"\n", args...)
				}
				clientCert, clientKey, err := p.readOrGenerateCert(nopLogf)
				if err != nil {
					return nil, err
				}

				credAttrs[credAttrClientCert] = string(clientCert)
				credAttrs[credAttrClientKey] = string(clientKey)

				credential := cloud.NewCredential(cloud.CertificateAuthType, credAttrs)
				credential.Label = args.Credential.Label

				args.Credential = credential
			}
		}
		fallthrough
	case cloud.CertificateAuthType:
		return p.finalizeCredential(ctx, args)
	default:
		return &args.Credential, nil
	}
}

func (p environProviderCredentials) finalizeCredential(
	ctx environs.FinalizeCredentialContext,
	args environs.FinalizeCredentialParams,
) (*cloud.Credential, error) {
	// Credential detection yields a partial certificate containing just
	// the client certificate and key. We check if we have a partial
	// credential, and fill in the server certificate if we can.
	stderr := ctx.GetStderr()
	credAttrs := args.Credential.Attributes()
	// The credential is fully formed, so we assume the client
	// certificate is uploaded to the server already.
	if v, ok := credAttrs[credAttrServerCert]; ok && v != "" {
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

	// If the end point is local, set up the local server and automate the local
	// certificate credentials.
	if isLocalEndpoint {
		svr, err := p.serverFactory.LocalServer()
		if err != nil {
			return nil, errors.Trace(err)
		}
		cred, err := p.finalizeLocalCredential(
			stderr, svr, certPEM, keyPEM,
			args.Credential.Label,
		)
		return cred, errors.Trace(err)
	}

	// We're not local, so setup the remote server and automate the remote
	// certificate credentials.
	return p.finalizeRemoteCredential(
		stderr,
		args.CloudEndpoint,
		args.Credential,
	)
}

func (p environProviderCredentials) finalizeRemoteCredential(
	output io.Writer,
	endpoint string,
	credentials cloud.Credential,
) (*cloud.Credential, error) {
	clientCert, ok := getClientCertificates(credentials)
	if !ok {
		return nil, errors.NotFoundf("client credentials")
	}
	if err := clientCert.Validate(); err != nil {
		return nil, errors.Annotate(err, "client credentials")
	}

	credAttrs := credentials.Attributes()
	trustPassword, ok := credAttrs[credAttrTrustPassword]
	if !ok {
		return nil, errors.NotValidf("missing %q attribute", credAttrTrustPassword)
	}

	insecureCreds := cloud.NewCredential(cloud.CertificateAuthType, credAttrs)
	server, err := p.serverFactory.InsecureRemoteServer(environs.CloudSpec{
		Endpoint:   endpoint,
		Credential: &insecureCreds,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	clientX509Cert, err := clientCert.X509()
	if err != nil {
		return nil, errors.Annotate(err, "client credentials")
	}

	// check to see if the cert already exists
	fingerprint, err := clientCert.Fingerprint()
	if err != nil {
		return nil, errors.Trace(err)
	}

	cert, _, err := server.GetCertificate(fingerprint)
	if err != nil || cert == nil {
		if err := server.CreateCertificate(api.CertificatesPost{
			CertificatePut: api.CertificatePut{
				Name: credentials.Label,
				Type: "client",
			},
			Certificate: base64.StdEncoding.EncodeToString(clientX509Cert.Raw),
			Password:    trustPassword,
		}); err != nil {
			return nil, errors.Trace(err)
		}
		fmt.Fprintln(output, "Uploaded certificate to LXD server.")
	} else {
		fmt.Fprintln(output, "Reusing certificate from LXD server.")
	}

	lxdServer, _, err := server.GetServer()
	if err != nil {
		return nil, errors.Trace(err)
	}
	lxdServerCert := lxdServer.Environment.Certificate

	// request to make sure that we can actually query correctly in a secure
	// manor.
	attributes := make(map[string]string)
	for k, v := range credAttrs {
		if k == credAttrTrustPassword {
			continue
		}
		attributes[k] = v
	}
	attributes[credAttrServerCert] = lxdServerCert

	secureCreds := cloud.NewCredential(cloud.CertificateAuthType, attributes)
	server, err = p.serverFactory.RemoteServer(environs.CloudSpec{
		Endpoint:   endpoint,
		Credential: &secureCreds,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Store the server's certificate in the credential.
	out := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		credAttrClientCert: string(clientCert.CertPEM),
		credAttrClientKey:  string(clientCert.KeyPEM),
		credAttrServerCert: server.ServerCertificate(),
	})
	out.Label = credentials.Label
	return &out, nil
}

func (p environProviderCredentials) finalizeLocalCredential(
	output io.Writer,
	svr Server,
	certPEM, keyPEM, label string,
) (*cloud.Credential, error) {

	// Upload the certificate to the server if necessary.
	clientCert := &lxd.Certificate{
		Name:    "juju",
		CertPEM: []byte(certPEM),
		KeyPEM:  []byte(keyPEM),
	}
	fingerprint, err := clientCert.Fingerprint()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, _, err := svr.GetCertificate(fingerprint); lxd.IsLXDNotFound(err) {
		if addCertErr := svr.CreateClientCertificate(clientCert); addCertErr != nil {
			// There is no specific error code returned when
			// attempting to add a certificate that already
			// exists in the database. We can just check
			// again to see if another process added the
			// certificate concurrently with us checking the
			// first time.
			if _, _, err := svr.GetCertificate(fingerprint); lxd.IsLXDNotFound(err) {
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
		fmt.Fprintln(output, "Uploaded certificate to LXD server.")

	} else if err != nil {
		return nil, errors.Annotate(err, "querying certificates")
	}

	// Store the server's certificate in the credential.
	out := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		credAttrClientCert: certPEM,
		credAttrClientKey:  keyPEM,
		credAttrServerCert: svr.ServerCertificate(),
	})
	out.Label = label
	return &out, nil
}

func (p environProviderCredentials) isLocalEndpoint(endpoint string) (bool, error) {
	if endpoint == "" {
		// No endpoint specified, so assume we're local. This
		// will happen, for example, when destroying a 2.0
		// LXD controller.
		return true, nil
	}
	endpointURL, err := endpointURL(endpoint)
	if err != nil {
		return false, errors.Trace(err)
	}
	host, _, err := net.SplitHostPort(endpointURL.Host)
	if err != nil {
		host = endpointURL.Host
	}
	endpointAddrs, err := p.lookup.LookupHost(host)
	if err != nil {
		return false, errors.Trace(err)
	}
	localAddrs, err := p.lookup.InterfaceAddrs()
	if err != nil {
		return false, errors.Trace(err)
	}
	return addrsContainsAny(localAddrs, endpointAddrs), nil
}

// certificateReadWriter is the default implementation for reading and writing
// certificates to disk.
type certificateReadWriter struct{}

func (certificateReadWriter) Read(path string) ([]byte, []byte, error) {
	clientCertPath := filepath.Join(path, "client.crt")
	clientKeyPath := filepath.Join(path, "client.key")
	certPEM, err := ioutil.ReadFile(clientCertPath)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	keyPEM, err := ioutil.ReadFile(clientKeyPath)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return certPEM, keyPEM, nil
}

func (certificateReadWriter) Write(path string, certPEM, keyPEM []byte) error {
	clientCertPath := filepath.Join(path, "client.crt")
	clientKeyPath := filepath.Join(path, "client.key")
	if err := os.MkdirAll(path, 0700); err != nil {
		return errors.Trace(err)
	}
	if err := ioutil.WriteFile(clientCertPath, certPEM, 0600); err != nil {
		return errors.Trace(err)
	}
	if err := ioutil.WriteFile(clientKeyPath, keyPEM, 0600); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// certificateGenerator is the default implementation for generating a
// certificate if it's not found on disk.
type certificateGenerator struct{}

func (certificateGenerator) Generate(client bool, addHosts bool) (certPEM, keyPEM []byte, err error) {
	return shared.GenerateMemCert(client, addHosts)
}

type netLookup struct{}

func (netLookup) LookupHost(host string) ([]string, error) {
	return net.LookupHost(host)
}

func (netLookup) InterfaceAddrs() ([]net.Addr, error) {
	return net.InterfaceAddrs()
}

func endpointURL(endpoint string) (*url.URL, error) {
	remoteURL, err := url.Parse(endpoint)
	if err != nil || remoteURL.Scheme == "" {
		remoteURL = &url.URL{
			Scheme: "https",
			Host:   endpoint,
		}
	} else {
		// If the user specifies an endpoint, it must be either
		// host:port, or https://host:port. We do not support
		// unix:// endpoints at present.
		if remoteURL.Scheme != "https" {
			return nil, errors.Errorf(
				"invalid URL %q: only HTTPS is supported",
				endpoint,
			)
		}
	}
	return remoteURL, nil
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

func getCertificates(credentials cloud.Credential) (client *lxd.Certificate, server string, ok bool) {
	clientCert, ok := getClientCertificates(credentials)
	if !ok {
		return nil, "", false
	}
	credAttrs := credentials.Attributes()
	serverCertPEM, ok := credAttrs[credAttrServerCert]
	if !ok {
		return nil, "", false
	}
	return clientCert, serverCertPEM, true
}

func getClientCertificates(credentials cloud.Credential) (client *lxd.Certificate, ok bool) {
	credAttrs := credentials.Attributes()
	clientCertPEM, ok := credAttrs[credAttrClientCert]
	if !ok {
		return nil, false
	}
	clientKeyPEM, ok := credAttrs[credAttrClientKey]
	if !ok {
		return nil, false
	}
	clientCert := &lxd.Certificate{
		Name:    "juju",
		CertPEM: []byte(clientCertPEM),
		KeyPEM:  []byte(clientKeyPEM),
	}
	return clientCert, true
}
