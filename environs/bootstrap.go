package environs

import (
	"fmt"
	"launchpad.net/juju-core/cert"
	"time"
)

// Bootstrap bootstraps the given environment.  If the environment does
// not contain a CA certificate, a new certificate and key pair are
// generated, added to the environment configuration, and writeCertAndKey
// will be called to save them.  If writeCertFile is nil, the generated
// certificate and key will be saved to ~/.juju/<environ-name>-cert.pem
// and ~/.juju/<environ-name>-private-key.pem.
//
// If uploadTools is true, the current version of the juju tools will be
// uploaded, as documented in Environ.Bootstrap.
func Bootstrap(environ Environ, uploadTools bool) error {
	cfg := environ.Config()
	caCert, hasCACert := cfg.CACert()
	caKey, hasCAKey := cfg.CAPrivateKey()
	if !hasCACert {
		return fmt.Errorf("environment configuration missing CA certificate")
	}
	if !hasCAKey {
		return fmt.Errorf("environment configuration missing CA private key")
	}
	// Generate a new key pair and certificate for
	// the newly bootstrapped instance.
	cert, key, err := cert.NewServer(environ.Name(), caCert, caKey, time.Now().UTC().AddDate(10, 0, 0))
	if err != nil {
		return fmt.Errorf("cannot generate bootstrap certificate: %v", err)
	}
	return environ.Bootstrap(uploadTools, cert, key)
}
