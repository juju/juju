// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"

	"launchpad.net/juju-core/agent"
	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils"
)

// Default data directory.
// Tests can override this where needed, so they don't need to mess with global
// system state.
var DataDir = "/var/lib/juju"

// NewMachineConfig sets up a basic machine configuration, for a non-bootstrap
// node.  You'll still need to supply more information, but this takes care of
// the fixed entries and the ones that are always needed.
func NewMachineConfig(machineID, machineNonce string,
	stateInfo *state.Info, apiInfo *api.Info,
	disableSSLHostnameVerification bool) *cloudinit.MachineConfig {
	return &cloudinit.MachineConfig{
		// Fixed entries.
		DataDir: DataDir,

		// Parameter entries.
		MachineId:    machineID,
		MachineNonce: machineNonce,
		StateInfo:    stateInfo,
		APIInfo:      apiInfo,
		DisableSSLHostnameVerification: disableSSLHostnameVerification,
	}
}

// NewBootstrapMachineConfig sets up a basic machine configuration for a
// bootstrap node.  You'll still need to supply more information, but this
// takes care of the fixed entries and the ones that are always needed.
// stateInfoURL is the storage URL for the environment's state file.
func NewBootstrapMachineConfig(machineID, stateInfoURL string,
	disableSSLHostnameVerification bool) *cloudinit.MachineConfig {
	// For a bootstrap instance, FinishMachineConfig will provide the
	// state.Info and the api.Info.
	mcfg := NewMachineConfig(machineID, state.BootstrapNonce, nil, nil,
		disableSSLHostnameVerification)
	mcfg.StateServer = true
	mcfg.StateInfoURL = stateInfoURL
	return mcfg
}

// FinishMachineConfig sets fields on a MachineConfig that can be determined by
// inspecting a plain config.Config and the machine constraints at the last
// moment before bootstrapping. It assumes that the supplied Config comes from
// an environment that has passed through all the validation checks in the
// Bootstrap func, and that has set an agent-version (via finding the tools to,
// use for bootstrap, or otherwise).
// TODO(fwereade) This function is not meant to be "good" in any serious way:
// it is better that this functionality be collected in one place here than
// that it be spread out across 3 or 4 providers, but this is its only
// redeeming feature.
func FinishMachineConfig(mcfg *cloudinit.MachineConfig, cfg *config.Config, cons constraints.Value) (err error) {
	defer utils.ErrorContextf(&err, "cannot complete machine configuration")

	// Everything needs the environment's authorized keys.
	authKeys := cfg.AuthorizedKeys()
	if authKeys == "" {
		return fmt.Errorf("environment configuration has no authorized-keys")
	}
	mcfg.AuthorizedKeys = authKeys
	if mcfg.AgentEnvironment == nil {
		mcfg.AgentEnvironment = make(map[string]string)
	}
	mcfg.AgentEnvironment[agent.ProviderType] = cfg.Type()
	mcfg.AgentEnvironment[agent.ContainerType] = string(mcfg.MachineContainerType)
	if !mcfg.StateServer {
		return nil
	}

	// These settings are only appropriate at bootstrap time. At the
	// moment, the only state server is the bootstrap node, but this
	// will probably change.
	if mcfg.APIInfo != nil || mcfg.StateInfo != nil {
		return fmt.Errorf("machine configuration already has api/state info")
	}
	caCert, hasCACert := cfg.CACert()
	if !hasCACert {
		return fmt.Errorf("environment configuration has no ca-cert")
	}
	password := cfg.AdminSecret()
	if password == "" {
		return fmt.Errorf("environment configuration has no admin-secret")
	}
	passwordHash := utils.PasswordHash(password)
	mcfg.APIInfo = &api.Info{Password: passwordHash, CACert: caCert}
	mcfg.StateInfo = &state.Info{Password: passwordHash, CACert: caCert}
	mcfg.StatePort = cfg.StatePort()
	mcfg.APIPort = cfg.APIPort()
	mcfg.Constraints = cons
	if mcfg.Config, err = BootstrapConfig(cfg); err != nil {
		return err
	}

	// These really are directly relevant to running a state server.
	cert, key, err := cfg.GenerateStateServerCertAndKey()
	if err != nil {
		return fmt.Errorf("cannot generate state server certificate: %v", err)
	}
	mcfg.StateServerCert = cert
	mcfg.StateServerKey = key
	return nil
}

// ComposeUserData puts together a binary (gzipped) blob of user data.
// The additionalScripts are additional command lines that you need cloudinit
// to run on the instance.  Use with care.
func ComposeUserData(cfg *cloudinit.MachineConfig, additionalScripts ...string) ([]byte, error) {
	cloudcfg := coreCloudinit.New()
	for _, script := range additionalScripts {
		cloudcfg.AddRunCmd(script)
	}
	cloudcfg, err := cloudinit.Configure(cfg, cloudcfg)
	if err != nil {
		return nil, err
	}
	data, err := cloudcfg.Render()
	logger.Tracef("Generated cloud init:\n%s", string(data))
	if err != nil {
		return nil, err
	}
	return utils.Gzip(data), nil
}
