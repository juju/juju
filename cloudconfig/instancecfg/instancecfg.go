// Copyright 2012, 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package instancecfg

import (
	"fmt"
	"net"
	"path"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/state/multiwatcher"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.cloudconfig.instancecfg")

// InstanceConfig represents initialization information for a new juju instance.
type InstanceConfig struct {
	// Tags is a set of tags to set on the instance, if supported. This
	// should be populated using the InstanceTags method in this package.
	Tags map[string]string

	// Bootstrap specifies whether the new instance is the bootstrap
	// instance. When this is true, StateServingInfo should be set
	// and filled out.
	Bootstrap bool

	// StateServingInfo holds the information for serving the state.
	// This must only be set if the Bootstrap field is true
	// (state servers started subsequently will acquire their serving info
	// from another server)
	StateServingInfo *params.StateServingInfo

	// MongoInfo holds the means for the new instance to communicate with the
	// juju state database. Unless the new instance is running a state server
	// (StateServer is set), there must be at least one state server address supplied.
	// The entity name must match that of the instance being started,
	// or be empty when starting a state server.
	MongoInfo *mongo.MongoInfo

	// APIInfo holds the means for the new instance to communicate with the
	// juju state API. Unless the new instance is running a state server (StateServer is
	// set), there must be at least one state server address supplied.
	// The entity name must match that of the instance being started,
	// or be empty when starting a state server.
	APIInfo *api.Info

	// InstanceId is the instance ID of the instance being initialised.
	// This is required when bootstrapping, and ignored otherwise.
	InstanceId instance.Id

	// HardwareCharacteristics contains the harrdware characteristics of
	// the instance being initialised. This optional, and is only used by
	// the bootstrap agent during state initialisation.
	HardwareCharacteristics *instance.HardwareCharacteristics

	// MachineNonce is set at provisioning/bootstrap time and used to
	// ensure the agent is running on the correct instance.
	MachineNonce string

	// Tools is juju tools to be used on the new instance.
	Tools *coretools.Tools

	// DataDir holds the directory that juju state will be put in the new
	// instance.
	DataDir string

	// LogDir holds the directory that juju logs will be written to.
	LogDir string

	// Jobs holds what machine jobs to run.
	Jobs []multiwatcher.MachineJob

	// CloudInitOutputLog specifies the path to the output log for cloud-init.
	// The directory containing the log file must already exist.
	CloudInitOutputLog string

	// MachineId identifies the new machine.
	MachineId string

	// MachineContainerType specifies the type of container that the instance
	// is.  If the instance is not a container, then the type is "".
	MachineContainerType instance.ContainerType

	// MachineContainerHostname specifies the hostname to be used with the
	// cloud config for the instance. If this is not set, hostname uses the default.
	MachineContainerHostname string

	// Networks holds a list of networks the instances should be on.
	//
	// TODO(dimitern): Drop this in a follow-up in favor or spaces
	// constraints.
	Networks []string

	// AuthorizedKeys specifies the keys that are allowed to
	// connect to the instance (see cloudinit.SSHAddAuthorizedKeys)
	// If no keys are supplied, there can be no ssh access to the node.
	// On a bootstrap instance, that is fatal. On other
	// instances it will mean that the ssh, scp and debug-hooks
	// commands cannot work.
	AuthorizedKeys string

	// AgentEnvironment defines additional configuration variables to set in
	// the instance agent config.
	AgentEnvironment map[string]string

	// WARNING: this is only set if the instance being configured is
	// a state server node.
	//
	// Config holds the initial environment configuration.
	Config *config.Config

	// Constraints holds the initial environment constraints.
	Constraints constraints.Value

	// DisableSSLHostnameVerification can be set to true to tell cloud-init
	// that it shouldn't verify SSL certificates
	DisableSSLHostnameVerification bool

	// Series represents the instance series.
	Series string

	// MachineAgentServiceName is the init service name for the Juju machine agent.
	MachineAgentServiceName string

	// ProxySettings define normal http, https and ftp proxies.
	ProxySettings proxy.Settings

	// AptProxySettings define the http, https and ftp proxy settings to use
	// for apt, which may or may not be the same as the normal ProxySettings.
	AptProxySettings proxy.Settings

	// AptMirror defines an APT mirror location, which, if specified, will
	// override the default APT sources.
	AptMirror string

	// PreferIPv6 mirrors the value of prefer-ipv6 environment setting
	// and when set IPv6 addresses for connecting to the API/state
	// servers will be preferred over IPv4 ones.
	PreferIPv6 bool

	// The type of Simple Stream to download and deploy on this instance.
	ImageStream string

	// CustomImageMetadata is optional custom simplestreams image metadata
	// to store in environment storage at bootstrap time. This is ignored
	// in non-bootstrap instances.
	CustomImageMetadata []*imagemetadata.ImageMetadata

	// EnableOSRefreshUpdate specifies whether Juju will refresh its
	// respective OS's updates list.
	EnableOSRefreshUpdate bool

	// EnableOSUpgrade defines Juju's behavior when provisioning
	// instances. If enabled, the OS will perform any upgrades
	// available as part of its provisioning.
	EnableOSUpgrade bool
}

func (cfg *InstanceConfig) agentInfo() service.AgentInfo {
	return service.NewMachineAgentInfo(
		cfg.MachineId,
		cfg.DataDir,
		cfg.LogDir,
	)
}

func (cfg *InstanceConfig) ToolsDir(renderer shell.Renderer) string {
	return cfg.agentInfo().ToolsDir(renderer)
}

func (cfg *InstanceConfig) InitService(renderer shell.Renderer) (service.Service, error) {
	conf := service.AgentConf(cfg.agentInfo(), renderer)

	name := cfg.MachineAgentServiceName
	svc, err := newService(name, conf, cfg.Series)
	return svc, errors.Trace(err)
}

var newService = func(name string, conf common.Conf, series string) (service.Service, error) {
	return service.NewService(name, conf, series)
}

func (cfg *InstanceConfig) AgentConfig(
	tag names.Tag,
	toolsVersion version.Number,
) (agent.ConfigSetter, error) {
	// TODO for HAState: the stateHostAddrs and apiHostAddrs here assume that
	// if the instance is a stateServer then to use localhost.  This may be
	// sufficient, but needs thought in the new world order.
	var password string
	if cfg.MongoInfo == nil {
		password = cfg.APIInfo.Password
	} else {
		password = cfg.MongoInfo.Password
	}
	configParams := agent.AgentConfigParams{
		DataDir:           cfg.DataDir,
		LogDir:            cfg.LogDir,
		Jobs:              cfg.Jobs,
		Tag:               tag,
		UpgradedToVersion: toolsVersion,
		Password:          password,
		Nonce:             cfg.MachineNonce,
		StateAddresses:    cfg.stateHostAddrs(),
		APIAddresses:      cfg.ApiHostAddrs(),
		CACert:            cfg.MongoInfo.CACert,
		Values:            cfg.AgentEnvironment,
		PreferIPv6:        cfg.PreferIPv6,
		Environment:       cfg.APIInfo.EnvironTag,
	}
	if !cfg.Bootstrap {
		return agent.NewAgentConfig(configParams)
	}
	return agent.NewStateMachineConfig(configParams, *cfg.StateServingInfo)
}

func (cfg *InstanceConfig) JujuTools() string {
	return agenttools.SharedToolsDir(cfg.DataDir, cfg.Tools.Version)
}

func (cfg *InstanceConfig) stateHostAddrs() []string {
	var hosts []string
	if cfg.Bootstrap {
		if cfg.PreferIPv6 {
			hosts = append(hosts, net.JoinHostPort("::1", strconv.Itoa(cfg.StateServingInfo.StatePort)))
		} else {
			hosts = append(hosts, net.JoinHostPort("localhost", strconv.Itoa(cfg.StateServingInfo.StatePort)))
		}
	}
	if cfg.MongoInfo != nil {
		hosts = append(hosts, cfg.MongoInfo.Addrs...)
	}
	return hosts
}

func (cfg *InstanceConfig) ApiHostAddrs() []string {
	var hosts []string
	if cfg.Bootstrap {
		if cfg.PreferIPv6 {
			hosts = append(hosts, net.JoinHostPort("::1", strconv.Itoa(cfg.StateServingInfo.APIPort)))
		} else {
			hosts = append(hosts, net.JoinHostPort("localhost", strconv.Itoa(cfg.StateServingInfo.APIPort)))
		}
	}
	if cfg.APIInfo != nil {
		hosts = append(hosts, cfg.APIInfo.Addrs...)
	}
	return hosts
}

// HasNetworks returns if there are any networks set.
func (cfg *InstanceConfig) HasNetworks() bool {
	return len(cfg.Networks) > 0 || cfg.Constraints.HaveNetworks()
}

type requiresError string

func (e requiresError) Error() string {
	return "invalid machine configuration: missing " + string(e)
}

func (cfg *InstanceConfig) VerifyConfig() (err error) {
	defer errors.DeferredAnnotatef(&err, "invalid machine configuration")
	if !names.IsValidMachine(cfg.MachineId) {
		return errors.New("invalid machine id")
	}
	if cfg.DataDir == "" {
		return errors.New("missing var directory")
	}
	if cfg.LogDir == "" {
		return errors.New("missing log directory")
	}
	if len(cfg.Jobs) == 0 {
		return errors.New("missing machine jobs")
	}
	if cfg.CloudInitOutputLog == "" {
		return errors.New("missing cloud-init output log path")
	}
	if cfg.Tools == nil {
		return errors.New("missing tools")
	}
	if cfg.Tools.URL == "" {
		return errors.New("missing tools URL")
	}
	if cfg.MongoInfo == nil {
		return errors.New("missing state info")
	}
	if len(cfg.MongoInfo.CACert) == 0 {
		return errors.New("missing CA certificate")
	}
	if cfg.APIInfo == nil {
		return errors.New("missing API info")
	}
	if cfg.APIInfo.EnvironTag.Id() == "" {
		return errors.New("missing environment tag")
	}
	if len(cfg.APIInfo.CACert) == 0 {
		return errors.New("missing API CA certificate")
	}
	if cfg.MachineAgentServiceName == "" {
		return errors.New("missing machine agent service name")
	}
	if cfg.Bootstrap {
		if cfg.Config == nil {
			return errors.New("missing environment configuration")
		}
		if cfg.MongoInfo.Tag != nil {
			return errors.New("entity tag must be nil when starting a state server")
		}
		if cfg.APIInfo.Tag != nil {
			return errors.New("entity tag must be nil when starting a state server")
		}
		if cfg.StateServingInfo == nil {
			return errors.New("missing state serving info")
		}
		if len(cfg.StateServingInfo.Cert) == 0 {
			return errors.New("missing state server certificate")
		}
		if len(cfg.StateServingInfo.PrivateKey) == 0 {
			return errors.New("missing state server private key")
		}
		if len(cfg.StateServingInfo.CAPrivateKey) == 0 {
			return errors.New("missing ca cert private key")
		}
		if cfg.StateServingInfo.StatePort == 0 {
			return errors.New("missing state port")
		}
		if cfg.StateServingInfo.APIPort == 0 {
			return errors.New("missing API port")
		}
		if cfg.InstanceId == "" {
			return errors.New("missing instance-id")
		}
	} else {
		if len(cfg.MongoInfo.Addrs) == 0 {
			return errors.New("missing state hosts")
		}
		if cfg.MongoInfo.Tag != names.NewMachineTag(cfg.MachineId) {
			return errors.New("entity tag must match started machine")
		}
		if len(cfg.APIInfo.Addrs) == 0 {
			return errors.New("missing API hosts")
		}
		if cfg.APIInfo.Tag != names.NewMachineTag(cfg.MachineId) {
			return errors.New("entity tag must match started machine")
		}
		if cfg.StateServingInfo != nil {
			return errors.New("state serving info unexpectedly present")
		}
	}
	if cfg.MachineNonce == "" {
		return errors.New("missing machine nonce")
	}
	return nil
}

// logDir returns a filesystem path to the location where applications
// may create a folder containing logs
var logDir = paths.MustSucceed(paths.LogDir(version.Current.Series))

// DefaultBridgeName is the network bridge device name used for LXC and KVM
// containers
const DefaultBridgeName = "juju-br0"

// NewInstanceConfig sets up a basic machine configuration, for a
// non-bootstrap node. You'll still need to supply more information,
// but this takes care of the fixed entries and the ones that are
// always needed.
func NewInstanceConfig(
	machineID,
	machineNonce,
	imageStream,
	series string,
	secureServerConnections bool,
	networks []string,
	mongoInfo *mongo.MongoInfo,
	apiInfo *api.Info,
) (*InstanceConfig, error) {
	dataDir, err := paths.DataDir(series)
	if err != nil {
		return nil, err
	}
	logDir, err := paths.LogDir(series)
	if err != nil {
		return nil, err
	}
	cloudInitOutputLog := path.Join(logDir, "cloud-init-output.log")
	icfg := &InstanceConfig{
		// Fixed entries.
		DataDir:                 dataDir,
		LogDir:                  path.Join(logDir, "juju"),
		Jobs:                    []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		CloudInitOutputLog:      cloudInitOutputLog,
		MachineAgentServiceName: "jujud-" + names.NewMachineTag(machineID).String(),
		Series:                  series,
		Tags:                    map[string]string{},

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
	return icfg, nil
}

// NewBootstrapInstanceConfig sets up a basic machine configuration for a
// bootstrap node.  You'll still need to supply more information, but this
// takes care of the fixed entries and the ones that are always needed.
func NewBootstrapInstanceConfig(cons constraints.Value, series string) (*InstanceConfig, error) {
	// For a bootstrap instance, FinishInstanceConfig will provide the
	// state.Info and the api.Info. The machine id must *always* be "0".
	icfg, err := NewInstanceConfig("0", agent.BootstrapNonce, "", series, true, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	icfg.Bootstrap = true
	icfg.Jobs = []multiwatcher.MachineJob{
		multiwatcher.JobManageEnviron,
		multiwatcher.JobHostUnits,
	}
	icfg.Constraints = cons
	return icfg, nil
}

// PopulateInstanceConfig is called both from the FinishInstanceConfig below,
// which does have access to the environment config, and from the container
// provisioners, which don't have access to the environment config. Everything
// that is needed to provision a container needs to be returned to the
// provisioner in the ContainerConfig structure. Those values are then used to
// call this function.
func PopulateInstanceConfig(icfg *InstanceConfig,
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
	icfg.AuthorizedKeys = authorizedKeys
	if icfg.AgentEnvironment == nil {
		icfg.AgentEnvironment = make(map[string]string)
	}
	icfg.AgentEnvironment[agent.ProviderType] = providerType
	icfg.AgentEnvironment[agent.ContainerType] = string(icfg.MachineContainerType)
	icfg.DisableSSLHostnameVerification = !sslHostnameVerification
	icfg.ProxySettings = proxySettings
	icfg.AptProxySettings = aptProxySettings
	icfg.AptMirror = aptMirror
	icfg.PreferIPv6 = preferIPv6
	icfg.EnableOSRefreshUpdate = enableOSRefreshUpdates
	icfg.EnableOSUpgrade = enableOSUpgrade
	return nil
}

// FinishInstanceConfig sets fields on a InstanceConfig that can be determined by
// inspecting a plain config.Config and the machine constraints at the last
// moment before bootstrapping. It assumes that the supplied Config comes from
// an environment that has passed through all the validation checks in the
// Bootstrap func, and that has set an agent-version (via finding the tools to,
// use for bootstrap, or otherwise).
// TODO(fwereade) This function is not meant to be "good" in any serious way:
// it is better that this functionality be collected in one place here than
// that it be spread out across 3 or 4 providers, but this is its only
// redeeming feature.
func FinishInstanceConfig(icfg *InstanceConfig, cfg *config.Config) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot complete machine configuration")

	if err := PopulateInstanceConfig(
		icfg,
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

	if isStateInstanceConfig(icfg) {
		// Add NUMACTL preference. Needed to work for both bootstrap and high availability
		// Only makes sense for state server
		logger.Debugf("Setting numa ctl preference to %v", cfg.NumaCtlPreference())
		// Unfortunately, AgentEnvironment can only take strings as values
		icfg.AgentEnvironment[agent.NumaCtlPreference] = fmt.Sprintf("%v", cfg.NumaCtlPreference())
	}
	// The following settings are only appropriate at bootstrap time. At the
	// moment, the only state server is the bootstrap node, but this
	// will probably change.
	if !icfg.Bootstrap {
		return nil
	}
	if icfg.APIInfo != nil || icfg.MongoInfo != nil {
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
	icfg.APIInfo = &api.Info{
		Password:   passwordHash,
		CACert:     caCert,
		EnvironTag: names.NewEnvironTag(envUUID),
	}
	icfg.MongoInfo = &mongo.MongoInfo{Password: passwordHash, Info: mongo.Info{CACert: caCert}}

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
	icfg.StateServingInfo = &srvInfo
	if icfg.Config, err = bootstrapConfig(cfg); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// InstanceTags returns the minimum set of tags that should be set on a
// machine instance, if the provider supports them.
func InstanceTags(cfg *config.Config, jobs []multiwatcher.MachineJob) map[string]string {
	uuid, _ := cfg.UUID()
	instanceTags := tags.ResourceTags(names.NewEnvironTag(uuid), cfg)
	if multiwatcher.AnyJobNeedsState(jobs...) {
		instanceTags[tags.JujuStateServer] = "true"
	}
	return instanceTags
}

// bootstrapConfig returns a copy of the supplied configuration with the
// admin-secret and ca-private-key attributes removed. If the resulting
// config is not suitable for bootstrapping an environment, an error is
// returned.
// This function is copied from environs in here so we can avoid an import loop
func bootstrapConfig(cfg *config.Config) (*config.Config, error) {
	m := cfg.AllAttrs()
	// We never want to push admin-secret or the root CA private key to the cloud.
	delete(m, "admin-secret")
	delete(m, "ca-private-key")
	cfg, err := config.New(config.NoDefaults, m)
	if err != nil {
		return nil, err
	}
	if _, ok := cfg.AgentVersion(); !ok {
		return nil, fmt.Errorf("environment configuration has no agent-version")
	}
	return cfg, nil
}

// isStateInstanceConfig determines if given machine configuration
// is for State Server by iterating over machine's jobs.
// If JobManageEnviron is present, this is a state server.
func isStateInstanceConfig(icfg *InstanceConfig) bool {
	for _, aJob := range icfg.Jobs {
		if aJob == multiwatcher.JobManageEnviron {
			return true
		}
	}
	return false
}
