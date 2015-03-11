// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"path"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	coreCloudinit "github.com/juju/juju/cloudinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/version"
)

// DataDir is the default data directory.  Tests can override this
// where needed, so they don't need to mess with global system state.
var DataDir = agent.DefaultDataDir

// logDir returns a filesystem path to the location where applications
// may create a folder containing logs
var logDir = paths.MustSucceed(paths.LogDir(version.Current.Series))

// NewMachineConfig sets up a basic machine configuration, for a
// non-bootstrap node. You'll still need to supply more information,
// but this takes care of the fixed entries and the ones that are
// always needed.
func NewMachineConfig(
	machineID,
	machineNonce,
	imageStream,
	series string,
	secureServerConnections bool,
	networks []string,
	mongoInfo *mongo.MongoInfo,
	apiInfo *api.Info,
) (*cloudinit.InstanceConfig, error) {
	dataDir, err := paths.DataDir(series)
	if err != nil {
		return nil, err
	}
	logDir, err := paths.LogDir(series)
	if err != nil {
		return nil, err
	}
	cloudInitOutputLog := path.Join(logDir, "cloud-init-output.log")
	mcfg := &cloudinit.InstanceConfig{
		// Fixed entries.
		DataDir:                 dataDir,
		LogDir:                  path.Join(logDir, "juju"),
		Jobs:                    []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		CloudInitOutputLog:      cloudInitOutputLog,
		MachineAgentServiceName: "jujud-" + names.NewMachineTag(machineID).String(),
		Series:                  series,

		// Parameter entries.
		MachineId:    machineID,
		MachineNonce: machineNonce,
		Networks:     networks,
		MongoInfo:    mongoInfo,
		APIInfo:      apiInfo,
		ImageStream:  imageStream,
		AgentEnvironment: map[string]string{
			agent.AllowsSecureConnection: strconv.FormatBool(secureServerConnections),
		},
	}
	return mcfg, nil
}

// NewBootstrapMachineConfig sets up a basic machine configuration for a
// bootstrap node.  You'll still need to supply more information, but this
// takes care of the fixed entries and the ones that are always needed.
func NewBootstrapMachineConfig(cons constraints.Value, series string) (*cloudinit.InstanceConfig, error) {
	// For a bootstrap instance, FinishMachineConfig will provide the
	// state.Info and the api.Info. The machine id must *always* be "0".
	mcfg, err := NewMachineConfig("0", agent.BootstrapNonce, "", series, true, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	mcfg.Bootstrap = true
	mcfg.Jobs = []multiwatcher.MachineJob{
		multiwatcher.JobManageEnviron,
		multiwatcher.JobHostUnits,
	}
	mcfg.Constraints = cons
	return mcfg, nil
}

// PopulateMachineConfig is called both from the FinishMachineConfig below,
// which does have access to the environment config, and from the container
// provisioners, which don't have access to the environment config. Everything
// that is needed to provision a container needs to be returned to the
// provisioner in the ContainerConfig structure. Those values are then used to
// call this function.
func PopulateMachineConfig(mcfg *cloudinit.InstanceConfig,
	providerType, authorizedKeys string,
	sslHostnameVerification bool,
	proxySettings, aptProxySettings proxy.Settings,
	aptMirror string,
	preferIPv6 bool,
	enableOSRefreshUpdates bool,
	enableOSUpgrade bool,
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
	mcfg.AptMirror = aptMirror
	mcfg.PreferIPv6 = preferIPv6
	mcfg.EnableOSRefreshUpdate = enableOSRefreshUpdates
	mcfg.EnableOSUpgrade = enableOSUpgrade
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
func FinishMachineConfig(mcfg *cloudinit.InstanceConfig, cfg *config.Config) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot complete machine configuration")

	if err := PopulateMachineConfig(
		mcfg,
		cfg.Type(),
		cfg.AuthorizedKeys(),
		cfg.SSLHostnameVerification(),
		cfg.ProxySettings(),
		cfg.AptProxySettings(),
		cfg.AptMirror(),
		cfg.PreferIPv6(),
		cfg.EnableOSRefreshUpdate(),
		cfg.EnableOSUpgrade(),
	); err != nil {
		return errors.Trace(err)
	}

	if isStateMachineConfig(mcfg) {
		// Add NUMACTL preference. Needed to work for both bootstrap and high availability
		// Only makes sense for state server
		logger.Debugf("Setting numa ctl preference to %v", cfg.NumaCtlPreference())
		// Unfortunately, AgentEnvironment can only take strings as values
		mcfg.AgentEnvironment[agent.NumaCtlPreference] = fmt.Sprintf("%v", cfg.NumaCtlPreference())
	}
	// The following settings are only appropriate at bootstrap time. At the
	// moment, the only state server is the bootstrap node, but this
	// will probably change.
	if !mcfg.Bootstrap {
		return nil
	}
	if mcfg.APIInfo != nil || mcfg.MongoInfo != nil {
		return errors.New("machine configuration already has api/state info")
	}
	caCert, hasCACert := cfg.CACert()
	if !hasCACert {
		return errors.New("environment configuration has no ca-cert")
	}
	password := cfg.AdminSecret()
	if password == "" {
		return errors.New("environment configuration has no admin-secret")
	}
	passwordHash := utils.UserPasswordHash(password, utils.CompatSalt)
	envUUID, uuidSet := cfg.UUID()
	if !uuidSet {
		return errors.New("config missing environment uuid")
	}
	mcfg.APIInfo = &api.Info{
		Password:   passwordHash,
		CACert:     caCert,
		EnvironTag: names.NewEnvironTag(envUUID),
	}
	mcfg.MongoInfo = &mongo.MongoInfo{Password: passwordHash, Info: mongo.Info{CACert: caCert}}

	// These really are directly relevant to running a state server.
	// Initially, generate a state server certificate with no host IP
	// addresses in the SAN field. Once the state server is up and the
	// NIC addresses become known, the certificate can be regenerated.
	cert, key, err := cfg.GenerateStateServerCertAndKey(nil)
	if err != nil {
		return errors.Annotate(err, "cannot generate state server certificate")
	}
	caPrivateKey, hasCAPrivateKey := cfg.CAPrivateKey()
	if !hasCAPrivateKey {
		return errors.New("environment configuration has no ca-private-key")
	}
	srvInfo := params.StateServingInfo{
		StatePort:    cfg.StatePort(),
		APIPort:      cfg.APIPort(),
		Cert:         string(cert),
		PrivateKey:   string(key),
		CAPrivateKey: caPrivateKey,
	}
	mcfg.StateServingInfo = &srvInfo
	if mcfg.Config, err = BootstrapConfig(cfg); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// isStateMachineConfig determines if given machine configuration
// is for State Server by iterating over machine's jobs.
// If JobManageEnviron is present, this is a state server.
func isStateMachineConfig(mcfg *cloudinit.InstanceConfig) bool {
	for _, aJob := range mcfg.Jobs {
		if aJob == multiwatcher.JobManageEnviron {
			return true
		}
	}
	return false
}

func configureCloudinit(mcfg *cloudinit.InstanceConfig, cloudcfg *coreCloudinit.Config) (cloudinit.UserdataConfig, error) {
	// When bootstrapping, we only want to apt-get update/upgrade
	// and setup the SSH keys. The rest we leave to cloudinit/sshinit.
	udata, err := cloudinit.NewUserdataConfig(mcfg, cloudcfg)
	if err != nil {
		return nil, err
	}
	if mcfg.Bootstrap {
		err = udata.ConfigureBasic()
		if err != nil {
			return nil, err
		}
		return udata, nil
	}
	err = udata.Configure()
	if err != nil {
		return nil, err
	}
	return udata, nil
}

// ComposeUserData fills out the provided cloudinit configuration structure
// so it is suitable for initialising a machine with the given configuration,
// and then renders it and returns it as a binary (gzipped) blob of user data.
//
// If the provided cloudcfg is nil, a new one will be created internally.
func ComposeUserData(mcfg *cloudinit.InstanceConfig, cloudcfg *coreCloudinit.Config) ([]byte, error) {
	if cloudcfg == nil {
		cfg, err := coreCloudinit.New(mcfg.Series)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cloudcfg = cfg
	}
	udata, err := configureCloudinit(mcfg, cloudcfg)
	if err != nil {
		return nil, err
	}
	data, err := udata.Render()
	logger.Tracef("Generated cloud init:\n%s", string(data))
	if err != nil {
		return nil, err
	}
	return utils.Gzip(data), nil
}
