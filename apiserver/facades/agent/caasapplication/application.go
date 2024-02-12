// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	"github.com/juju/juju/agent"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/paths"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ControllerConfigService defines the API methods on the ControllerState facade.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ApplicationSaver instances save an application to dqlite state.
type ApplicationSaver interface {
	Save(ctx context.Context, name string, units ...applicationservice.AddUnitParams) error
}

// Facade defines the API methods on the CAASApplication facade.
type Facade struct {
	auth                    facade.Authorizer
	resources               facade.Resources
	ctrlSt                  ControllerState
	controllerConfigService ControllerConfigService
	applicationSaver        ApplicationSaver
	state                   State
	model                   Model
	clock                   clock.Clock
	broker                  Broker
	logger                  loggo.Logger
}

// NewFacade returns a new CAASOperator facade.
func NewFacade(
	resources facade.Resources,
	authorizer facade.Authorizer,
	ctrlSt ControllerState,
	st State,
	controllerConfigService ControllerConfigService,
	applicationSaver ApplicationSaver,
	broker Broker,
	clock clock.Clock,
	logger loggo.Logger,
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
		applicationSaver:        applicationSaver,
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

	errResp := func(err error) (params.CAASUnitIntroductionResult, error) {
		return params.CAASUnitIntroductionResult{Error: apiservererrors.ServerError(err)}, nil
	}

	if args.PodName == "" {
		return errResp(errors.NotValidf("pod-name"))
	}
	if args.PodUUID == "" {
		return errResp(errors.NotValidf("pod-uuid"))
	}

	f.logger.Debugf("introducing pod %q (%q)", args.PodName, args.PodUUID)

	application, err := f.state.Application(tag.Name)
	if err != nil {
		return errResp(err)
	}

	if application.Life() != state.Alive {
		return errResp(errors.NotProvisionedf("application"))
	}

	// TODO(sidecar): handle deployment other than statefulset
	// ch, _, err := application.Charm()
	// if err != nil {
	// 	return errResp(err)
	// }
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
		n := fmt.Sprintf("%s/%d", application.Name(), ord)
		upsert.UnitName = &n
		upsert.OrderedId = ord
		upsert.OrderedScale = true
	default:
		return errResp(errors.NotSupportedf("unknown deployment type"))
	}

	// Find the pod/unit in the provider.
	caasApp := f.broker.Application(application.Name(), caas.DeploymentStateful)
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

	unit, err := application.UpsertCAASUnit(upsert)
	if err != nil {
		return errResp(err)
	}
	if err := f.applicationSaver.Save(ctx, application.Name(), applicationservice.AddUnitParams{UnitName: upsert.UnitName}); err != nil {
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

	caCert, _ := controllerConfig.CACert()
	version, _ := f.model.AgentVersion()
	dataDir := paths.DataDir(paths.OSUnixLike)
	logDir := path.Join(paths.LogDir(paths.OSUnixLike), "juju")
	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir: dataDir,
				LogDir:  logDir,
			},
			Tag:               unit.Tag(),
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
			UnitName:  unit.Tag().Id(),
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
		return params.CAASUnitTerminationResult{Error: apiservererrors.ServerError(err)}, nil
	}

	unitTag, err := names.ParseUnitTag(args.Tag)
	if err != nil {
		return errResp(err)
	}
	if unitTag != tag {
		return params.CAASUnitTerminationResult{}, apiservererrors.ErrPerm
	}

	unit, err := f.state.Unit(unitTag.Id())
	if err != nil {
		return errResp(err)
	}
	if unit.Life() != state.Alive {
		return params.CAASUnitTerminationResult{WillRestart: false}, nil
	}

	// TODO(sidecar): handle deployment other than statefulset
	deploymentType := caas.DeploymentStateful
	restart := true

	switch deploymentType {
	case caas.DeploymentStateful:
		application, err := f.state.Application(unit.ApplicationName())
		if err != nil {
			return errResp(err)
		}
		caasApp := f.broker.Application(unit.ApplicationName(), caas.DeploymentStateful)
		appState, err := caasApp.State()
		if err != nil {
			return errResp(err)
		}
		n := unitTag.Number()
		if n >= application.GetScale() || n >= appState.DesiredReplicas {
			restart = false
		}
	case caas.DeploymentStateless, caas.DeploymentDaemon:
		// Both handled the same way.
		restart = true
	default:
		return errResp(errors.NotSupportedf("unknown deployment type"))
	}

	return params.CAASUnitTerminationResult{
		WillRestart: restart,
	}, nil
}
