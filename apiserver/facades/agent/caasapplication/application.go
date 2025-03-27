// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication

import (
	"context"
	"path"
	"strconv"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/agent"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ControllerConfigService defines the API methods on the ControllerState facade.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ApplicationService instances implement an application service.
type ApplicationService interface {
	RegisterCAASUnit(ctx context.Context, appName string, unit application.RegisterCAASUnitArg) error
	CAASUnitTerminating(ctx context.Context, appName string, unitNum int, broker applicationservice.Broker) (bool, error)
	GetApplicationLife(ctx context.Context, appName string) (life.Value, error)
	GetUnitLife(ctx context.Context, unitName unit.Name) (life.Value, error)
}

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model
	// does not exist.
	GetModelTargetAgentVersion(ctx context.Context) (semversion.Number, error)
}

// Facade defines the API methods on the CAASApplication facade.
type Facade struct {
	auth                    facade.Authorizer
	resources               facade.Resources
	ctrlSt                  ControllerState
	controllerConfigService ControllerConfigService
	applicationService      ApplicationService
	modelAgentService       ModelAgentService
	state                   State
	model                   Model
	clock                   clock.Clock
	broker                  Broker
	logger                  logger.Logger
}

// NewFacade returns a new CAASOperator facade.
func NewFacade(
	resources facade.Resources,
	authorizer facade.Authorizer,
	ctrlSt ControllerState,
	st State,
	controllerConfigService ControllerConfigService,
	applicationService ApplicationService,
	modelAgentService ModelAgentService,
	broker Broker,
	clock clock.Clock,
	logger logger.Logger,
) (*Facade, error) {
	if !authorizer.AuthApplicationAgent() && !authorizer.AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Facade{
		auth:                    authorizer,
		resources:               resources,
		ctrlSt:                  ctrlSt,
		state:                   st,
		controllerConfigService: controllerConfigService,
		applicationService:      applicationService,
		modelAgentService:       modelAgentService,
		model:                   model,
		clock:                   clock,
		broker:                  broker,
		logger:                  logger,
	}, nil
}

// UnitIntroduction sets the status of each given entity.
func (f *Facade) UnitIntroduction(ctx context.Context, args params.CAASUnitIntroductionArgs) (params.CAASUnitIntroductionResult, error) {
	tag, ok := f.auth.GetAuthTag().(names.ApplicationTag)
	if !ok {
		return params.CAASUnitIntroductionResult{}, apiservererrors.ErrPerm
	}

	var unitName unit.Name
	errResp := func(err error) (params.CAASUnitIntroductionResult, error) {
		f.logger.Warningf(context.TODO(), "error introducing k8s pod %q: %v", args.PodName, err)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			err = errors.NotFoundf("appliction %s", tag.Name)
		} else if errors.Is(err, applicationerrors.UnitAlreadyExists) {
			err = errors.AlreadyExistsf("unit %s", unitName)
		} else if errors.Is(err, applicationerrors.UnitNotAssigned) {
			err = errors.NotAssignedf("unit %s", unitName)
		}
		return params.CAASUnitIntroductionResult{Error: apiservererrors.ServerError(err)}, nil
	}

	if args.PodName == "" {
		return errResp(errors.NotValidf("pod-name"))
	}
	if args.PodUUID == "" {
		return errResp(errors.NotValidf("pod-uuid"))
	}

	f.logger.Debugf(context.TODO(), "introducing pod %q (%q)", args.PodName, args.PodUUID)

	appName := tag.Name
	appLife, err := f.applicationService.GetApplicationLife(ctx, appName)
	if err != nil {
		return errResp(err)
	}

	if appLife != life.Alive {
		return errResp(errors.NotProvisionedf("application"))
	}

	deploymentType := caas.DeploymentStateful

	upsert := state.UpsertCAASUnitParams{}

	containerID := args.PodName
	switch deploymentType {
	case caas.DeploymentStateful:
		splitPodName := strings.Split(args.PodName, "-")
		ord, err := strconv.Atoi(splitPodName[len(splitPodName)-1])
		if err != nil {
			return errResp(err)
		}
		unitName, err = unit.NewNameFromParts(appName, ord)
		if err != nil {
			return errResp(err)
		}
		unitNameStr := unitName.String()
		upsert.UnitName = &unitNameStr
		upsert.OrderedId = ord
		upsert.OrderedScale = true
	default:
		return errResp(errors.NotSupportedf("unknown deployment type"))
	}

	// Find the pod/unit in the provider.
	caasApp := f.broker.Application(appName, caas.DeploymentStateful)
	pods, err := caasApp.Units()
	if err != nil {
		return errResp(err)
	}
	var pod *caas.Unit
	for _, v := range pods {
		p := v
		if p.Id == args.PodName {
			pod = &p
			break
		}
	}
	if pod == nil {
		return errResp(errors.NotFoundf("pod %s in provider", args.PodName))
	}
	upsert.ProviderId = &containerID
	if pod.Address != "" {
		upsert.Address = &pod.Address
	}
	if len(pod.Ports) != 0 {
		upsert.Ports = &pod.Ports
	}
	for _, fs := range pod.FilesystemInfo {
		upsert.ObservedAttachedVolumeIDs = append(upsert.ObservedAttachedVolumeIDs, fs.Volume.VolumeId)
	}

	pass, err := password.RandomPassword()
	if err != nil {
		return errResp(err)
	}
	passwordHash := password.AgentPasswordHash(pass)
	upsert.PasswordHash = &passwordHash

	if err := f.applicationService.RegisterCAASUnit(ctx, appName, application.RegisterCAASUnitArg{
		UnitName:         unitName,
		ProviderID:       containerID,
		PasswordHash:     passwordHash,
		Address:          upsert.Address,
		Ports:            upsert.Ports,
		OrderedScale:     upsert.OrderedScale,
		OrderedId:        upsert.OrderedId,
		StorageParentDir: application.StorageParentDir,
	}); err != nil {
		return errResp(err)
	}

	// TODO(units) - remove dual write to state
	application, err := f.state.Application(tag.Name)
	if err != nil {
		return errResp(err)
	}
	_, err = application.UpsertCAASUnit(upsert)
	if err != nil {
		return errResp(err)
	}

	controllerConfig, err := f.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errResp(err)
	}
	apiHostPorts, err := f.ctrlSt.APIHostPortsForAgents(controllerConfig)
	if err != nil {
		return errResp(err)
	}
	addrs := []string(nil)
	for _, hostPorts := range apiHostPorts {
		ordered := hostPorts.HostPorts().PrioritizedForScope(network.ScopeMatchCloudLocal)
		for _, addr := range ordered {
			if addr != "" {
				addrs = append(addrs, addr)
			}
		}
	}

	// Skip checking okay on CACerts result, it will always be there
	// Method has a comment to remove the boolean return value.
	caCert, _ := controllerConfig.CACert()
	version, err := f.modelAgentService.GetModelTargetAgentVersion(ctx)
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
			Tag:               names.NewUnitTag(*upsert.UnitName),
			Controller:        f.model.ControllerTag(),
			Model:             f.model.Tag().(names.ModelTag),
			APIAddresses:      addrs,
			CACert:            caCert,
			Password:          pass,
			UpgradedToVersion: version,
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
			UnitName:  *upsert.UnitName,
			AgentConf: agentConfBytes,
		},
	}
	return res, nil
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
			err = errors.NotFoundf("application %s", tag.Id())
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
	unitName, err := unit.NewName(unitTag.Id())
	if err != nil {
		return errResp(err)
	}

	unitLife, err := f.applicationService.GetUnitLife(ctx, unitName)
	if err != nil {
		return errResp(err)
	}
	if unitLife != life.Alive {
		return params.CAASUnitTerminationResult{WillRestart: false}, nil
	}

	appName, err := names.UnitApplication(unitTag.Id())
	if err != nil {
		return errResp(err)
	}
	willRestart, err := f.applicationService.CAASUnitTerminating(ctx, appName, unitTag.Number(), f.broker)
	if err != nil {
		return errResp(err)
	}
	return params.CAASUnitTerminationResult{WillRestart: willRestart}, nil
}
