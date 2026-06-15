// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication

import (
	"context"
	"path"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/agent"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	tracingservice "github.com/juju/juju/domain/tracing/service"
	"github.com/juju/juju/rpc/params"
)

// ControllerConfigService defines the API methods on the ControllerState facade.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ControllerNodeService represents a way to get controller api addresses.
type ControllerNodeService interface {
	// GetAllAPIAddressesForAgents returns a string of api
	// addresses available for agents ordered to prefer local-cloud scoped
	// addresses and IPv4 over IPv6 for each machine.
	GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error)
}

// ApplicationService instances implement an application service.
type ApplicationService interface {
	RegisterCAASUnit(ctx context.Context, params application.RegisterCAASUnitParams) (unit.Name, string, error)
	CAASUnitTerminating(ctx context.Context, unitName string) (bool, error)
}

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model
	// does not exist.
	GetModelTargetAgentVersion(ctx context.Context) (semversion.Number, error)
}

// TracingService provides access to the workload tracing configuration.
type TracingService interface {
	// GetWorkloadTracingConfig returns the workload tracing config from the
	// state.
	GetWorkloadTracingConfig(ctx context.Context) (tracingservice.WorkloadTracingConfig, error)
}

// Facade defines the API methods on the CAASApplication facade.
type Facade struct {
	controllerUUID string
	modelUUID      coremodel.UUID

	auth                    facade.Authorizer
	controllerConfigService ControllerConfigService
	controllerNodeService   ControllerNodeService
	applicationService      ApplicationService
	modelAgentService       ModelAgentService
	tracingService          TracingService
	logger                  logger.Logger
}

// NewFacade returns a new CAASOperator facade.
func NewFacade(
	authorizer facade.Authorizer,
	controllerUUID string,
	modelUUID coremodel.UUID,
	controllerConfigService ControllerConfigService,
	controllerNodeService ControllerNodeService,
	applicationService ApplicationService,
	modelAgentService ModelAgentService,
	tracingService TracingService,
	logger logger.Logger,
) *Facade {
	return &Facade{
		auth:                    authorizer,
		controllerUUID:          controllerUUID,
		modelUUID:               modelUUID,
		controllerConfigService: controllerConfigService,
		controllerNodeService:   controllerNodeService,
		applicationService:      applicationService,
		modelAgentService:       modelAgentService,
		tracingService:          tracingService,
		logger:                  logger,
	}
}

// UnitIntroduction sets the status of each given unit.
func (f *Facade) UnitIntroduction(ctx context.Context, args params.CAASUnitIntroductionArgs) (params.CAASUnitIntroductionResult, error) {
	tag, ok := f.auth.GetAuthTag().(names.ApplicationTag)
	if !ok {
		return params.CAASUnitIntroductionResult{}, apiservererrors.ErrPerm
	}

	errResp := func(err error) (params.CAASUnitIntroductionResult, error) {
		f.logger.Warningf(ctx, "error introducing k8s pod %q: %v", args.PodName, err)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			err = errors.NotFoundf("application %s", tag.Name)
		} else if errors.Is(err, applicationerrors.ApplicationNotAlive) {
			err = errors.NotProvisionedf("application %s", tag.Name)
		} else if errors.Is(err, applicationerrors.UnitAlreadyExists) {
			err = errors.AlreadyExistsf("unit for pod %s", args.PodName)
		} else if errors.Is(err, applicationerrors.UnitNotAssigned) {
			err = errors.NotAssignedf("unit for pod %s", args.PodName)
		}
		return params.CAASUnitIntroductionResult{Error: apiservererrors.ServerError(err)}, nil
	}

	if args.PodName == "" {
		return errResp(errors.NotValidf("pod-name"))
	}
	if args.PodUUID == "" {
		return errResp(errors.NotValidf("pod-uuid"))
	}

	f.logger.Debugf(ctx, "introducing pod %q (%q)", args.PodName, args.PodUUID)

	registerArgs := application.RegisterCAASUnitParams{
		ApplicationName: tag.Name,
		ProviderID:      args.PodName,
	}
	unitName, unitPassword, err := f.applicationService.RegisterCAASUnit(ctx, registerArgs)
	if err != nil {
		return errResp(err)
	}

	addrs, err := f.controllerNodeService.GetAllAPIAddressesForAgents(ctx)
	if err != nil {
		return errResp(err)
	}

	controllerConfig, err := f.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errResp(err)
	}
	// Skip checking okay on CACerts result, it will always be there
	// Method has a comment to remove the boolean return value.
	caCert, _ := controllerConfig.CACert()
	version, err := f.modelAgentService.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return errResp(err)
	}
	tracingConfig, err := f.tracingService.GetWorkloadTracingConfig(ctx)
	if err != nil {
		return errResp(err)
	}
	openTelemetryTailSamplingThreshold, err := openTelemetryTailSamplingThreshold(tracingConfig)
	if err != nil {
		return errResp(err)
	}
	dataDir := paths.DataDir(paths.OSUnixLike)
	logDir := path.Join(paths.LogDir(paths.OSUnixLike), "juju")
	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir: dataDir,
				LogDir:  logDir,
			},
			Tag:               names.NewUnitTag(unitName.String()),
			Controller:        names.NewControllerTag(f.controllerUUID),
			Model:             names.NewModelTag(f.modelUUID.String()),
			APIAddresses:      addrs,
			CACert:            caCert,
			Password:          unitPassword,
			UpgradedToVersion: version,

			OpenTelemetryEnabled:               tracingConfig.GRPCEndpoint != "",
			OpenTelemetryEndpoint:              tracingConfig.GRPCEndpoint,
			OpenTelemetryInsecure:              openTelemetryInsecure(tracingConfig),
			OpenTelemetryStackTraces:           openTelemetryStackTraces(tracingConfig),
			OpenTelemetrySampleRatio:           openTelemetrySampleRatio(tracingConfig),
			OpenTelemetryTailSamplingThreshold: openTelemetryTailSamplingThreshold,
		},
	)
	if err != nil {
		return errResp(errors.Annotatef(err, "creating new agent config"))
	}
	agentConfBytes, err := conf.Render()
	if err != nil {
		return errResp(err)
	}

	res := params.CAASUnitIntroductionResult{
		Result: &params.CAASUnitIntroduction{
			UnitName:  unitName.String(),
			AgentConf: agentConfBytes,
		},
	}
	return res, nil
}

func openTelemetryInsecure(config tracingservice.WorkloadTracingConfig) bool {
	if config.InsecureSkipVerify == nil {
		return agent.DefaultOpenTelemetryInsecure
	}
	return *config.InsecureSkipVerify
}

func openTelemetryStackTraces(config tracingservice.WorkloadTracingConfig) bool {
	if config.OpenTelemetryStackTraces == nil {
		return agent.DefaultOpenTelemetryStackTraces
	}
	return *config.OpenTelemetryStackTraces
}

func openTelemetrySampleRatio(config tracingservice.WorkloadTracingConfig) float64 {
	if config.OpenTelemetrySampleRatio == nil {
		return agent.DefaultOpenTelemetrySampleRatio
	}
	return *config.OpenTelemetrySampleRatio
}

func openTelemetryTailSamplingThreshold(config tracingservice.WorkloadTracingConfig) (time.Duration, error) {
	if config.OpenTelemetryTailSamplingThreshold == nil {
		return agent.DefaultOpenTelemetryTailSamplingThreshold, nil
	}
	threshold, err := time.ParseDuration(*config.OpenTelemetryTailSamplingThreshold)
	if err != nil {
		return 0, errors.Annotatef(err, "parsing open telemetry tail sampling threshold")
	}
	return threshold, nil
}

// UnitTerminating should be called by the CAASUnitTerminationWorker when
// the agent receives a signal to exit. UnitTerminating will return how
// the agent should shutdown.
func (f *Facade) UnitTerminating(ctx context.Context, args params.Entity) (params.CAASUnitTerminationResult, error) {
	tag, ok := f.auth.GetAuthTag().(names.UnitTag)
	if !ok {
		return params.CAASUnitTerminationResult{}, apiservererrors.ErrPerm
	}

	errResp := func(err error) (params.CAASUnitTerminationResult, error) {
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			err = errors.NotFoundf("application for unit %s", tag.Id())
		} else if errors.Is(err, applicationerrors.UnitNotFound) {
			err = errors.NotFoundf("unit %s", tag.Id())
		}
		return params.CAASUnitTerminationResult{Error: apiservererrors.ServerError(err)}, nil
	}

	unitTag, err := names.ParseUnitTag(args.Tag)
	if err != nil {
		return errResp(err)
	}
	if unitTag != tag {
		return params.CAASUnitTerminationResult{}, apiservererrors.ErrPerm
	}
	willRestart, err := f.applicationService.CAASUnitTerminating(ctx, unitTag.Id())
	if err != nil {
		return errResp(err)
	}
	return params.CAASUnitTerminationResult{WillRestart: willRestart}, nil
}
