// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg

import (
	"net"
	"path"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/proxy"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/password"
)

// ControllerPodConfig represents initialization information for a new juju caas controller pod.
type ControllerPodConfig struct {
	// Tags is a set of tags/labels to set on the Pod, if supported. This
	// should be populated using the PodLabels method in this package.
	Tags map[string]string

	// Bootstrap contains bootstrap-specific configuration. If this is set,
	// Controller must also be set.
	Bootstrap *BootstrapConfig

	// DisableSSLHostnameVerification can be set to true to tell cloud-init
	// that it shouldn't verify SSL certificates
	DisableSSLHostnameVerification bool

	// ProxySettings encapsulates all proxy-related settings used to access
	// an outside network.
	ProxySettings proxy.Settings

	// Controller contains controller-specific configuration. If this is
	// set, then the instance will be configured as a controller pod.
	Controller controller.Config

	// APIInfo holds the means for the new pod to communicate with the
	// juju state API. Unless the new pod is running a controller (Controller is
	// set), there must be at least one controller address supplied.
	// The entity name must match that of the pod being started,
	// or be empty when starting a controller.
	APIInfo *api.Info

	// ControllerTag identifies the controller.
	ControllerTag names.ControllerTag

	// ControllerName is the controller name.
	ControllerName string

	// JujuVersion is the juju version.
	JujuVersion semversion.Number

	// DataDir holds the directory that juju state will be put in the new
	// instance.
	DataDir string

	// LogDir holds the directory that juju logs will be written to.
	LogDir string

	// MetricsSpoolDir represents the spool directory path, where all
	// metrics are stored.
	MetricsSpoolDir string

	// ControllerId identifies the new controller.
	ControllerId string

	// AgentEnvironment defines additional configuration variables to set in
	// the pod agent config.
	AgentEnvironment map[string]string
}

// BootstrapConfig represents bootstrap-specific initialization information
// for a new juju caas pod. This is only relevant for the bootstrap pod.
type BootstrapConfig struct {
	instancecfg.BootstrapConfig
}

// AgentConfig returns an agent config.
func (cfg *ControllerPodConfig) AgentConfig(tag names.Tag) (agent.ConfigSetterWriter, error) {
	configParams := agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir:         cfg.DataDir,
			LogDir:          cfg.LogDir,
			MetricsSpoolDir: cfg.MetricsSpoolDir,
		},
		Tag:                                tag,
		UpgradedToVersion:                  cfg.JujuVersion,
		Password:                           cfg.APIInfo.Password,
		APIAddresses:                       cfg.APIHostAddrs(),
		CACert:                             cfg.APIInfo.CACert,
		Values:                             cfg.AgentEnvironment,
		Controller:                         cfg.ControllerTag,
		Model:                              cfg.APIInfo.ModelTag,
		QueryTracingEnabled:                cfg.Controller.QueryTracingEnabled(),
		QueryTracingThreshold:              cfg.Controller.QueryTracingThreshold(),
		OpenTelemetryEnabled:               cfg.Controller.OpenTelemetryEnabled(),
		OpenTelemetryEndpoint:              cfg.Controller.OpenTelemetryEndpoint(),
		OpenTelemetryInsecure:              cfg.Controller.OpenTelemetryInsecure(),
		OpenTelemetryStackTraces:           cfg.Controller.OpenTelemetryStackTraces(),
		OpenTelemetrySampleRatio:           cfg.Controller.OpenTelemetrySampleRatio(),
		OpenTelemetryTailSamplingThreshold: cfg.Controller.OpenTelemetryTailSamplingThreshold(),
		ObjectStoreType:                    cfg.Controller.ObjectStoreType(),
	}
	return agent.NewStateMachineConfig(configParams, cfg.Bootstrap.StateServingInfo)
}

// UnitAgentConfig returns the agent config file for the controller unit charm.
// This is created a bootstrap time.
func (cfg *ControllerPodConfig) UnitAgentConfig() (agent.ConfigSetterWriter, error) {
	password, err := password.RandomPassword()
	if err != nil {
		return nil, errors.Trace(err)
	}
	configParams := agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir:         cfg.DataDir,
			LogDir:          cfg.LogDir,
			MetricsSpoolDir: cfg.MetricsSpoolDir,
		},
		Tag:               names.NewUnitTag("controller/" + cfg.ControllerId),
		UpgradedToVersion: cfg.JujuVersion,
		Password:          password,
		// Unit agent should always connect to the local controller.
		APIAddresses: []string{net.JoinHostPort(
			"localhost", strconv.Itoa(cfg.Bootstrap.StateServingInfo.APIPort),
		)},
		CACert:     cfg.APIInfo.CACert,
		Values:     cfg.AgentEnvironment,
		Controller: cfg.ControllerTag,
		Model:      cfg.APIInfo.ModelTag,
	}
	conf, err := agent.NewAgentConfig(configParams)
	if err != nil {
		return nil, errors.Trace(err)
	}
	conf.SetPassword(password)
	return conf, nil
}

// APIHostAddrs returns a list of api server addresses.
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

// VerifyConfig verifies that the ControllerPodConfig is valid.
func (cfg *ControllerPodConfig) VerifyConfig() (err error) {
	defer errors.DeferredAnnotatef(&err, "invalid controller pod configuration")
	if !names.IsValidControllerAgent(cfg.ControllerId) {
		return errors.New("invalid controller id")
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
	if cfg.JujuVersion == semversion.Zero {
		return errors.New("missing juju version")
	}
	if cfg.APIInfo == nil {
		return errors.New("missing API info")
	}
	if cfg.APIInfo.ModelTag.Id() == "" {
		return errors.New("missing model tag")
	}
	if len(cfg.APIInfo.CACert) == 0 {
		return errors.New("missing API CA certificate")
	}
	if cfg.ControllerName == "" {
		return errors.New("missing controller name")
	}

	if cfg.Bootstrap != nil {
		if err := cfg.verifyBootstrapConfig(); err != nil {
			return errors.Trace(err)
		}
	} else {
		if cfg.APIInfo.Tag != names.NewControllerAgentTag(cfg.ControllerId) {
			return errors.New("API entity tag must match started controller")
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
		return errors.New("controller pod config supplied without controller config")
	}
	if err := cfg.Bootstrap.VerifyConfig(); err != nil {
		return errors.Trace(err)
	}
	if cfg.APIInfo.Tag != nil {
		return errors.New("entity tag must be nil when bootstrapping")
	}
	if cfg.Bootstrap.ControllerCloud.HostCloudRegion == "" {
		return errors.New(`
host cloud region is missing.
The k8s cloud definition might be stale, please try to re-import the k8s cloud using
    juju add-k8s <cloud-name> --cluster-name <cluster-name> --client

See juju help add-k8s for more information.
`[1:])
	}
	return nil
}

// GetPodName returns pod name.
func (cfg *ControllerPodConfig) GetPodName() string {
	return "controller-" + cfg.ControllerId
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

// NewControllerPodConfig sets up a basic pod configuration. You'll still need to supply more information,
// but this takes care of the fixed entries and the ones that are
// always needed.
func NewControllerPodConfig(
	controllerTag names.ControllerTag,
	podID,
	controllerName,
	osName string,
	apiInfo *api.Info,
) (*ControllerPodConfig, error) {
	osType := paths.OSType(osName)
	pcfg := &ControllerPodConfig{
		// Fixed entries.
		DataDir:         paths.DataDir(osType),
		LogDir:          path.Join(paths.LogDir(osType), "juju"),
		MetricsSpoolDir: paths.MetricsSpoolDir(osType),
		Tags:            map[string]string{},
		// Parameter entries.
		ControllerTag:  controllerTag,
		ControllerId:   podID,
		ControllerName: controllerName,
		APIInfo:        apiInfo,
	}
	return pcfg, nil
}

// NewBootstrapControllerPodConfig sets up a basic pod configuration for a
// bootstrap pod.  You'll still need to supply more information, but this
// takes care of the fixed entries and the ones that are always needed.
func NewBootstrapControllerPodConfig(
	config controller.Config,
	controllerName,
	osname string,
	bootstrapConstraints constraints.Value,
) (*ControllerPodConfig, error) {
	// For a bootstrap pod, the caller must provide the state.Info
	// and the api.Info. The pod id must *always* be "0".
	pcfg, err := NewControllerPodConfig(
		names.NewControllerTag(config.ControllerUUID()), "0", controllerName, osname, nil,
	)
	if err != nil {
		return nil, err
	}
	pcfg.Controller = make(map[string]interface{})
	for k, v := range config {
		pcfg.Controller[k] = v
	}
	pcfg.Bootstrap = &BootstrapConfig{
		BootstrapConfig: instancecfg.BootstrapConfig{
			StateInitializationParams: instancecfg.StateInitializationParams{
				BootstrapMachineConstraints: bootstrapConstraints,
			},
		},
	}

	return pcfg, nil
}

// FinishControllerPodConfig sets fields on a ControllerPodConfig that can be determined by
// inspecting a plain config.Config and the pod constraints at the last
// moment before creating podspec. It assumes that the supplied Config comes
// from an environment that has passed through all the validation checks in the
// Bootstrap func, and that has set an agent-version (via finding the tools to,
// use for bootstrap, or otherwise).
func FinishControllerPodConfig(pcfg *ControllerPodConfig, cfg *config.Config, agentEnvironment map[string]string) {
	pcfg.DisableSSLHostnameVerification = !cfg.SSLHostnameVerification()
	pcfg.ProxySettings = cfg.JujuProxySettings()
	if pcfg.AgentEnvironment == nil {
		pcfg.AgentEnvironment = make(map[string]string)
	}
	pcfg.AgentEnvironment[agent.ProviderType] = cfg.Type()
	for k, v := range agentEnvironment {
		pcfg.AgentEnvironment[k] = v
	}
}

// PodLabels returns the minimum set of tags that should be set on a
// pod, if the provider supports them.
func PodLabels(modelUUID, controllerUUID string, tagger tags.ResourceTagger, jobs []model.MachineJob) map[string]string {
	podLabels := tags.ResourceTags(
		names.NewModelTag(modelUUID),
		names.NewControllerTag(controllerUUID),
		tagger,
	)
	// always be a controller.
	podLabels[tags.JujuIsController] = "true"
	return podLabels
}
