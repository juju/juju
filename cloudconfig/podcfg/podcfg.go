// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg

import (
	"fmt"
	"net"
	"path"
	"reflect"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/shell"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/service"
	"github.com/juju/juju/state/multiwatcher"
	coretools "github.com/juju/juju/tools"
)

var logger = loggo.GetLogger("juju.cloudconfig.podcfg")

// ControllerPodConfig represents initialization information for a new juju caas unit.
type ControllerPodConfig struct {
	// Tags is a set of tags to set on the instance, if supported. This
	// should be populated using the InstanceTags method in this package.
	Tags map[string]string

	// Bootstrap contains bootstrap-specific configuration. If this is set,
	// Controller must also be set.
	Bootstrap *BootstrapConfig

	// Controller contains controller-specific configuration. If this is
	// set, then the instance will be configured as a controller machine.
	Controller *ControllerConfig

	// APIInfo holds the means for the new instance to communicate with the
	// juju state API. Unless the new instance is running a controller (Controller is
	// set), there must be at least one controller address supplied.
	// The entity name must match that of the instance being started,
	// or be empty when starting a controller.
	APIInfo *api.Info

	// ControllerTag identifies the controller.
	ControllerTag names.ControllerTag

	// MachineNonce is set at provisioning/bootstrap time and used to
	// ensure the agent is running on the correct instance.
	MachineNonce string

	// tools is the list of juju tools used to install the Juju agent
	// on the new instance. Each of the entries in the list must have
	// identical versions and hashes, but may have different URLs.
	tools coretools.List // ???

	// DataDir holds the directory that juju state will be put in the new
	// instance.
	DataDir string

	// LogDir holds the directory that juju logs will be written to.
	LogDir string

	// MetricsSpoolDir represents the spool directory path, where all
	// metrics are stored.
	MetricsSpoolDir string

	// Jobs holds what machine jobs to run.
	Jobs []multiwatcher.MachineJob

	// MachineId identifies the new machine.
	MachineId string

	// AgentEnvironment defines additional configuration variables to set in
	// the instance agent config.
	AgentEnvironment map[string]string

	// Series represents the instance series.
	Series string

	// MachineAgentServiceName is the init service name for the Juju machine agent.
	MachineAgentServiceName string
}

type BootstrapConfig struct {
	instancecfg.BootstrapConfig
}

type ControllerConfig struct {
	instancecfg.ControllerConfig
}

func (cfg *ControllerPodConfig) agentInfo() service.AgentInfo {
	return service.NewMachineAgentInfo(
		cfg.MachineId,
		cfg.DataDir,
		cfg.LogDir,
	)
}

func (cfg *ControllerPodConfig) ToolsDir(renderer shell.Renderer) string {
	return cfg.agentInfo().ToolsDir(renderer)
}

func (cfg *ControllerPodConfig) AgentConfig(
	tag names.Tag,
	toolsVersion version.Number,
) (agent.ConfigSetterWriter, error) {
	var password, cacert string
	if cfg.Controller == nil {
		password = cfg.APIInfo.Password
		cacert = cfg.APIInfo.CACert
	} else {
		password = cfg.Controller.MongoInfo.Password
		cacert = cfg.Controller.MongoInfo.CACert
	}
	configParams := agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir:         cfg.DataDir,
			LogDir:          cfg.LogDir,
			MetricsSpoolDir: cfg.MetricsSpoolDir,
		},
		Jobs:              cfg.Jobs,
		Tag:               tag,
		UpgradedToVersion: toolsVersion,
		Password:          password,
		Nonce:             cfg.MachineNonce,
		APIAddresses:      cfg.APIHostAddrs(),
		CACert:            cacert,
		Values:            cfg.AgentEnvironment,
		Controller:        cfg.ControllerTag,
		Model:             cfg.APIInfo.ModelTag,
	}
	return agent.NewStateMachineConfig(configParams, cfg.Bootstrap.StateServingInfo)
}

// JujuTools returns the directory where Juju tools are stored.
func (cfg *ControllerPodConfig) JujuTools() string {
	return agenttools.SharedToolsDir(cfg.DataDir, cfg.AgentVersion())
}

func (cfg *ControllerPodConfig) stateHostAddrs() []string {
	var hosts []string
	if cfg.Bootstrap != nil {
		hosts = append(hosts, net.JoinHostPort(
			"localhost", strconv.Itoa(cfg.Bootstrap.StateServingInfo.StatePort)),
		)
	}
	if cfg.Controller != nil {
		hosts = append(hosts, cfg.Controller.MongoInfo.Addrs...)
	}
	return hosts
}

func (cfg *ControllerPodConfig) APIHostAddrs() []string {
	var hosts []string
	if cfg.Bootstrap != nil {
		hosts = append(hosts, net.JoinHostPort(
			"localhost", strconv.Itoa(cfg.Bootstrap.StateServingInfo.APIPort)),
		)
	}
	if cfg.APIInfo != nil {
		hosts = append(hosts, cfg.APIInfo.Addrs...)
	}
	return hosts
}

func (cfg *ControllerPodConfig) APIHosts() []string {
	var hosts []string
	if cfg.Bootstrap != nil {
		hosts = append(hosts, "localhost")
	}
	if cfg.APIInfo != nil {
		for _, addr := range cfg.APIInfo.Addrs {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				logger.Errorf("Can't split API address %q to host:port - %q", host, err)
				continue
			}
			hosts = append(hosts, host)
		}
	}
	return hosts
}

// AgentVersion returns the version of the Juju agent that will be configured
// on the instance.
func (cfg *ControllerPodConfig) AgentVersion() version.Binary {
	if len(cfg.tools) == 0 {
		return version.Binary{}
	}
	return cfg.tools[0].Version
}

func (cfg *ControllerPodConfig) SetTools(toolsList coretools.List) error {
	if len(toolsList) == 0 {
		return errors.New("need at least 1 agent binary")
	}
	var tools *coretools.Tools
	for _, listed := range toolsList {
		if listed == nil {
			return errors.New("nil entry in agent binaries list")
		}
		info := *listed
		info.URL = ""
		if tools == nil {
			tools = &info
			continue
		}
		if !reflect.DeepEqual(info, *tools) {
			return errors.Errorf("agent binary info mismatch (%v, %v)", *tools, info)
		}
	}
	cfg.tools = copyToolsList(toolsList)
	return nil
}

func copyToolsList(in coretools.List) coretools.List {
	out := make(coretools.List, len(in))
	for i, tools := range in {
		copied := *tools
		out[i] = &copied
	}
	return out
}

type requiresError string

func (e requiresError) Error() string {
	return "invalid machine configuration: missing " + string(e)
}

// VerifyConfig verifies that the ControllerPodConfig is valid.
func (cfg *ControllerPodConfig) VerifyConfig() (err error) {
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
	if cfg.MetricsSpoolDir == "" {
		return errors.New("missing metrics spool directory")
	}
	if len(cfg.Jobs) == 0 {
		return errors.New("missing machine jobs")
	}
	if cfg.tools == nil {
		// SetTools() has never been called successfully.
		return errors.New("missing agent binaries")
	}
	// We don't need to check cfg.toolsURLs since SetTools() does.
	if cfg.APIInfo == nil {
		return errors.New("missing API info")
	}
	if cfg.APIInfo.ModelTag.Id() == "" {
		return errors.New("missing model tag")
	}
	if len(cfg.APIInfo.CACert) == 0 {
		return errors.New("missing API CA certificate")
	}
	if cfg.MachineAgentServiceName == "" {
		return errors.New("missing machine agent service name")
	}
	if cfg.MachineNonce == "" {
		return errors.New("missing machine nonce")
	}
	if cfg.Controller != nil {
		if err := cfg.verifyControllerConfig(); err != nil {
			return errors.Trace(err)
		}
	}
	if cfg.Bootstrap != nil {
		if err := cfg.verifyBootstrapConfig(); err != nil {
			return errors.Trace(err)
		}
	} else {
		if cfg.APIInfo.Tag != names.NewMachineTag(cfg.MachineId) {
			return errors.New("API entity tag must match started machine")
		}
		if len(cfg.APIInfo.Addrs) == 0 {
			return errors.New("missing API hosts")
		}
	}
	return nil
}

func (cfg *ControllerPodConfig) verifyBootstrapConfig() (err error) {
	defer errors.DeferredAnnotatef(&err, "invalid bootstrap configuration")
	if cfg.Controller == nil {
		return errors.New("bootstrap config supplied without controller config")
	}
	if err := cfg.Bootstrap.VerifyConfig(); err != nil {
		return errors.Trace(err)
	}
	if cfg.APIInfo.Tag != nil || cfg.Controller.MongoInfo.Tag != nil {
		return errors.New("entity tag must be nil when bootstrapping")
	}
	return nil
}

func (cfg *ControllerPodConfig) verifyControllerConfig() (err error) {
	defer errors.DeferredAnnotatef(&err, "invalid controller configuration")
	if err := cfg.Controller.VerifyConfig(); err != nil {
		return errors.Trace(err)
	}
	if cfg.Bootstrap == nil {
		if len(cfg.Controller.MongoInfo.Addrs) == 0 {
			return errors.New("missing state hosts")
		}
		if cfg.Controller.MongoInfo.Tag != names.NewMachineTag(cfg.MachineId) {
			return errors.New("entity tag must match started machine")
		}
	}
	return nil
}

// VerifyConfig verifies that the BootstrapConfig is valid.
func (cfg *BootstrapConfig) VerifyConfig() (err error) {
	if cfg.ControllerModelConfig == nil {
		return errors.New("missing model configuration")
	}
	if len(cfg.StateServingInfo.Cert) == 0 {
		return errors.New("missing controller certificate")
	}
	if len(cfg.StateServingInfo.PrivateKey) == 0 {
		return errors.New("missing controller private key")
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
	return nil
}

// VerifyConfig verifies that the ControllerConfig is valid.
func (cfg *ControllerConfig) VerifyConfig() error {
	if cfg.MongoInfo == nil {
		return errors.New("missing state info")
	}
	if len(cfg.MongoInfo.CACert) == 0 {
		return errors.New("missing CA certificate")
	}
	return nil
}

// NewControllerPodConfig sets up a basic machine configuration, for a
// non-bootstrap node. You'll still need to supply more information,
// but this takes care of the fixed entries and the ones that are
// always needed.
func NewControllerPodConfig(
	controllerTag names.ControllerTag,
	machineID,
	machineNonce,
	imageStream,
	series string,
	apiInfo *api.Info,
) (*ControllerPodConfig, error) {
	dataDir, err := paths.DataDir(series)
	if err != nil {
		return nil, err
	}
	logDir, err := paths.LogDir(series)
	if err != nil {
		return nil, err
	}
	metricsSpoolDir, err := paths.MetricsSpoolDir(series)
	if err != nil {
		return nil, err
	}
	pcfg := &ControllerPodConfig{
		// Fixed entries.
		DataDir:         dataDir,
		LogDir:          path.Join(logDir, "juju"),
		MetricsSpoolDir: metricsSpoolDir,
		// CAAS only has JobManageModel.
		Jobs: []multiwatcher.MachineJob{multiwatcher.JobManageModel},
		MachineAgentServiceName: "jujud-" + names.NewMachineTag(machineID).String(),
		Series:                  series,
		Tags:                    map[string]string{},

		// Parameter entries.
		ControllerTag: controllerTag,
		MachineId:     machineID,
		MachineNonce:  machineNonce,
		APIInfo:       apiInfo,
	}
	return pcfg, nil
}

// NewBootstrapControllerPodConfig sets up a basic machine configuration for a
// bootstrap node.  You'll still need to supply more information, but this
// takes care of the fixed entries and the ones that are always needed.
func NewBootstrapControllerPodConfig(
	config controller.Config,
	series string,
) (*ControllerPodConfig, error) {
	// For a bootstrap instance, the caller must provide the state.Info
	// and the api.Info. The machine id must *always* be "0".
	pcfg, err := NewControllerPodConfig(names.NewControllerTag(config.ControllerUUID()), "0", agent.BootstrapNonce, "", series, nil)
	if err != nil {
		return nil, err
	}
	pcfg.Controller = &ControllerConfig{}
	pcfg.Controller.Config = make(map[string]interface{})
	for k, v := range config {
		pcfg.Controller.Config[k] = v
	}
	pcfg.Bootstrap = &BootstrapConfig{
		instancecfg.BootstrapConfig{
			StateInitializationParams: instancecfg.StateInitializationParams{},
		},
	}
	pcfg.Jobs = []multiwatcher.MachineJob{
		multiwatcher.JobManageModel,
	}
	return pcfg, nil
}

// PopulateControllerPodConfig is called both from the FinishControllerPodConfig below,
// which does have access to the environment config, and from the container
// provisioners, which don't have access to the environment config. Everything
// that is needed to provision a container needs to be returned to the
// provisioner in the ContainerConfig structure. Those values are then used to
// call this function.
func PopulateControllerPodConfig(pcfg *ControllerPodConfig, providerType string) error {
	if pcfg.AgentEnvironment == nil {
		pcfg.AgentEnvironment = make(map[string]string)
	}
	pcfg.AgentEnvironment[agent.ProviderType] = providerType
	return nil
}

// FinishControllerPodConfig sets fields on a ControllerPodConfig that can be determined by
// inspecting a plain config.Config and the machine constraints at the last
// moment before creating the user-data. It assumes that the supplied Config comes
// from an environment that has passed through all the validation checks in the
// Bootstrap func, and that has set an agent-version (via finding the tools to,
// use for bootstrap, or otherwise).
// TODO(fwereade) This function is not meant to be "good" in any serious way:
// it is better that this functionality be collected in one place here than
// that it be spread out across 3 or 4 providers, but this is its only
// redeeming feature.
func FinishControllerPodConfig(pcfg *ControllerPodConfig, cfg *config.Config) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot complete machine configuration")
	if err := PopulateControllerPodConfig(pcfg, cfg.Type()); err != nil {
		return errors.Trace(err)
	}
	if pcfg.Controller != nil {
		// Add NUMACTL preference. Needed to work for both bootstrap and high availability
		// Only makes sense for controller
		logger.Debugf("Setting numa ctl preference to %v", pcfg.Controller.Config.NUMACtlPreference())
		// Unfortunately, AgentEnvironment can only take strings as values
		pcfg.AgentEnvironment[agent.NUMACtlPreference] = fmt.Sprintf("%v", pcfg.Controller.Config.NUMACtlPreference())
	}
	return nil
}
