// Copyright 2012, 2013, 2015, 2016 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package instancecfg

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/proxy"
	"github.com/juju/utils/v4/shell"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/charm"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/service"
	"github.com/juju/juju/internal/service/common"
	"github.com/juju/juju/internal/storage"
	coretools "github.com/juju/juju/internal/tools"
)

var logger = internallogger.GetLogger("juju.cloudconfig.instancecfg")

// InstanceConfig represents initialization information for a new juju instance.
type InstanceConfig struct {
	// Tags is a set of tags to set on the instance, if supported. This
	// should be populated using the InstanceTags method in this package.
	Tags map[string]string

	// Bootstrap contains bootstrap-specific configuration. If this is set,
	// Controller must also be set.
	Bootstrap *BootstrapConfig

	// Controller contains configuration for the controller
	// used to manage this new instance.
	ControllerConfig controller.Config

	// The public key used to sign Juju simplestreams image metadata.
	PublicImageSigningKey string

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

	// TransientDataDir holds the directory that juju can use to write
	// transient files that get purged after a system reboot.
	TransientDataDir string

	// DataDir holds the directory that juju state will be put in the new
	// instance.
	DataDir string

	// LogDir holds the directory that juju logs will be written to.
	LogDir string

	// MetricsSpoolDir represents the spool directory path, where all
	// metrics are stored.
	MetricsSpoolDir string

	// Jobs holds what machine jobs to run.
	Jobs []model.MachineJob

	// CloudInitOutputLog specifies the path to the output log for cloud-init.
	// The directory containing the log file must already exist.
	CloudInitOutputLog string

	// CloudInitUserData defines key/value pairs from the model-config
	// specified by the user.
	CloudInitUserData map[string]interface{}

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

	// Base represents the instance base.
	Base corebase.Base

	// MachineAgentServiceName is the init service name for the Juju machine agent.
	MachineAgentServiceName string

	// LegacyProxySettings define normal http, https and ftp proxies.
	// These values are written to the /etc for the user profile and systemd settings.
	LegacyProxySettings proxy.Settings

	// JujuProxySettings define normal http, https and ftp proxies for accessing
	// the outside network. These values are not written to disk.
	JujuProxySettings proxy.Settings

	// AptProxySettings define the http, https and ftp proxy settings to use
	// for apt, which may or may not be the same as the normal ProxySettings.
	AptProxySettings proxy.Settings

	// AptMirror defines an APT mirror location, which, if specified, will
	// override the default APT sources.
	AptMirror string

	// SnapProxySettings define the http, https and ftp proxy settings to
	// use for snap, which may or may not be the same as the normal
	// ProxySettings.
	SnapProxySettings proxy.Settings

	// SnapStoreAssertions contains a list of assertions that must be
	// passed to snapd together with a store proxy ID parameter before it
	// can connect to a snap store proxy.
	SnapStoreAssertions string

	// SnapStoreProxyID references a store entry in the snap store
	// assertion list that must be passed to snapd before it can connect to
	// a snap store proxy.
	SnapStoreProxyID string

	// SnapStoreProxyURL specifies the address of the snap store proxy. If
	// specified instead of the assertions/storeID settings above, juju can
	// directly contact the proxy to retrieve the assertions and store ID.
	SnapStoreProxyURL string

	// The type of Simple Stream to download and deploy on this instance.
	ImageStream string

	// EnableOSRefreshUpdate specifies whether Juju will refresh its
	// respective OS's updates list.
	EnableOSRefreshUpdate bool

	// EnableOSUpgrade defines Juju's behavior when provisioning
	// instances. If enabled, the OS will perform any upgrades
	// available as part of its provisioning.
	EnableOSUpgrade bool

	// NetBondReconfigureDelay defines the duration in seconds that the
	// networking bridgescript should pause between ifdown, then
	// ifup when bridging bonded interfaces. See bugs #1594855 and
	// #1269921.
	NetBondReconfigureDelay int

	// Profiles is a slice of (lxd) profile names to be used by a container
	Profiles []string
}

// BootstrapConfig represents bootstrap-specific initialization information
// for a new juju instance. This is only relevant for the bootstrap machine.
type BootstrapConfig struct {
	StateInitializationParams

	// ControllerCharm is a local controller charm to be used.
	ControllerCharm string

	// Timeout is the amount of time to wait for bootstrap to complete.
	Timeout time.Duration

	// InitialSSHHostKeys contains the initial SSH host keys to configure
	// on the bootstrap machine, indexed by algorithm. These will only be
	// valid for the initial SSH connection. The first thing we do upon
	// making the initial SSH connection is to replace each of these host
	// keys, to avoid the host keys being extracted from the metadata
	// service by a bad actor post-bootstrap.
	//
	// Any existing host keys on the machine with algorithms not specified
	// in the map will be left alone. This is important so that we do not
	// trample on the host keys of manually provisioned machines.
	InitialSSHHostKeys SSHHostKeys

	// ControllerAgentInfo holds the information for the controller agent.
	// This is only specified for bootstrap; controllers started
	// subsequently will acquire their serving info from another
	// server.
	ControllerAgentInfo controller.ControllerAgentInfo

	// JujuDbSnapPath is the path to a .snap file that will be used as the juju-db
	// service.
	JujuDbSnapPath string

	// JujuDbSnapAssertions is a path to a .assert file that will be used
	// to verify the .snap at JujuDbSnapPath
	JujuDbSnapAssertionsPath string

	// ControllerServiceType is the service type of a k8s controller.
	ControllerServiceType string

	// IgnoreProxy tells the boostrap provider to no deploy any controller
	// proxying resources. Currently only used in k8s
	IgnoreProxy bool

	// ControllerExternalName is the external name of a k8s controller.
	ControllerExternalName string

	// ControllerExternalIPs is the list of external ips for a k8s controller.
	ControllerExternalIPs []string
}

// SSHHostKeys contains the SSH host keys to configure for a bootstrap host.
type SSHHostKeys []SSHKeyPair

// SSHKeyPair is an SSH host key pair.
type SSHKeyPair struct {
	// Private contains the private key, PEM-encoded.
	Private string

	// Public contains the public key in authorized_keys format.
	Public string

	// PublicKeyAlgorithm contains the public key algorithm as defined by golang.org/x/crypto/ssh KeyAlgo*
	PublicKeyAlgorithm string
}

// StateInitializationParams contains parameters for initializing the
// state database.
//
// This structure will be passed to the bootstrap agent. To do so, the
// Marshal and Unmarshal methods must be used.
type StateInitializationParams struct {
	// AgentVersion is the desired agent version to run for models created as
	// part of state initialization.
	AgentVersion semversion.Number

	// ControllerModelConfig holds the initial controller model configuration.
	ControllerModelConfig *config.Config

	// ControllerModelAuthorizedKeys is a list of authorized keys to be added to
	// the controller model and the admin user during bootstrap.
	ControllerModelAuthorizedKeys []string

	// ControllerModelEnvironVersion holds the initial controller model
	// environ version.
	ControllerModelEnvironVersion int

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

	// ControllerCharmPath points to a controller charm on Charmhub.
	ControllerCharmPath string

	// ControllerCharmChannel is used when deploying the controller charm.
	ControllerCharmChannel charm.Channel

	// ControllerInheritedConfig is a set of default config attributes to be set
	// as defaults on the cloud that is in use by the controller
	// ("the controller cloud"). These default config attributes do not actually
	// get applied to every model in reality just to models that use the same
	// cloud as the controller.
	ControllerInheritedConfig map[string]interface{}

	// RegionInheritedConfig holds region specific configuration attributes to
	// be shared across all models in the same controller on a particular
	// cloud.
	RegionInheritedConfig cloud.RegionConfig

	// BootstrapMachineInstanceId is the instance ID of the bootstrap
	// machine instance being initialized.
	BootstrapMachineInstanceId instance.Id

	// BootstrapMachineDisplayName is the human readable name for
	// the bootstrap machine instance being initialized.
	BootstrapMachineDisplayName string

	// BootstrapMachineConstraints holds the constraints for the bootstrap
	// machine.
	BootstrapMachineConstraints constraints.Value

	// BootstrapMachineHardwareCharacteristics contains the hardware
	// characteristics of the bootstrap machine instance being initialized.
	BootstrapMachineHardwareCharacteristics *instance.HardwareCharacteristics

	// ModelConstraints holds the initial model constraints.
	ModelConstraints constraints.Value

	// CustomImageMetadata is optional custom simplestreams image metadata
	// to store in environment storage at bootstrap time. This is ignored
	// in non-bootstrap instances.
	CustomImageMetadata []*imagemetadata.ImageMetadata

	// StoragePools is one or more named storage pools to create
	// in the controller model.
	StoragePools map[string]storage.Attrs

	// SSHServerHostKey holds the host key to be used within the embedded SSH server for Juju.
	SSHServerHostKey string
}

type stateInitializationParamsInternal struct {
	AgentVersion                            string                            `yaml:"agent-version"`
	ControllerConfig                        map[string]any                    `yaml:"controller-config"`
	ControllerModelConfig                   map[string]any                    `yaml:"controller-model-config"`
	ControllerModelAuthorizedKeys           []string                          `yaml:"controller-model-authorized-keys"`
	ControllerModelEnvironVersion           int                               `yaml:"controller-model-version"`
	ControllerInheritedConfig               map[string]interface{}            `yaml:"controller-config-defaults,omitempty"`
	RegionInheritedConfig                   cloud.RegionConfig                `yaml:"region-inherited-config,omitempty"`
	StoragePools                            map[string]storage.Attrs          `yaml:"storage-pools,omitempty"`
	BootstrapMachineInstanceId              instance.Id                       `yaml:"bootstrap-machine-instance-id,omitempty"`
	BootstrapMachineConstraints             constraints.Value                 `yaml:"bootstrap-machine-constraints"`
	BootstrapMachineHardwareCharacteristics *instance.HardwareCharacteristics `yaml:"bootstrap-machine-hardware,omitempty"`
	BootstrapMachineDisplayName             string                            `yaml:"bootstrap-machine-display-name,omitempty"`
	ModelConstraints                        constraints.Value                 `yaml:"model-constraints"`
	CustomImageMetadataJSON                 string                            `yaml:"custom-image-metadata,omitempty"`
	ControllerCloud                         string                            `yaml:"controller-cloud"`
	ControllerCloudRegion                   string                            `yaml:"controller-cloud-region"`
	ControllerCloudCredentialName           string                            `yaml:"controller-cloud-credential-name,omitempty"`
	ControllerCloudCredential               *cloud.Credential                 `yaml:"controller-cloud-credential,omitempty"`
	ControllerCharmPath                     string                            `yaml:"controller-charm-path,omitempty"`
	ControllerCharmChannel                  charm.Channel                     `yaml:"controller-charm-channel,omitempty"`
	SSHServerHostKey                        string                            `yaml:"ssh-server-host-key,omitempty"`
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
		AgentVersion:                            p.AgentVersion.String(),
		ControllerConfig:                        p.ControllerConfig,
		ControllerModelConfig:                   p.ControllerModelConfig.AllAttrs(),
		ControllerModelAuthorizedKeys:           p.ControllerModelAuthorizedKeys,
		ControllerModelEnvironVersion:           p.ControllerModelEnvironVersion,
		ControllerInheritedConfig:               p.ControllerInheritedConfig,
		RegionInheritedConfig:                   p.RegionInheritedConfig,
		StoragePools:                            p.StoragePools,
		BootstrapMachineInstanceId:              p.BootstrapMachineInstanceId,
		BootstrapMachineConstraints:             p.BootstrapMachineConstraints,
		BootstrapMachineHardwareCharacteristics: p.BootstrapMachineHardwareCharacteristics,
		BootstrapMachineDisplayName:             p.BootstrapMachineDisplayName,
		ModelConstraints:                        p.ModelConstraints,
		CustomImageMetadataJSON:                 string(customImageMetadataJSON),
		ControllerCloud:                         string(controllerCloud),
		ControllerCloudRegion:                   p.ControllerCloudRegion,
		ControllerCloudCredentialName:           p.ControllerCloudCredentialName,
		ControllerCloudCredential:               p.ControllerCloudCredential,
		ControllerCharmPath:                     p.ControllerCharmPath,
		ControllerCharmChannel:                  p.ControllerCharmChannel,
		SSHServerHostKey:                        p.SSHServerHostKey,
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
	agentVersion, err := semversion.Parse(internal.AgentVersion)
	if err != nil {
		return fmt.Errorf("parsing agent-version in state initialisation params: %w", err)
	}
	*p = StateInitializationParams{
		AgentVersion:                            agentVersion,
		ControllerConfig:                        internal.ControllerConfig,
		ControllerModelConfig:                   cfg,
		ControllerModelAuthorizedKeys:           internal.ControllerModelAuthorizedKeys,
		ControllerModelEnvironVersion:           internal.ControllerModelEnvironVersion,
		ControllerInheritedConfig:               internal.ControllerInheritedConfig,
		RegionInheritedConfig:                   internal.RegionInheritedConfig,
		StoragePools:                            internal.StoragePools,
		BootstrapMachineInstanceId:              internal.BootstrapMachineInstanceId,
		BootstrapMachineConstraints:             internal.BootstrapMachineConstraints,
		BootstrapMachineHardwareCharacteristics: internal.BootstrapMachineHardwareCharacteristics,
		BootstrapMachineDisplayName:             internal.BootstrapMachineDisplayName,
		ModelConstraints:                        internal.ModelConstraints,
		CustomImageMetadata:                     imageMetadata,
		ControllerCloud:                         controllerCloud,
		ControllerCloudRegion:                   internal.ControllerCloudRegion,
		ControllerCloudCredentialName:           internal.ControllerCloudCredentialName,
		ControllerCloudCredential:               internal.ControllerCloudCredential,
		ControllerCharmPath:                     internal.ControllerCharmPath,
		ControllerCharmChannel:                  internal.ControllerCharmChannel,
		SSHServerHostKey:                        internal.SSHServerHostKey,
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

func (cfg *InstanceConfig) IsController() bool {
	return model.AnyJobNeedsState(cfg.Jobs...)
}

func (cfg *InstanceConfig) ToolsDir(renderer shell.Renderer) string {
	return cfg.agentInfo().ToolsDir(renderer)
}

func (cfg *InstanceConfig) InitService(renderer shell.Renderer) (service.Service, error) {
	conf := service.AgentConf(cfg.agentInfo(), renderer)

	name := cfg.MachineAgentServiceName
	svc, err := newService(name, conf)
	return svc, errors.Trace(err)
}

var newService = func(name string, conf common.Conf) (service.Service, error) {
	return service.NewService(name, conf)
}

func (cfg *InstanceConfig) AgentConfig(
	tag names.Tag,
	toolsVersion semversion.Number,
) (agent.ConfigSetter, error) {
	configParams := agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir:          cfg.DataDir,
			TransientDataDir: cfg.TransientDataDir,
			LogDir:           cfg.LogDir,
			MetricsSpoolDir:  cfg.MetricsSpoolDir,
		},
		Jobs:              cfg.Jobs,
		Tag:               tag,
		UpgradedToVersion: toolsVersion,
		Password:          cfg.APIInfo.Password,
		Nonce:             cfg.MachineNonce,
		APIAddresses:      cfg.APIHostAddrs(),
		CACert:            cfg.APIInfo.CACert,
		Values:            cfg.AgentEnvironment,
		Controller:        cfg.ControllerTag,
		Model:             cfg.APIInfo.ModelTag,
	}
	if cfg.ControllerConfig != nil {
		configParams.AgentLogfileMaxBackups = cfg.ControllerConfig.AgentLogfileMaxBackups()
		configParams.AgentLogfileMaxSizeMB = cfg.ControllerConfig.AgentLogfileMaxSizeMB()
		configParams.QueryTracingEnabled = cfg.ControllerConfig.QueryTracingEnabled()
		configParams.QueryTracingThreshold = cfg.ControllerConfig.QueryTracingThreshold()
		configParams.OpenTelemetryEnabled = cfg.ControllerConfig.OpenTelemetryEnabled()
		configParams.OpenTelemetryEndpoint = cfg.ControllerConfig.OpenTelemetryEndpoint()
		configParams.OpenTelemetryInsecure = cfg.ControllerConfig.OpenTelemetryInsecure()
		configParams.OpenTelemetryStackTraces = cfg.ControllerConfig.OpenTelemetryStackTraces()
		configParams.OpenTelemetrySampleRatio = cfg.ControllerConfig.OpenTelemetrySampleRatio()
		configParams.OpenTelemetryTailSamplingThreshold = cfg.ControllerConfig.OpenTelemetryTailSamplingThreshold()
		configParams.ObjectStoreType = cfg.ControllerConfig.ObjectStoreType()
	}
	if cfg.Bootstrap == nil {
		return agent.NewAgentConfig(configParams)
	}
	return agent.NewStateMachineConfig(configParams, cfg.Bootstrap.ControllerAgentInfo)
}

// JujuTools returns the directory where Juju tools are stored.
func (cfg *InstanceConfig) JujuTools() string {
	return agenttools.SharedToolsDir(cfg.DataDir, cfg.AgentVersion())
}

// SnapDir returns the directory where snaps should be uploaded to.
func (cfg *InstanceConfig) SnapDir() string {
	return path.Join(cfg.DataDir, "snap")
}

// CharmDir returns the directory where system charms should be uploaded to.
func (cfg *InstanceConfig) CharmDir() string {
	return path.Join(cfg.DataDir, "charms")
}

func (cfg *InstanceConfig) APIHostAddrs() []string {
	var hosts []string
	if cfg.Bootstrap != nil {
		hosts = append(hosts, net.JoinHostPort(
			"localhost", strconv.Itoa(cfg.Bootstrap.ControllerAgentInfo.APIPort)),
		)
	}
	if cfg.APIInfo != nil {
		hosts = append(hosts, cfg.APIInfo.Addrs...)
	}
	return hosts
}

func (cfg *InstanceConfig) APIHosts() []string {
	var hosts []string
	if cfg.Bootstrap != nil {
		hosts = append(hosts, "localhost")
	}
	if cfg.APIInfo != nil {
		for _, addr := range cfg.APIInfo.Addrs {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				logger.Errorf(context.TODO(), "Can't split API address %q to host:port - %q", host, err)
				continue
			}
			hosts = append(hosts, host)
		}
	}
	return hosts
}

// AgentVersion returns the version of the Juju agent that will be configured
// on the instance. The zero value will be returned if there are no tools set.
func (cfg *InstanceConfig) AgentVersion() semversion.Binary {
	if len(cfg.tools) == 0 {
		return semversion.Binary{}
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

// SetSnapSource annotates the instance configuration
// with the location of a local .snap to upload during
// the instance's provisioning.
func (cfg *InstanceConfig) SetSnapSource(snapPath string, snapAssertionsPath string) error {
	if snapPath == "" {
		return nil
	}

	_, err := os.Stat(snapPath)
	if err != nil {
		return errors.Annotatef(err, "unable set local snap (at %s)", snapPath)
	}

	_, err = os.Stat(snapAssertionsPath)
	if err != nil {
		return errors.Annotatef(err, "unable set local snap .assert (at %s)", snapAssertionsPath)
	}

	cfg.Bootstrap.JujuDbSnapPath = snapPath
	cfg.Bootstrap.JujuDbSnapAssertionsPath = snapAssertionsPath

	return nil
}

// SetControllerCharm annotates the instance configuration
// with the location of a local controller charm to upload during
// the instance's provisioning.
func (cfg *InstanceConfig) SetControllerCharm(controllerCharmPath string) error {
	if controllerCharmPath == "" {
		return nil
	}

	_, err := os.Stat(controllerCharmPath)
	if err != nil {
		return errors.Annotatef(err, "unable set local controller charm (at %s)", controllerCharmPath)
	}

	cfg.Bootstrap.ControllerCharm = controllerCharmPath

	return nil
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
	if cfg.ControllerConfig == nil {
		return errors.New("bootstrap config supplied without controller config")
	}
	if err := cfg.Bootstrap.VerifyConfig(); err != nil {
		return errors.Trace(err)
	}
	if cfg.APIInfo.Tag != nil {
		return errors.New("entity tag must be nil when bootstrapping")
	}
	return nil
}

// VerifyConfig verifies that the BootstrapConfig is valid.
func (cfg *BootstrapConfig) VerifyConfig() (err error) {
	if cfg.ControllerModelConfig == nil {
		return errors.New("missing model configuration")
	}
	if len(cfg.ControllerAgentInfo.Cert) == 0 {
		return errors.New("missing controller certificate")
	}
	if len(cfg.ControllerAgentInfo.PrivateKey) == 0 {
		return errors.New("missing controller private key")
	}
	if len(cfg.ControllerAgentInfo.CAPrivateKey) == 0 {
		return errors.New("missing ca cert private key")
	}
	if cfg.ControllerAgentInfo.APIPort == 0 {
		return errors.New("missing API port")
	}
	if cfg.BootstrapMachineInstanceId == "" {
		return errors.New("missing bootstrap machine instance ID")
	}
	return nil
}

// DefaultBridgeName is the network bridge device name used for LXC and KVM
// containers
const DefaultBridgeName = "br-eth0"

// NewInstanceConfig sets up a basic machine configuration, for a
// non-bootstrap node. You'll still need to supply more information,
// but this takes care of the fixed entries and the ones that are
// always needed.
func NewInstanceConfig(
	controllerTag names.ControllerTag,
	machineID,
	machineNonce,
	imageStream string,
	base corebase.Base,
	apiInfo *api.Info,
) (*InstanceConfig, error) {
	osType := paths.OSType(base.OS)
	logDir := paths.LogDir(osType)
	icfg := &InstanceConfig{
		// Fixed entries.
		DataDir:                 paths.DataDir(osType),
		LogDir:                  path.Join(logDir, "juju"),
		MetricsSpoolDir:         paths.MetricsSpoolDir(osType),
		Jobs:                    []model.MachineJob{model.JobHostUnits},
		CloudInitOutputLog:      path.Join(logDir, "cloud-init-output.log"),
		TransientDataDir:        paths.TransientDataDir(osType),
		MachineAgentServiceName: "jujud-" + names.NewMachineTag(machineID).String(),
		Base:                    base,
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
	base corebase.Base, publicImageSigningKey string,
	agentEnvironment map[string]string,
) (*InstanceConfig, error) {
	// For a bootstrap instance, the caller must provide the state.Info
	// and the api.Info. The machine id must *always* be "0".
	icfg, err := NewInstanceConfig(names.NewControllerTag(config.ControllerUUID()), agent.BootstrapControllerId, agent.BootstrapNonce, "", base, nil)
	if err != nil {
		return nil, err
	}
	icfg.PublicImageSigningKey = publicImageSigningKey
	icfg.ControllerConfig = make(map[string]interface{})
	for k, v := range config {
		icfg.ControllerConfig[k] = v
	}
	icfg.Bootstrap = &BootstrapConfig{
		StateInitializationParams: StateInitializationParams{
			BootstrapMachineConstraints: cons,
			ModelConstraints:            modelCons,
		},
	}
	for k, v := range agentEnvironment {
		if icfg.AgentEnvironment == nil {
			icfg.AgentEnvironment = make(map[string]string)
		}
		icfg.AgentEnvironment[k] = v
	}
	icfg.Jobs = []model.MachineJob{
		model.JobManageModel,
		model.JobHostUnits,
	}
	return icfg, nil
}

// ProxyConfiguration encapsulates all proxy-related settings that can be used
// to populate an InstanceConfig.
type ProxyConfiguration struct {
	// Legacy proxy settings.
	Legacy proxy.Settings

	// Juju-specific proxy settings.
	Juju proxy.Settings

	// Apt-specific proxy settings.
	Apt proxy.Settings

	// Snap-specific proxy settings.
	Snap proxy.Settings

	// Apt mirror.
	AptMirror string

	// SnapStoreAssertions contains a list of assertions that must be
	// passed to snapd together with a store proxy ID parameter before it
	// can connect to a snap store proxy.
	SnapStoreAssertions string

	// SnapStoreProxyID references a store entry in the snap store
	// assertion list that must be passed to snapd before it can connect to
	// a snap store proxy.
	SnapStoreProxyID string

	// SnapStoreProxyURL specifies the address of the snap store proxy. If
	// specified instead of the assertions/storeID settings above, juju can
	// directly contact the proxy to retrieve the assertions and store ID.
	SnapStoreProxyURL string
}

// proxyConfigurationFromEnv populates a ProxyConfiguration object from an
// environment Config value.
func proxyConfigurationFromEnv(cfg *config.Config) ProxyConfiguration {
	return ProxyConfiguration{
		Legacy:              cfg.LegacyProxySettings(),
		Juju:                cfg.JujuProxySettings(),
		Apt:                 cfg.AptProxySettings(),
		AptMirror:           cfg.AptMirror(),
		Snap:                cfg.SnapProxySettings(),
		SnapStoreAssertions: cfg.SnapStoreAssertions(),
		SnapStoreProxyID:    cfg.SnapStoreProxy(),
		SnapStoreProxyURL:   cfg.SnapStoreProxyURL(),
	}
}

// PopulateInstanceConfig is called both from the FinishInstanceConfig below,
// which does have access to the environment config, and from the container
// provisioners, which don't have access to the environment config. Everything
// that is needed to provision a container needs to be returned to the
// provisioner in the ContainerConfig structure. Those values are then used to
// call this function.
func PopulateInstanceConfig(icfg *InstanceConfig,
	providerType string,
	sslHostnameVerification bool,
	proxyCfg ProxyConfiguration,
	enableOSRefreshUpdates bool,
	enableOSUpgrade bool,
	cloudInitUserData map[string]interface{},
	profiles []string,
) error {
	if icfg.AgentEnvironment == nil {
		icfg.AgentEnvironment = make(map[string]string)
	}
	icfg.AgentEnvironment[agent.ProviderType] = providerType
	icfg.AgentEnvironment[agent.ContainerType] = string(icfg.MachineContainerType)
	icfg.DisableSSLHostnameVerification = !sslHostnameVerification
	icfg.LegacyProxySettings = proxyCfg.Legacy
	icfg.LegacyProxySettings.AutoNoProxy = strings.Join(icfg.APIHosts(), ",")
	icfg.JujuProxySettings = proxyCfg.Juju
	// No AutoNoProxy needed as juju no proxy values are CIDR aware.
	icfg.AptProxySettings = proxyCfg.Apt
	icfg.AptMirror = proxyCfg.AptMirror
	icfg.SnapProxySettings = proxyCfg.Snap
	icfg.SnapStoreAssertions = proxyCfg.SnapStoreAssertions
	icfg.SnapStoreProxyID = proxyCfg.SnapStoreProxyID
	icfg.SnapStoreProxyURL = proxyCfg.SnapStoreProxyURL
	icfg.EnableOSRefreshUpdate = enableOSRefreshUpdates
	icfg.EnableOSUpgrade = enableOSUpgrade
	icfg.CloudInitUserData = cloudInitUserData
	icfg.Profiles = profiles
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
// redeeming featureflag.
func FinishInstanceConfig(
	icfg *InstanceConfig,
	cfg *config.Config,
) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot complete machine configuration")
	if err := PopulateInstanceConfig(
		icfg,
		cfg.Type(),
		cfg.SSLHostnameVerification(),
		proxyConfigurationFromEnv(cfg),
		cfg.EnableOSRefreshUpdate(),
		cfg.EnableOSUpgrade(),
		cfg.CloudInitUserData(),
		nil,
	); err != nil {
		return errors.Trace(err)
	}
	if icfg.IsController() {
		// Add NUMACTL preference. Needed to work for both bootstrap and high availability
		// Only makes sense for controller,
		logger.Debugf(context.TODO(), "Setting numa ctl preference to %v", icfg.ControllerConfig.NUMACtlPreference())
		// Unfortunately, AgentEnvironment can only take strings as values
		icfg.AgentEnvironment[agent.NUMACtlPreference] = fmt.Sprintf("%v", icfg.ControllerConfig.NUMACtlPreference())
	}
	return nil
}

// InstanceTags returns the minimum set of tags that should be set on a
// machine instance, if the provider supports them.
func InstanceTags(modelUUID, controllerUUID string, tagger tags.ResourceTagger, isController bool) map[string]string {
	instanceTags := tags.ResourceTags(
		names.NewModelTag(modelUUID),
		names.NewControllerTag(controllerUUID),
		tagger,
	)
	if isController {
		instanceTags[tags.JujuIsController] = "true"
	}
	return instanceTags
}
