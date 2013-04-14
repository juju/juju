package environs

import (
	"fmt"
	"launchpad.net/juju-core/constraints"
)

// Bootstrap bootstraps the given environment. The supplied constraints are
// used to provision the instance, and are also set within the bootstrapped
// environment.
func Bootstrap(environ Environ, cons constraints.Value) error {
	cfg := environ.Config()
	if secret := cfg.AdminSecret(); secret == "" {
		return fmt.Errorf("environment configuration missing admin-secret")
	}
	if _, hasCACert := cfg.CACert(); !hasCACert {
		return fmt.Errorf("environment configuration missing CA certificate")
	}
	if _, hasCAKey := cfg.CAPrivateKey(); !hasCAKey {
		return fmt.Errorf("environment configuration missing CA private key")
	}
	return environ.Bootstrap(cons)
}
