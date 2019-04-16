// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg

import (
	"fmt"
	"net"
	"path"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/multiwatcher"
)

var logger = loggo.GetLogger("juju.cloudconfig.podcfg")

const (
	jujudOCINamespace = "jujusolutions"
	jujudOCIName      = "jujud-operator"
	jujudbOCIName     = "juju-db"
)

// jujudbVersion is the version of juju-db to use.
var jujudbVersion = mongo.Mongo40wt

// ControllerPodConfig represents initialization information for a new juju caas controller pod.
type ControllerPodConfig struct {
	// Tags is a set of tags/labels to set on the Pod, if supported. This
	// should be populated using the PodLabels method in this package.
	Tags map[string]string

	// Bootstrap contains bootstrap-specific configuration. If this is set,
	// Controller must also be set.
	Bootstrap *BootstrapConfig

	// Controller contains controller-specific configuration. If this is
	// set, then the instance will be configured as a controller pod.
	Controller *ControllerConfig

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

	// TODO(bootstrap): remove me.
	// PodNonce is set at provisioning/bootstrap time and used to
	// ensure the agent is running on the correct instance.
	PodNonce string

	// JujuVersion is the juju version.
	JujuVersion version.Number

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
	MachineId string // TODO(caas): change it to PodId once we introduced the new tag for pod.

	// AgentEnvironment defines additional configuration variables to set in
	// the pod agent config.
	AgentEnvironment map[string]string
}

// BootstrapConfig represents bootstrap-specific initialization information
// for a new juju caas pod. This is only relevant for the bootstrap pod.
type BootstrapConfig struct {
	instancecfg.BootstrapConfig

	// ControllerServiceType is the service type to use for a k8s controller.
	ControllerServiceType string
}

// ControllerConfig represents controller-specific initialization information
// for a new juju caas pod. This is only relevant for controller pod.
type ControllerConfig struct {
	instancecfg.ControllerConfig
}

// AgentConfig returns an agent config.
func (cfg *ControllerPodConfig) AgentConfig(tag names.Tag) (agent.ConfigSetterWriter, error) {
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
		Jobs:               cfg.Jobs,
		Tag:                tag,
		UpgradedToVersion:  cfg.JujuVersion,
		Password:           password,
		Nonce:              cfg.PodNonce,
		APIAddresses:       cfg.APIHostAddrs(),
		CACert:             cacert,
		Values:             cfg.AgentEnvironment,
		Controller:         cfg.ControllerTag,
		Model:              cfg.APIInfo.ModelTag,
		MongoVersion:       jujudbVersion,
		MongoMemoryProfile: mongo.MemoryProfile(cfg.Controller.Config.MongoMemoryProfile()),
	}
	return agent.NewStateMachineConfig(configParams, cfg.Bootstrap.StateServingInfo)
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
	if cfg.JujuVersion == version.Zero {
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
	if cfg.PodNonce == "" {
		return errors.New("missing pod nonce")
	}
	if cfg.ControllerName == "" {
		return errors.New("missing controller name")
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

// GetControllerImagePath returns oci image path of jujud for a controller.
func (cfg *ControllerPodConfig) GetControllerImagePath() string {
	return GetJujuOCIImagePath(cfg.Controller.Config, cfg.JujuVersion)
}

// GetJujuDbOCIImagePath returns the juju-db oci image path.
func (cfg *ControllerPodConfig) GetJujuDbOCIImagePath() string {
	imageRepo := cfg.Controller.Config.CAASImageRepo()
	if imageRepo == "" {
		imageRepo = jujudOCINamespace
	}
	v := jujudbVersion
	return fmt.Sprintf("%s/%s:%d.%d", imageRepo, jujudbOCIName, v.Major, v.Minor)
}

// IsJujuOCIImage returns true if the image path is for a Juju operator.
func IsJujuOCIImage(imagePath string) bool {
	return strings.Contains(imagePath, jujudOCIName+":")
}

// GetJujuOCIImagePath returns the jujud oci image path.
func GetJujuOCIImagePath(controllerCfg controller.Config, ver version.Number) string {
	// First check the deprecated "caas-operator-image-path" config.
	imagePath := controllerCfg.CAASOperatorImagePath()
	if imagePath != "" {
		return imagePath
	}
	imageRepo := controllerCfg.CAASImageRepo()
	if imageRepo == "" {
		imageRepo = jujudOCINamespace
	}
	if ver == version.Zero {
		return fmt.Sprintf("%s/%s", imageRepo, jujudOCIName)
	}
	return fmt.Sprintf("%s/%s:%s", imageRepo, jujudOCIName, ver.String())
}

func (cfg *ControllerPodConfig) verifyBootstrapConfig() (err error) {
	defer errors.DeferredAnnotatef(&err, "invalid bootstrap configuration")

	if cfg.Controller == nil {
		return errors.New("controller pod config supplied without controller config")
	}
	if err := cfg.Bootstrap.VerifyConfig(); err != nil {
		return errors.Trace(err)
	}
	if cfg.APIInfo.Tag != nil || cfg.Controller.MongoInfo.Tag != nil {
		return errors.New("entity tag must be nil when bootstrapping")
	}
	if cfg.Bootstrap.ControllerServiceType == "" {
		return errors.New(`
controller service type is missing.
The k8s cloud definition might be stale, please try to re-import the k8s cloud using
    juju add-k8s <cloud-name> --cluster-name <cluster-name> --local

See juju help add-k8s for more information.
`[1:])
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

// GetHostedModel checks if hosted model was requested to create.
func (cfg *ControllerPodConfig) GetHostedModel() (string, bool) {
	hasHostedModel := len(cfg.Bootstrap.HostedModelConfig) > 0
	if hasHostedModel {
		modelName := cfg.Bootstrap.HostedModelConfig[config.NameKey].(string)
		logger.Debugf("configured hosted model %q for bootstrapping", modelName)
		return modelName, true
	}
	return "", false
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

// NewControllerPodConfig sets up a basic pod configuration. You'll still need to supply more information,
// but this takes care of the fixed entries and the ones that are
// always needed.
func NewControllerPodConfig(
	controllerTag names.ControllerTag,
	podID,
	podNonce,
	controllerName,
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
		Tags:            map[string]string{},
		// Parameter entries.
		ControllerTag:  controllerTag,
		MachineId:      podID,
		PodNonce:       podNonce,
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
	series,
	controllerServiceType string,

) (*ControllerPodConfig, error) {
	// For a bootstrap pod, the caller must provide the state.Info
	// and the api.Info. The machine id must *always* be "0".
	pcfg, err := NewControllerPodConfig(
		names.NewControllerTag(config.ControllerUUID()), "0", agent.BootstrapNonce, controllerName, series, nil,
	)
	if err != nil {
		return nil, err
	}
	pcfg.Controller = &ControllerConfig{}
	pcfg.Controller.Config = make(map[string]interface{})
	for k, v := range config {
		pcfg.Controller.Config[k] = v
	}
	// TODO(bootstrap): remove me.
	arch := arch.AMD64
	var cores uint64 = 2
	var mem uint64 = 123
	var rootDisk uint64 = 123
	pcfg.Bootstrap = &BootstrapConfig{
		BootstrapConfig: instancecfg.BootstrapConfig{
			StateInitializationParams: instancecfg.StateInitializationParams{
				// TODO(bootstrap): remove me once agentbootstrap.initBootstrapMachine works for CAAS bootstrap in jujud.
				BootstrapMachineHardwareCharacteristics: &instance.HardwareCharacteristics{
					Arch:     &arch,
					CpuCores: &cores,
					Mem:      &mem,
					RootDisk: &rootDisk,
				},
				BootstrapMachineInstanceId:  "i-0a373a526fcf5c882",
				BootstrapMachineConstraints: constraints.Value{Mem: &mem},
			},
		},
		ControllerServiceType: controllerServiceType,
	}

	pcfg.Jobs = []multiwatcher.MachineJob{
		multiwatcher.JobManageModel,
		multiwatcher.JobHostUnits,
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
	pcfg.AgentEnvironment[agent.AgentServiceName] = "jujud-" + names.NewMachineTag(pcfg.MachineId).String()
	return nil
}

// FinishControllerPodConfig sets fields on a ControllerPodConfig that can be determined by
// inspecting a plain config.Config and the pod constraints at the last
// moment before creating podspec. It assumes that the supplied Config comes
// from an environment that has passed through all the validation checks in the
// Bootstrap func, and that has set an agent-version (via finding the tools to,
// use for bootstrap, or otherwise).
func FinishControllerPodConfig(pcfg *ControllerPodConfig, cfg *config.Config) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot complete pod configuration")
	if err := PopulateControllerPodConfig(pcfg, cfg.Type()); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// PodLabels returns the minimum set of tags that should be set on a
// pod, if the provider supports them.
func PodLabels(modelUUID, controllerUUID string, tagger tags.ResourceTagger, jobs []multiwatcher.MachineJob) map[string]string {
	podLabels := tags.ResourceTags(
		names.NewModelTag(modelUUID),
		names.NewControllerTag(controllerUUID),
		tagger,
	)
	// always be a controller.
	podLabels[tags.JujuIsController] = "true"
	return podLabels
}
