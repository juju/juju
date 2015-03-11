// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"net"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/state/multiwatcher"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

var (
	logger = loggo.GetLogger("juju.environs.cloudinit")
)

// fileSchemePrefix is the prefix for file:// URLs.
const (
	fileSchemePrefix = "file://"

	// NonceFile is written by cloud-init as the last thing it does.
	// The file will contain the machine's nonce. The filename is
	// relative to the Juju data-dir.
	NonceFile = "nonce.txt"
)

// MachineConfig represents initialization information for a new juju machine.
type InstanceConfig struct {
	// Bootstrap specifies whether the new machine is the bootstrap
	// machine. When this is true, StateServingInfo should be set
	// and filled out.
	Bootstrap bool

	// StateServingInfo holds the information for serving the state.
	// This must only be set if the Bootstrap field is true
	// (state servers started subsequently will acquire their serving info
	// from another server)
	StateServingInfo *params.StateServingInfo

	// MongoInfo holds the means for the new instance to communicate with the
	// juju state database. Unless the new machine is running a state server
	// (StateServer is set), there must be at least one state server address supplied.
	// The entity name must match that of the machine being started,
	// or be empty when starting a state server.
	MongoInfo *mongo.MongoInfo

	// APIInfo holds the means for the new instance to communicate with the
	// juju state API. Unless the new machine is running a state server (StateServer is
	// set), there must be at least one state server address supplied.
	// The entity name must match that of the machine being started,
	// or be empty when starting a state server.
	APIInfo *api.Info

	// InstanceId is the instance ID of the machine being initialised.
	// This is required when bootstrapping, and ignored otherwise.
	InstanceId instance.Id

	// HardwareCharacteristics contains the harrdware characteristics of
	// the machine being initialised. This optional, and is only used by
	// the bootstrap agent during state initialisation.
	HardwareCharacteristics *instance.HardwareCharacteristics

	// MachineNonce is set at provisioning/bootstrap time and used to
	// ensure the agent is running on the correct instance.
	MachineNonce string

	// Tools is juju tools to be used on the new machine.
	Tools *coretools.Tools

	// DataDir holds the directory that juju state will be put in the new
	// machine.
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

	// MachineContainerType specifies the type of container that the machine
	// is.  If the machine is not a container, then the type is "".
	MachineContainerType instance.ContainerType

	// Networks holds a list of networks the machine should be on.
	Networks []string

	// AuthorizedKeys specifies the keys that are allowed to
	// connect to the machine (see cloudinit.SSHAddAuthorizedKeys)
	// If no keys are supplied, there can be no ssh access to the node.
	// On a bootstrap machine, that is fatal. On other
	// machines it will mean that the ssh, scp and debug-hooks
	// commands cannot work.
	AuthorizedKeys string

	// AgentEnvironment defines additional configuration variables to set in
	// the machine agent config.
	AgentEnvironment map[string]string

	// WARNING: this is only set if the machine being configured is
	// a state server node.
	//
	// Config holds the initial environment configuration.
	Config *config.Config

	// Constraints holds the initial environment constraints.
	Constraints constraints.Value

	// DisableSSLHostnameVerification can be set to true to tell cloud-init
	// that it shouldn't verify SSL certificates
	DisableSSLHostnameVerification bool

	// Series represents the machine series.
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

	// The type of Simple Stream to download and deploy on this machine.
	ImageStream string

	// CustomImageMetadata is optional custom simplestreams image metadata
	// to store in environment storage at bootstrap time. This is ignored
	// in non-bootstrap machines.
	CustomImageMetadata []*imagemetadata.ImageMetadata

	// EnableOSRefreshUpdate specifies whether Juju will refresh its
	// respective OS's updates list.
	EnableOSRefreshUpdate bool

	// EnableOSUpgrade defines Juju's behavior when provisioning
	// machines. If enabled, the OS will perform any upgrades
	// available as part of its provisioning.
	EnableOSUpgrade bool
}

func (cfg *InstanceConfig) initService() (service.Service, string, error) {
	conf, toolsDir := service.MachineAgentConf(
		cfg.MachineId,
		cfg.DataDir,
		cfg.LogDir,
	)
}

func (cfg *MachineConfig) toolsDir(renderer shell.Renderer) string {
	return cfg.agentInfo().ToolsDir(renderer)
}

func (cfg *MachineConfig) initService(renderer shell.Renderer) (service.Service, error) {
	conf := service.AgentConf(cfg.agentInfo(), renderer)

	name := cfg.MachineAgentServiceName
	initSystem, ok := cfg.initSystem()
	if !ok {
		return nil, errors.New("could not identify init system")
	}
	logger.Debugf("using init system %q for machine agent script", initSystem)
	svc, err := newService(name, conf, initSystem)
	return svc, errors.Trace(err)
}

func (cfg *InstanceConfig) initSystem() (string, bool) {
	return service.VersionInitSystem(cfg.Tools.Version)
}

var newService = func(name string, conf common.Conf, initSystem string) (service.Service, error) {
	return service.NewService(name, conf, initSystem)
}

func (cfg *InstanceConfig) agentConfig(
	tag names.Tag,
	toolsVersion version.Number,
) (agent.ConfigSetter, error) {
	// TODO for HAState: the stateHostAddrs and apiHostAddrs here assume that
	// if the machine is a stateServer then to use localhost.  This may be
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
		APIAddresses:      cfg.apiHostAddrs(),
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

func (cfg *InstanceConfig) jujuTools() string {
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

func (cfg *InstanceConfig) apiHostAddrs() []string {
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

func shquote(p string) string {
	return utils.ShQuote(p)
}

type requiresError string

func (e requiresError) Error() string {
	return "invalid machine configuration: missing " + string(e)
}

func verifyConfig(cfg *InstanceConfig) (err error) {
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
