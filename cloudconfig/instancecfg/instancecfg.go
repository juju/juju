// Copyright 2012, 2013, 2015, 2016 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package instancecfg

import (
	"encoding/json"
	"fmt"
	"net"
	"path"
	"reflect"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/shell"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/controller"
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
)

var logger = loggo.GetLogger("juju.cloudconfig.instancecfg")

// InstanceConfig represents initialization information for a new juju instance.
type InstanceConfig struct {
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
	tools coretools.List

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

	// The type of Simple Stream to download and deploy on this instance.
	ImageStream string

	// EnableOSRefreshUpdate specifies whether Juju will refresh its
	// respective OS's updates list.
	EnableOSRefreshUpdate bool

	// EnableOSUpgrade defines Juju's behavior when provisioning
	// instances. If enabled, the OS will perform any upgrades
	// available as part of its provisioning.
	EnableOSUpgrade bool
}

// ControllerConfig represents controller-specific initialization information
// for a new juju instance. This is only relevant for controller machines.
type ControllerConfig struct {
	// MongoInfo holds the means for the new instance to communicate with the
	// juju state database. Unless the new instance is running a controller
	// (Controller is set), there must be at least one controller address supplied.
	// The entity name must match that of the instance being started,
	// or be empty when starting a controller.
	MongoInfo *mongo.MongoInfo

	// Config contains controller config attributes.
	Config controller.Config

	// The public key used to sign Juju simplestreams image metadata.
	PublicImageSigningKey string
}

// BootstrapConfig represents bootstrap-specific initialization information
// for a new juju instance. This is only relevant for the bootstrap machine.
type BootstrapConfig struct {
	StateInitializationParams

	// GUI is the Juju GUI archive to be installed in the new instance.
	GUI *coretools.GUIArchive

	// Timeout is the amount of time to wait for bootstrap to complete.
	Timeout time.Duration

	// StateServingInfo holds the information for serving the state.
	// This is only specified for bootstrap; controllers started
	// subsequently will acquire their serving info from another
	// server.
	StateServingInfo params.StateServingInfo
}

// StateInitializationParams contains parameters for initializing the
// state database.
//
// This structure will be passed to the bootstrap agent. To do so, the
// Marshal and Unmarshal methods must be used.
type StateInitializationParams struct {
	// ControllerModelConfig holds the initial controller model configuration.
	ControllerModelConfig *config.Config

	// ControllerCloudName is the name of the cloud that Juju will be
	// bootstrapped in.
	ControllerCloudName string

	// ControllerCloud contains the properties of the cloud that Juju will
	// be bootstrapped in.
	ControllerCloud cloud.Cloud

	// ControllerCloudRegion is the name of the cloud region that Juju will be
	// bootstrapped in.
	ControllerCloudRegion string

	// ControllerCloudCredentialName is the name of the cloud credential that
	// Juju will be bootstrapped with.
	ControllerCloudCredentialName string

	// ControllerCloudCredential contains the cloud credential that Juju will
	// be bootstrapped with.
	ControllerCloudCredential *cloud.Credential

	// ControllerConfig is the set of config attributes relevant
	// to a controller.
	ControllerConfig controller.Config

	// ControllerInheritedConfig is a set of config attributes to be shared by all
	// models managed by this controller.
	ControllerInheritedConfig map[string]interface{}

	// RegionInheritedConfig holds region specific configuration attributes to
	// be shared across all models in the same controller on a particular
	// cloud.
	RegionInheritedConfig cloud.RegionConfig

	// HostedModelConfig is a set of config attributes to be overlaid
	// on the controller model config (Config, above) to construct the
	// initial hosted model config.
	HostedModelConfig map[string]interface{}

	// BootstrapMachineInstanceId is the instance ID of the bootstrap
	// machine instance being initialized.
	BootstrapMachineInstanceId instance.Id

	// BootstrapMachineConstraints holds the constraints for the bootstrap
	// machine.
	BootstrapMachineConstraints constraints.Value

	// BootstrapMachineHardwareCharacteristics contains the harrdware
	// characteristics of the bootstrap machine instance being initialized.
	BootstrapMachineHardwareCharacteristics *instance.HardwareCharacteristics

	// ModelConstraints holds the initial model constraints.
	ModelConstraints constraints.Value

	// CustomImageMetadata is optional custom simplestreams image metadata
	// to store in environment storage at bootstrap time. This is ignored
	// in non-bootstrap instances.
	CustomImageMetadata []*imagemetadata.ImageMetadata
}

type stateInitializationParamsInternal struct {
	ControllerConfig                        map[string]interface{}            `yaml:"controller-config"`
	ControllerModelConfig                   map[string]interface{}            `yaml:"controller-model-config"`
	ControllerInheritedConfig               map[string]interface{}            `yaml:"controller-config-defaults,omitempty"`
	RegionInheritedConfig                   cloud.RegionConfig                `yaml:"region-inherited-config,omitempty"`
	HostedModelConfig                       map[string]interface{}            `yaml:"hosted-model-config,omitempty"`
	BootstrapMachineInstanceId              instance.Id                       `yaml:"bootstrap-machine-instance-id"`
	BootstrapMachineConstraints             constraints.Value                 `yaml:"bootstrap-machine-constraints"`
	BootstrapMachineHardwareCharacteristics *instance.HardwareCharacteristics `yaml:"bootstrap-machine-hardware,omitempty"`
	ModelConstraints                        constraints.Value                 `yaml:"model-constraints"`
	CustomImageMetadataJSON                 string                            `yaml:"custom-image-metadata,omitempty"`
	ControllerCloudName                     string                            `yaml:"controller-cloud-name"`
	ControllerCloud                         string                            `yaml:"controller-cloud"`
	ControllerCloudRegion                   string                            `yaml:"controller-cloud-region"`
	ControllerCloudCredentialName           string                            `yaml:"controller-cloud-credential-name,omitempty"`
	ControllerCloudCredential               *cloud.Credential                 `yaml:"controller-cloud-credential,omitempty"`
}

// Marshal marshals StateInitializationParams to an opaque byte array.
func (p *StateInitializationParams) Marshal() ([]byte, error) {
	customImageMetadataJSON, err := json.Marshal(p.CustomImageMetadata)
	if err != nil {
		return nil, errors.Annotate(err, "marshalling custom image metadata")
	}
	controllerCloud, err := cloud.MarshalCloud(p.ControllerCloud)
	if err != nil {
		return nil, errors.Annotate(err, "marshalling cloud definition")
	}
	internal := stateInitializationParamsInternal{
		p.ControllerConfig,
		p.ControllerModelConfig.AllAttrs(),
		p.ControllerInheritedConfig,
		p.RegionInheritedConfig,
		p.HostedModelConfig,
		p.BootstrapMachineInstanceId,
		p.BootstrapMachineConstraints,
		p.BootstrapMachineHardwareCharacteristics,
		p.ModelConstraints,
		string(customImageMetadataJSON),
		p.ControllerCloudName,
		string(controllerCloud),
		p.ControllerCloudRegion,
		p.ControllerCloudCredentialName,
		p.ControllerCloudCredential,
	}
	return yaml.Marshal(&internal)
}

// Unmarshal unmarshals StateInitializationParams from a byte array that
// was generated with StateInitializationParams.Marshal.
func (p *StateInitializationParams) Unmarshal(data []byte) error {
	var internal stateInitializationParamsInternal
	if err := yaml.Unmarshal(data, &internal); err != nil {
		return errors.Annotate(err, "unmarshalling state initialization params")
	}
	var imageMetadata []*imagemetadata.ImageMetadata
	if err := json.Unmarshal([]byte(internal.CustomImageMetadataJSON), &imageMetadata); err != nil {
		return errors.Trace(err)
	}
	cfg, err := config.New(config.NoDefaults, internal.ControllerModelConfig)
	if err != nil {
		return errors.Trace(err)
	}
	controllerCloud, err := cloud.UnmarshalCloud([]byte(internal.ControllerCloud))
	if err != nil {
		return errors.Trace(err)
	}
	*p = StateInitializationParams{
		ControllerConfig:                        internal.ControllerConfig,
		ControllerModelConfig:                   cfg,
		ControllerInheritedConfig:               internal.ControllerInheritedConfig,
		RegionInheritedConfig:                   internal.RegionInheritedConfig,
		HostedModelConfig:                       internal.HostedModelConfig,
		BootstrapMachineInstanceId:              internal.BootstrapMachineInstanceId,
		BootstrapMachineConstraints:             internal.BootstrapMachineConstraints,
		BootstrapMachineHardwareCharacteristics: internal.BootstrapMachineHardwareCharacteristics,
		ModelConstraints:                        internal.ModelConstraints,
		CustomImageMetadata:                     imageMetadata,
		ControllerCloudName:                     internal.ControllerCloudName,
		ControllerCloud:                         controllerCloud,
		ControllerCloudRegion:                   internal.ControllerCloudRegion,
		ControllerCloudCredentialName:           internal.ControllerCloudCredentialName,
		ControllerCloudCredential:               internal.ControllerCloudCredential,
	}
	return nil
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
	// if the instance is a controller then to use localhost.  This may be
	// sufficient, but needs thought in the new world order.
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
		StateAddresses:    cfg.stateHostAddrs(),
		APIAddresses:      cfg.ApiHostAddrs(),
		CACert:            cacert,
		Values:            cfg.AgentEnvironment,
		Controller:        cfg.ControllerTag,
		Model:             cfg.APIInfo.ModelTag,
	}
	if cfg.Bootstrap == nil {
		return agent.NewAgentConfig(configParams)
	}
	return agent.NewStateMachineConfig(configParams, cfg.Bootstrap.StateServingInfo)
}

// JujuTools returns the directory where Juju tools are stored.
func (cfg *InstanceConfig) JujuTools() string {
	return agenttools.SharedToolsDir(cfg.DataDir, cfg.AgentVersion())
}

// GUITools returns the directory where the Juju GUI release is stored.
func (cfg *InstanceConfig) GUITools() string {
	return agenttools.SharedGUIDir(cfg.DataDir)
}

func (cfg *InstanceConfig) stateHostAddrs() []string {
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

func (cfg *InstanceConfig) ApiHostAddrs() []string {
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

// AgentVersion returns the version of the Juju agent that will be configured
// on the instance. The zero value will be returned if there are no tools set.
func (cfg *InstanceConfig) AgentVersion() version.Binary {
	if len(cfg.tools) == 0 {
		return version.Binary{}
	}
	return cfg.tools[0].Version
}

// ToolsList returns the list of tools in the order in which they will
// be tried.
func (cfg *InstanceConfig) ToolsList() coretools.List {
	if cfg.tools == nil {
		return nil
	}
	return copyToolsList(cfg.tools)
}

// SetTools sets the tools that should be tried when provisioning this
// instance. There must be at least one. Other than the URL, each item
// must be the same.
//
// TODO(axw) 2016-04-19 lp:1572116
// SetTools should verify that the tools have URLs, since they will
// be needed for downloading on the instance. We can't do that until
// all usage-sites are updated to pass through non-empty URLs.
func (cfg *InstanceConfig) SetTools(toolsList coretools.List) error {
	if len(toolsList) == 0 {
		return errors.New("need at least 1 tools")
	}
	var tools *coretools.Tools
	for _, listed := range toolsList {
		if listed == nil {
			return errors.New("nil entry in tools list")
		}
		info := *listed
		info.URL = ""
		if tools == nil {
			tools = &info
			continue
		}
		if !reflect.DeepEqual(info, *tools) {
			return errors.Errorf("tools info mismatch (%v, %v)", *tools, info)
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

// VerifyConfig verifies that the InstanceConfig is valid.
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
	if cfg.MetricsSpoolDir == "" {
		return errors.New("missing metrics spool directory")
	}
	if len(cfg.Jobs) == 0 {
		return errors.New("missing machine jobs")
	}
	if cfg.CloudInitOutputLog == "" {
		return errors.New("missing cloud-init output log path")
	}
	if cfg.tools == nil {
		// SetTools() has never been called successfully.
		return errors.New("missing tools")
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

func (cfg *InstanceConfig) verifyBootstrapConfig() (err error) {
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

func (cfg *InstanceConfig) verifyControllerConfig() (err error) {
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
	if cfg.BootstrapMachineInstanceId == "" {
		return errors.New("missing bootstrap machine instance ID")
	}
	if len(cfg.HostedModelConfig) == 0 {
		return errors.New("missing hosted model config")
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

// DefaultBridgePrefix is the prefix for all network bridge device
// name used for LXC and KVM containers.
const DefaultBridgePrefix = "br-"

// DefaultBridgeName is the network bridge device name used for LXC and KVM
// containers
const DefaultBridgeName = DefaultBridgePrefix + "eth0"

// NewInstanceConfig sets up a basic machine configuration, for a
// non-bootstrap node. You'll still need to supply more information,
// but this takes care of the fixed entries and the ones that are
// always needed.
func NewInstanceConfig(
	controllerTag names.ControllerTag,
	machineID,
	machineNonce,
	imageStream,
	series string,
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
	metricsSpoolDir, err := paths.MetricsSpoolDir(series)
	if err != nil {
		return nil, err
	}
	cloudInitOutputLog := path.Join(logDir, "cloud-init-output.log")
	icfg := &InstanceConfig{
		// Fixed entries.
		DataDir:                 dataDir,
		LogDir:                  path.Join(logDir, "juju"),
		MetricsSpoolDir:         metricsSpoolDir,
		Jobs:                    []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		CloudInitOutputLog:      cloudInitOutputLog,
		MachineAgentServiceName: "jujud-" + names.NewMachineTag(machineID).String(),
		Series:                  series,
		Tags:                    map[string]string{},

		// Parameter entries.
		ControllerTag: controllerTag,
		MachineId:     machineID,
		MachineNonce:  machineNonce,
		APIInfo:       apiInfo,
		ImageStream:   imageStream,
	}
	return icfg, nil
}

// NewBootstrapInstanceConfig sets up a basic machine configuration for a
// bootstrap node.  You'll still need to supply more information, but this
// takes care of the fixed entries and the ones that are always needed.
func NewBootstrapInstanceConfig(
	config controller.Config,
	cons, modelCons constraints.Value,
	series, publicImageSigningKey string,
) (*InstanceConfig, error) {
	// For a bootstrap instance, the caller must provide the state.Info
	// and the api.Info. The machine id must *always* be "0".
	icfg, err := NewInstanceConfig(names.NewControllerTag(config.ControllerUUID()), "0", agent.BootstrapNonce, "", series, nil)
	if err != nil {
		return nil, err
	}
	icfg.Controller = &ControllerConfig{
		PublicImageSigningKey: publicImageSigningKey,
	}
	icfg.Controller.Config = make(map[string]interface{})
	for k, v := range config {
		icfg.Controller.Config[k] = v
	}
	icfg.Bootstrap = &BootstrapConfig{
		StateInitializationParams: StateInitializationParams{
			BootstrapMachineConstraints: cons,
			ModelConstraints:            modelCons,
		},
	}
	icfg.Jobs = []multiwatcher.MachineJob{
		multiwatcher.JobManageModel,
		multiwatcher.JobHostUnits,
	}
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
	enableOSRefreshUpdates bool,
	enableOSUpgrade bool,
) error {
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
	icfg.EnableOSRefreshUpdate = enableOSRefreshUpdates
	icfg.EnableOSUpgrade = enableOSUpgrade
	return nil
}

// FinishInstanceConfig sets fields on a InstanceConfig that can be determined by
// inspecting a plain config.Config and the machine constraints at the last
// moment before creating the user-data. It assumes that the supplied Config comes
// from an environment that has passed through all the validation checks in the
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
		cfg.EnableOSRefreshUpdate(),
		cfg.EnableOSUpgrade(),
	); err != nil {
		return errors.Trace(err)
	}
	if icfg.Controller != nil {
		// Add NUMACTL preference. Needed to work for both bootstrap and high availability
		// Only makes sense for controller
		logger.Debugf("Setting numa ctl preference to %v", icfg.Controller.Config.NumaCtlPreference())
		// Unfortunately, AgentEnvironment can only take strings as values
		icfg.AgentEnvironment[agent.NumaCtlPreference] = fmt.Sprintf("%v", icfg.Controller.Config.NumaCtlPreference())
	}
	return nil
}

// InstanceTags returns the minimum set of tags that should be set on a
// machine instance, if the provider supports them.
func InstanceTags(modelUUID, controllerUUID string, tagger tags.ResourceTagger, jobs []multiwatcher.MachineJob) map[string]string {
	instanceTags := tags.ResourceTags(
		names.NewModelTag(modelUUID),
		names.NewControllerTag(controllerUUID),
		tagger,
	)
	if multiwatcher.AnyJobNeedsState(jobs...) {
		instanceTags[tags.JujuIsController] = "true"
	}
	return instanceTags
}
