// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"path"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"

	"github.com/juju/juju/agent"
	coreCloudinit "github.com/juju/juju/cloudinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/version"
)

// DataDir is the default data directory.
// Tests can override this where needed, so they don't need to mess with global
// system state.
var DataDir = agent.DefaultDataDir

// logDir returns a filesystem path to the location where applications
// may create a folder containing logs
var logDir = paths.MustSucceed(paths.LogDir(version.Current.Series))

// CloudInitOutputLog is the default cloud-init-output.log file path.
var CloudInitOutputLog = path.Join(logDir, "cloud-init-output.log")

// NewMachineConfig sets up a basic machine configuration, for a non-bootstrap
// node.  You'll still need to supply more information, but this takes care of
// the fixed entries and the ones that are always needed.
func NewMachineConfig(
	machineID, machineNonce string, networks []string,
	mongoInfo *authentication.MongoInfo, apiInfo *api.Info,
) *cloudinit.MachineConfig {
	mcfg := &cloudinit.MachineConfig{
		// Fixed entries.
		DataDir:                 DataDir,
		LogDir:                  agent.DefaultLogDir,
		Jobs:                    []params.MachineJob{params.JobHostUnits},
		CloudInitOutputLog:      CloudInitOutputLog,
		MachineAgentServiceName: "jujud-" + names.NewMachineTag(machineID).String(),

		// Parameter entries.
		MachineId:    machineID,
		MachineNonce: machineNonce,
		Networks:     networks,
		MongoInfo:    mongoInfo,
		APIInfo:      apiInfo,
	}
	return mcfg
}

// NewBootstrapMachineConfig sets up a basic machine configuration for a
// bootstrap node.  You'll still need to supply more information, but this
// takes care of the fixed entries and the ones that are always needed.
func NewBootstrapMachineConfig(privateSystemSSHKey string) *cloudinit.MachineConfig {
	// For a bootstrap instance, FinishMachineConfig will provide the
	// state.Info and the api.Info. The machine id must *always* be "0".
	mcfg := NewMachineConfig("0", agent.BootstrapNonce, nil, nil, nil)
	mcfg.Bootstrap = true
	mcfg.SystemPrivateSSHKey = privateSystemSSHKey
	mcfg.Jobs = []params.MachineJob{params.JobManageEnviron, params.JobHostUnits}
	return mcfg
}

// PopulateMachineConfig is called both from the FinishMachineConfig below,
// which does have access to the environment config, and from the container
// provisioners, which don't have access to the environment config. Everything
// that is needed to provision a container needs to be returned to the
// provisioner in the ContainerConfig structure. Those values are then used to
// call this function.
func PopulateMachineConfig(mcfg *cloudinit.MachineConfig,
	providerType, authorizedKeys string,
	sslHostnameVerification bool,
	proxySettings, aptProxySettings proxy.Settings,
	preferIPv6 bool,
) error {
	if authorizedKeys == "" {
		return fmt.Errorf("environment configuration has no authorized-keys")
	}
	mcfg.AuthorizedKeys = authorizedKeys
	if mcfg.AgentEnvironment == nil {
		mcfg.AgentEnvironment = make(map[string]string)
	}
	mcfg.AgentEnvironment[agent.ProviderType] = providerType
	mcfg.AgentEnvironment[agent.ContainerType] = string(mcfg.MachineContainerType)
	mcfg.DisableSSLHostnameVerification = !sslHostnameVerification
	mcfg.ProxySettings = proxySettings
	mcfg.AptProxySettings = aptProxySettings
	mcfg.PreferIPv6 = preferIPv6
	return nil
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
	defer errors.Maskf(&err, "cannot complete machine configuration")

	if err := PopulateMachineConfig(
		mcfg,
		cfg.Type(),
		cfg.AuthorizedKeys(),
		cfg.SSLHostnameVerification(),
		cfg.ProxySettings(),
		cfg.AptProxySettings(),
		cfg.PreferIPv6(),
	); err != nil {
		return err
	}

	// The following settings are only appropriate at bootstrap time. At the
	// moment, the only state server is the bootstrap node, but this
	// will probably change.
	if !mcfg.Bootstrap {
		return nil
	}
	if mcfg.APIInfo != nil || mcfg.MongoInfo != nil {
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
	passwordHash := utils.UserPasswordHash(password, utils.CompatSalt)
	mcfg.APIInfo = &api.Info{Password: passwordHash, CACert: caCert}
	mcfg.MongoInfo = &authentication.MongoInfo{Password: passwordHash, Info: mongo.Info{CACert: caCert}}

	// These really are directly relevant to running a state server.
	cert, key, err := cfg.GenerateStateServerCertAndKey()
	if err != nil {
		return errors.Annotate(err, "cannot generate state server certificate")
	}

	srvInfo := params.StateServingInfo{
		StatePort:      cfg.StatePort(),
		APIPort:        cfg.APIPort(),
		Cert:           string(cert),
		PrivateKey:     string(key),
		SystemIdentity: mcfg.SystemPrivateSSHKey,
	}
	mcfg.StateServingInfo = &srvInfo
	mcfg.Constraints = cons
	if mcfg.Config, err = BootstrapConfig(cfg); err != nil {
		return err
	}

	return nil
}

func configureCloudinit(mcfg *cloudinit.MachineConfig, cloudcfg *coreCloudinit.Config) error {
	// When bootstrapping, we only want to apt-get update/upgrade
	// and setup the SSH keys. The rest we leave to cloudinit/sshinit.
	if mcfg.Bootstrap {
		return cloudinit.ConfigureBasic(mcfg, cloudcfg)
	}
	return cloudinit.Configure(mcfg, cloudcfg)
}

// ComposeUserData fills out the provided cloudinit configuration structure
// so it is suitable for initialising a machine with the given configuration,
// and then renders it and returns it as a binary (gzipped) blob of user data.
//
// If the provided cloudcfg is nil, a new one will be created internally.
func ComposeUserData(mcfg *cloudinit.MachineConfig, cloudcfg *coreCloudinit.Config) ([]byte, error) {
	if cloudcfg == nil {
		cloudcfg = coreCloudinit.New()
	}
	if err := configureCloudinit(mcfg, cloudcfg); err != nil {
		return nil, err
	}
	data, err := cloudcfg.Render()
	logger.Tracef("Generated cloud init:\n%s", string(data))
	if err != nil {
		return nil, err
	}
	return utils.Gzip(data), nil
}
