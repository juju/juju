package environs

import (
	"fmt"
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/constraints"
	"time"
)

// Bootstrap bootstraps the given environment. The supplied constraints are
// used to provision the instance, and are also set within the bootstrapped
// environment. The uploadTools parameter requires that the juju-core source
// code be available within $GOPATH; if that is the case, that code will be
// built locally and made available to the environment.
func Bootstrap(environ Environ, cons constraints.Value, uploadTools bool) error {
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
	return environ.Bootstrap(cons, uploadTools, cert, key)
}
