// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication

import (
	"context"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/agent"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domaincloud "github.com/juju/juju/domain/cloud"
	"github.com/juju/juju/domain/logging"
	loggingerrors "github.com/juju/juju/domain/logging/errors"
	tracingservice "github.com/juju/juju/domain/tracing/service"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/rpc/params"
)

// ControllerConfigService defines the API methods on the ControllerState facade.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ControllerNodeService represents a way to get controller api addresses.
type ControllerNodeService interface {
	// AddControllerNode ensures a controller node exists for the supplied ID.
	AddControllerNode(ctx context.Context, controllerID string) error
	// GetAllAPIAddressesForAgents returns a string of api
	// addresses available for agents ordered to prefer local-cloud scoped
	// addresses and IPv4 over IPv6 for each machine.
	GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error)
}

// ControllerService provides controller agent configuration.
type ControllerService interface {
	GetControllerAgentInfo(ctx context.Context) (controller.ControllerAgentInfo, error)
}

// AgentPasswordService manages agent passwords.
type AgentPasswordService interface {
	HasControllerNodePassword(ctx context.Context, controllerID string) (bool, error)
	SetControllerNodePasswordIfAbsent(ctx context.Context, controllerID, password string) (bool, error)
}

// ApplicationService instances implement an application service.
type ApplicationService interface {
	GetApplicationUUIDByName(ctx context.Context, name string) (coreapplication.UUID, error)
	IsControllerApplication(ctx context.Context, appUUID coreapplication.UUID) (bool, error)
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

// LokiConfigService provides access to the controller-wide Loki push API
// configuration. It is used to seed newly introduced unit agents with the
// current Loki endpoint so they start in the correct forwarding mode on
// first boot.
type LokiConfigService interface {
	// GetLokiConfig returns the configured Loki push API endpoint and CA
	// certificate. If no endpoint is configured, an error satisfying
	// [github.com/juju/juju/domain/logging/errors.LokiConfigNotFound] is
	// returned.
	GetLokiConfig(ctx context.Context) (logging.LokiConfig, error)
}

// Facade defines the API methods on the CAASApplication facade.
type Facade struct {
	controllerUUID         string
	modelUUID              coremodel.UUID
	isControllerModelScope bool

	auth                    facade.Authorizer
	controllerConfigService ControllerConfigService
	controllerNodeService   ControllerNodeService
	controllerService       ControllerService
	agentPasswordService    AgentPasswordService
	applicationService      ApplicationService
	modelAgentService       ModelAgentService
	tracingService          TracingService
	lokiConfigService       LokiConfigService
	logger                  logger.Logger
}

// NewFacade returns a new CAASOperator facade.
func NewFacade(
	authorizer facade.Authorizer,
	controllerUUID string,
	modelUUID coremodel.UUID,
	isControllerModelScope bool,
	controllerConfigService ControllerConfigService,
	controllerNodeService ControllerNodeService,
	controllerService ControllerService,
	agentPasswordService AgentPasswordService,
	applicationService ApplicationService,
	modelAgentService ModelAgentService,
	tracingService TracingService,
	lokiConfigService LokiConfigService,
	logger logger.Logger,
) *Facade {
	return &Facade{
		auth:                    authorizer,
		controllerUUID:          controllerUUID,
		modelUUID:               modelUUID,
		isControllerModelScope:  isControllerModelScope,
		controllerConfigService: controllerConfigService,
		controllerNodeService:   controllerNodeService,
		controllerService:       controllerService,
		agentPasswordService:    agentPasswordService,
		applicationService:      applicationService,
		modelAgentService:       modelAgentService,
		tracingService:          tracingService,
		lokiConfigService:       lokiConfigService,
		logger:                  logger,
	}
}

// UnitIntroduction registers a Kubernetes pod belonging to the authenticated
// application as a CAAS unit and returns the unit agent configuration needed by
// the pod's charm container. Registration assigns the Juju unit from the
// StatefulSet ordinal, records the pod details, and sets the unit password.
//
// A controller application pod hosts both the controller charm unit agent and
// a controller agent. Non-bootstrap replicas cannot reuse controller-0's
// identity, so introduction also creates a controller identity matching the
// replica ordinal and returns its controller agent configuration. This is done
// here because unit introduction is where the pod has been matched to its Juju
// unit and the init container is already receiving its local agent configs.
// Controller configuration is issued only in the controller model and only for
// the application marked as the controller application.
//
// This method mutates both model and controller state. Unit registration can
// create or update the unit and its password. Controller introduction also
// ensures the controller node exists and initializes its password. These writes
// are not one transaction, so an error can leave partial state for a later
// retry. An established controller password is never replaced by this method;
// replayed introduction is denied rather than returning replacement controller
// credentials.
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

	// TODO (stickupkid): We should stream line this into a singular call to
	// the application service, rather than having the facade orchestrate
	// multiple calls to the application service. This will allow us to have a
	// single transaction for the entire introduction process, rather than
	// having multiple transactions that can leave the model in an inconsistent
	// state.

	isControllerApplication := tag.Name == coreapplication.ControllerApplicationName
	var controllerID string
	if isControllerApplication {
		if !f.isControllerModelScope {
			return params.CAASUnitIntroductionResult{}, apiservererrors.ErrPerm
		}
		appUUID, err := f.applicationService.GetApplicationUUIDByName(ctx, tag.Name)
		if err != nil {
			return errResp(err)
		}
		isController, err := f.applicationService.IsControllerApplication(ctx, appUUID)
		if err != nil {
			return errResp(err)
		}
		if !isController {
			return params.CAASUnitIntroductionResult{}, apiservererrors.ErrPerm
		}
		controllerID, err = controllerIDFromPodName(args.PodName)
		if err != nil {
			return errResp(err)
		}
		passwordSet, err := f.agentPasswordService.HasControllerNodePassword(ctx, controllerID)
		if err != nil {
			return errResp(err)
		}
		if passwordSet {
			return params.CAASUnitIntroductionResult{}, apiservererrors.ErrPerm
		}
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
	// Fetch the controller-wide Loki config so the unit agent starts in the
	// correct forwarding mode on first boot. When Loki is not active the
	// config is empty and the agent falls back to logsink mode.
	lokiConfig, err := f.lokiConfigService.GetLokiConfig(ctx)
	if err != nil && !errors.Is(err, loggingerrors.LokiConfigNotFound) {
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

			OpenTelemetryEnabled:               tracingConfig.GRPCEndpoint != "" || tracingConfig.HTTPEndpoint != "",
			OpenTelemetryHTTPEndpoint:          tracingConfig.HTTPEndpoint,
			OpenTelemetryGRPCEndpoint:          tracingConfig.GRPCEndpoint,
			OpenTelemetryInsecure:              openTelemetryInsecure(tracingConfig),
			OpenTelemetryStackTraces:           openTelemetryStackTraces(tracingConfig),
			OpenTelemetrySampleRatio:           openTelemetrySampleRatio(tracingConfig),
			OpenTelemetryTailSamplingThreshold: openTelemetryTailSamplingThreshold,

			LokiEndpoint:           lokiConfig.Endpoint,
			LokiCACert:             lokiConfig.CACertificate,
			LokiInsecureSkipVerify: lokiConfig.InsecureSkipVerify,
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

	// If the application is not the controller application, we don't need to
	// generate a controller agent config, so we can return early.
	if !isControllerApplication {
		return res, nil
	}

	if controllerID != strconv.Itoa(unitName.Number()) {
		return errResp(errors.NotValidf("controller pod name %q", args.PodName))
	}
	controllerPassword, err := password.RandomPassword()
	if err != nil {
		return errResp(errors.Annotate(err, "generating controller agent password"))
	}
	controllerAgentInfo, err := f.controllerService.GetControllerAgentInfo(ctx)
	if err != nil {
		return errResp(errors.Annotate(err, "getting controller agent info"))
	}
	controllerConf, err := agent.NewStateMachineConfig(agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir: dataDir,
			LogDir:  logDir,
		},
		Tag:               names.NewControllerAgentTag(controllerID),
		Controller:        names.NewControllerTag(f.controllerUUID),
		Model:             names.NewModelTag(f.modelUUID.String()),
		APIAddresses:      addrs,
		CACert:            caCert,
		Password:          controllerPassword,
		UpgradedToVersion: version,
		Values: map[string]string{
			agent.ProviderType: domaincloud.CloudTypeKubernetes.String(),
		},
		QueryTracingEnabled:                controllerConfig.QueryTracingEnabled(),
		QueryTracingThreshold:              controllerConfig.QueryTracingThreshold(),
		DqliteBusyTimeout:                  controllerConfig.DqliteBusyTimeout(),
		OpenTelemetryEnabled:               tracingConfig.GRPCEndpoint != "" || tracingConfig.HTTPEndpoint != "",
		OpenTelemetryHTTPEndpoint:          tracingConfig.HTTPEndpoint,
		OpenTelemetryGRPCEndpoint:          tracingConfig.GRPCEndpoint,
		OpenTelemetryInsecure:              openTelemetryInsecure(tracingConfig),
		OpenTelemetryStackTraces:           openTelemetryStackTraces(tracingConfig),
		OpenTelemetrySampleRatio:           openTelemetrySampleRatio(tracingConfig),
		OpenTelemetryTailSamplingThreshold: openTelemetryTailSamplingThreshold,
		LokiEndpoint:                       lokiConfig.Endpoint,
		LokiCACert:                         lokiConfig.CACertificate,
		LokiInsecureSkipVerify:             lokiConfig.InsecureSkipVerify,
	}, controllerAgentInfo)
	if err != nil {
		return errResp(errors.Annotate(err, "creating controller agent config"))
	}
	controllerConfBytes, err := controllerConf.Render()
	if err != nil {
		return errResp(errors.Annotate(err, "rendering controller agent config"))
	}
	if err := f.controllerNodeService.AddControllerNode(ctx, controllerID); err != nil {
		return errResp(err)
	}
	// The application password is shared by controller pods and remains valid
	// after introduction. Never overwrite an existing controller password here:
	// anyone retaining that shared credential could replay UnitIntroduction,
	// rotate a live controller's credentials, and obtain its replacement config.
	// Insert-if-absent is the final atomic guard against replayed or concurrent
	// requests.
	passwordSet, err := f.agentPasswordService.SetControllerNodePasswordIfAbsent(ctx, controllerID, controllerPassword)
	if err != nil {
		return errResp(err)
	}
	if !passwordSet {
		return params.CAASUnitIntroductionResult{}, apiservererrors.ErrPerm
	}
	res.Result.ControllerAgentTag = names.NewControllerAgentTag(controllerID).String()
	res.Result.ControllerAgentConf = controllerConfBytes

	return res, nil
}

func controllerIDFromPodName(podName string) (string, error) {
	id, ok := strings.CutPrefix(podName, coreapplication.ControllerApplicationName+"-")
	if !ok {
		return "", errors.NotValidf("controller pod name %q", podName)
	}
	number, err := strconv.Atoi(id)
	if err != nil || number < 0 {
		return "", errors.NotValidf("controller pod name %q", podName)
	}
	return strconv.Itoa(number), nil
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
