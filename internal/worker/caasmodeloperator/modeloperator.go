// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/watcher"
	tracingservice "github.com/juju/juju/domain/tracing/service"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/password"
)

type ModelOperatorAPI interface {
	SetPassword(ctx context.Context, password string) error
	ModelOperatorProvisioningInfo(context.Context) (ModelOperatorProvisioningInfo, error)
	WatchModelOperatorProvisioningInfo(context.Context) (watcher.NotifyWatcher, error)
}

// ModelOperatorBroker describes the caas broker interface needed for installing
// a ModelOperator into Kubernetes
type ModelOperatorBroker interface {
	EnsureModelOperator(context.Context, string, string, *caas.ModelOperatorConfig) error
	ModelOperator(ctx context.Context) (*caas.ModelOperatorConfig, error)
	ModelOperatorExists(ctx context.Context) (bool, error)
	GetModelOperatorDeploymentImage(ctx context.Context) (string, error)
}

// ModelOperatorProvisioningInfo represents return api information for
// provisioning a caas model operator
type ModelOperatorProvisioningInfo struct {
	APIAddresses         []string
	ImageDetails         resource.DockerImageDetails
	Version              semversion.Number
	ControllerCert       string
	ControllerPrivateKey string
	CAPrivateKey         string
}

// ModelOperatorManager defines the worker used for managing model operators in
// caas
type ModelOperatorManager struct {
	dataDir        string
	logDir         string
	controllerTag  names.ControllerTag
	configProvider ConfigProvider
	tracingService TracingService
	api            ModelOperatorAPI
	broker         ModelOperatorBroker
	catacomb       catacomb.Catacomb
	logger         logger.Logger
	modelUUID      string
}

const (
	// DefaultModelOperatorPort is the default port used for the api server on
	// the model operator
	DefaultModelOperatorPort = 17071
)

// Kill implements worker kill method
func (m *ModelOperatorManager) Kill() {
	m.catacomb.Kill(nil)
}

// Wait implements worker Wait method
func (m *ModelOperatorManager) Wait() error {
	return m.catacomb.Wait()
}

func (m *ModelOperatorManager) loop() error {
	ctx, cancel := m.scopedContext()
	defer cancel()

	shortModelID := m.modelUUID
	if names.IsValidModel(m.modelUUID) {
		shortModelID = names.NewModelTag(m.modelUUID).ShortId()
	}
	w, err := m.api.WatchModelOperatorProvisioningInfo(ctx)
	if err != nil {
		return errors.Annotatef(err, "cannot watch model operator [%s] provisioning info", shortModelID)
	}
	err = m.catacomb.Add(w)
	if err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-m.catacomb.Dying():
			return m.catacomb.ErrDying()
		case <-w.Changes():
			err := m.update(ctx)
			if err != nil {
				return errors.Annotatef(err, "failed to update model operator [%s]", shortModelID)
			}
		}
	}
}

func (m *ModelOperatorManager) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return m.catacomb.Context(ctx), cancel
}

func (m *ModelOperatorManager) update(ctx context.Context) error {
	m.logger.Debugf(ctx, "gathering model operator provisioning information for model %s", m.modelUUID)
	info, err := m.api.ModelOperatorProvisioningInfo(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	exists, err := m.broker.ModelOperatorExists(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	setPassword := true
	pwd, err := password.RandomPassword()
	if err != nil {
		return errors.Trace(err)
	}
	if exists {
		mo, err := m.broker.ModelOperator(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		prevConf, err := agent.ParseConfigData(mo.AgentConf)
		if err != nil {
			return errors.Annotate(err, "cannot parse model operator agent config: ")
		}
		// reuse old password
		if prevInfo, ok := prevConf.APIInfo(); ok && prevInfo.Password != "" {
			pwd = prevInfo.Password
			setPassword = false
		} else if prevConf.OldPassword() != "" {
			pwd = prevConf.OldPassword()
			setPassword = false
		}

		// retrieves model operator deployment image to keep model operator's image the same after migration
		modelImage, err := m.broker.GetModelOperatorDeploymentImage(ctx)
		if err != nil {
			return errors.Annotate(err, "failed to get model deployment image")
		}

		modelImageRepo, err := podcfg.RecoverRepoFromOperatorPath(modelImage)
		if err != nil {
			return errors.Trace(err)
		}

		registryPath, err := podcfg.GetJujuOCIImagePathFromModelRepo(modelImageRepo, info.Version)
		if err != nil {
			return errors.Trace(err)
		}

		info.ImageDetails.RegistryPath = registryPath

	}
	if setPassword {
		err := m.api.SetPassword(ctx, pwd)
		if err != nil {
			return errors.Annotate(err, "failed to set model api passwords")
		}
	}

	agentConfBuf, err := m.updateAgentConf(info, pwd)
	if err != nil {
		return errors.Trace(err)
	}

	m.logger.Debugf(ctx, "ensuring model operator deployment in kubernetes for model %s", m.modelUUID)
	err = m.broker.EnsureModelOperator(
		ctx,
		m.modelUUID,
		m.dataDir,
		&caas.ModelOperatorConfig{
			AgentConf:    agentConfBuf,
			ImageDetails: info.ImageDetails,
			Port:         DefaultModelOperatorPort,
		},
	)
	if err != nil {
		return errors.Annotate(err, "deploying model operator")
	}

	return nil
}

// NewModelOperatorManager constructs a new model operator manager worker
func NewModelOperatorManager(
	logger logger.Logger,
	api ModelOperatorAPI,
	broker ModelOperatorBroker,
	modelUUID string,
	dataDir string,
	logDir string,
	controllerTag names.ControllerTag,
	configProvider ConfigProvider,
	tracingService TracingService,
) (*ModelOperatorManager, error) {
	m := &ModelOperatorManager{
		dataDir:        dataDir,
		logDir:         logDir,
		controllerTag:  controllerTag,
		configProvider: configProvider,
		tracingService: tracingService,
		api:            api,
		broker:         broker,
		logger:         logger,
		modelUUID:      modelUUID,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "caas-model-operator-manager",
		Site: &m.catacomb,
		Work: m.loop,
	}); err != nil {
		return m, errors.Trace(err)
	}

	return m, nil
}

func (m *ModelOperatorManager) updateAgentConf(
	info ModelOperatorProvisioningInfo,
	password string,
) ([]byte, error) {
	modelTag := names.NewModelTag(m.modelUUID)
	caCert, err := m.configProvider.CACert()
	if err != nil {
		return nil, errors.Annotate(err, "reading CA cert")
	}
	tracingConfig, err := m.tracingService.GetWorkloadTracingConfig(context.Background())
	if err != nil {
		return nil, errors.Annotate(err, "reading workload tracing config")
	}
	runtimeTracingCfg, err := runtimeConfigFromWorkloadTracingConfig(tracingConfig)
	if err != nil {
		return nil, errors.Annotate(err, "building workload tracing config")
	}
	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir: m.dataDir,
				LogDir:  m.logDir,
			},
			Tag:          modelTag,
			Controller:   m.controllerTag,
			Model:        modelTag,
			APIAddresses: info.APIAddresses,
			CACert:       caCert,
			Password:     password,

			UpgradedToVersion: info.Version,

			OpenTelemetryEnabled:               runtimeTracingCfg.Enabled,
			OpenTelemetryEndpoint:              runtimeTracingCfg.Endpoint,
			OpenTelemetryInsecure:              runtimeTracingCfg.InsecureSkipVerify,
			OpenTelemetryStackTraces:           runtimeTracingCfg.StackTracesEnabled,
			OpenTelemetrySampleRatio:           runtimeTracingCfg.SampleRatio,
			OpenTelemetryTailSamplingThreshold: runtimeTracingCfg.TailSamplingThreshold,
		},
	)
	if err != nil {
		return nil, errors.Annotatef(err, "creating new agent config for model")
	}
	conf.SetControllerAgentInfo(controller.ControllerAgentInfo{
		Cert:         info.ControllerCert,
		PrivateKey:   info.ControllerPrivateKey,
		CAPrivateKey: info.CAPrivateKey,
	})

	return conf.Render()
}

const (
	defaultOpenTelemetrySampleRatio           = 0.1
	defaultOpenTelemetryTailSamplingThreshold = time.Millisecond
)

type runtimeTracingConfig struct {
	Enabled               bool
	Endpoint              string
	CACertificate         string
	InsecureSkipVerify    bool
	StackTracesEnabled    bool
	SampleRatio           float64
	TailSamplingThreshold time.Duration
}

func runtimeConfigFromWorkloadTracingConfig(cfg tracingservice.WorkloadTracingConfig) (runtimeTracingConfig, error) {
	runtimeCfg := runtimeTracingConfig{
		SampleRatio:           defaultOpenTelemetrySampleRatio,
		TailSamplingThreshold: defaultOpenTelemetryTailSamplingThreshold,
		CACertificate:         cfg.CACertificate,
	}

	endpoint := cfg.GRPCEndpoint
	if endpoint == "" {
		endpoint = cfg.HTTPEndpoint
	}
	if endpoint != "" {
		runtimeCfg.Enabled = true
		runtimeCfg.Endpoint = endpoint
	}

	if cfg.OpenTelemetryStackTraces != nil {
		runtimeCfg.StackTracesEnabled = *cfg.OpenTelemetryStackTraces
	}

	if cfg.InsecureSkipVerify != nil {
		runtimeCfg.InsecureSkipVerify = *cfg.InsecureSkipVerify
	}

	if cfg.OpenTelemetrySampleRatio != nil {
		if *cfg.OpenTelemetrySampleRatio < 0 || *cfg.OpenTelemetrySampleRatio > 1 {
			return runtimeTracingConfig{}, errors.NotValidf("open telemetry sample ratio %.4f", *cfg.OpenTelemetrySampleRatio)
		}
		runtimeCfg.SampleRatio = *cfg.OpenTelemetrySampleRatio
	}

	if cfg.OpenTelemetryTailSamplingThreshold != nil {
		d, err := time.ParseDuration(*cfg.OpenTelemetryTailSamplingThreshold)
		if err != nil {
			return runtimeTracingConfig{}, errors.Annotatef(err, "parsing open telemetry tail sampling threshold %q", *cfg.OpenTelemetryTailSamplingThreshold)
		}
		if d < 0 {
			return runtimeTracingConfig{}, errors.NotValidf("open telemetry tail sampling threshold %q", *cfg.OpenTelemetryTailSamplingThreshold)
		}
		runtimeCfg.TailSamplingThreshold = d
	}

	return runtimeCfg, nil
}
