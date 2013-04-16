package environs

import (
	"fmt"
	"time"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils"
)

// FinishMachineConfig sets fields on a MachineConfig that can be determined by
// inspecting a plain config.Config.
func FinishMachineConfig(mcfg *cloudinit.MachineConfig, cfg *config.Config, cons constraints.Value) (err error) {
	defer utils.ErrorContextf(&err, "cannot complete machine configuration")

	// Everything needs the environment's authorized keys.
	authKeys := cfg.AuthorizedKeys()
	if authKeys == "" {
		return fmt.Errorf("environment configuration missing authorized-keys")
	}
	mcfg.AuthorizedKeys = authKeys
	if !mcfg.StateServer {
		return nil
	}

	// These settings are only appropriate at bootstrap time. At the
	// moment, the only state server is the bootstrap node, but this
	// will probably change.
	caCert, hasCACert := cfg.CACert()
	if !hasCACert {
		return fmt.Errorf("environment configuration missing CA certificate")
	}
	password := cfg.AdminSecret()
	if password == "" {
		return fmt.Errorf("environment configuration missing admin-secret")
	}
	passwordHash := utils.PasswordHash(password)
	mcfg.APIInfo = &api.Info{Password: passwordHash, CACert: caCert}
	mcfg.StateInfo = &state.Info{Password: passwordHash, CACert: caCert}
	mcfg.Constraints = cons
	if mcfg.Config, err = BootstrapConfig(cfg); err != nil {
		return err
	}

	// These really are directly relevant to running a state server.
	caKey, hasCAKey := cfg.CAPrivateKey()
	if !hasCAKey {
		return fmt.Errorf("environment configuration missing CA private key")
	}
	cert, key, err := cert.NewServer(cfg.Name(), caCert, caKey, time.Now().UTC().AddDate(10, 0, 0))
	if err != nil {
		return fmt.Errorf("cannot generate bootstrap certificate: %v", err)
	}
	mcfg.StateServerCert = cert
	mcfg.StateServerKey = key
	return nil
}
