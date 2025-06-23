// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"
	"strconv"

	"github.com/juju/collections/set"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// ControllerConfigService is an interface that provides access to the
// controller configuration.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	ModelConfig(ctx context.Context) (*config.Config, error)
	Watch() (watcher.StringsWatcher, error)
}

// FirewallerAPI provides access to the Firewaller API facade.
type FirewallerAPI struct {
	*common.LifeGetter
	*commonmodel.ModelConfigWatcher
	*commonmodel.ModelMachinesWatcher
	*common.InstanceIdGetter
	ControllerConfigAPI

	st                                       State
	networkService                           NetworkService
	applicationService                       ApplicationService
	machineService                           MachineService
	resources                                facade.Resources
	watcherRegistry                          facade.WatcherRegistry
	authorizer                               facade.Authorizer
	accessUnit                               common.GetAuthFunc
	accessApplication                        common.GetAuthFunc
	accessMachine                            common.GetAuthFunc
	accessUnitApplicationOrMachineOrRelation common.GetAuthFunc
	logger                                   corelogger.Logger

	controllerConfigService ControllerConfigService
	modelConfigService      ModelConfigService
	modelInfoService        ModelInfoService
}

// NewStateFirewallerAPI creates a new server-side FirewallerAPIV7 facade.
func NewStateFirewallerAPI(
	st State,
	networkService NetworkService,
	resources facade.Resources,
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	controllerConfigAPI ControllerConfigAPI,
	controllerConfigService ControllerConfigService,
	modelConfigService ModelConfigService,
	applicationService ApplicationService,
	machineService MachineService,
	modelInfoService ModelInfoService,
	logger corelogger.Logger,
) (*FirewallerAPI, error) {
	if !authorizer.AuthController() {
		// Firewaller must run as a controller.
		return nil, apiservererrors.ErrPerm
	}
	// Set up the various authorization checkers.
	accessUnit := common.AuthFuncForTagKind(names.UnitTagKind)
	accessMachine := common.AuthFuncForTagKind(names.MachineTagKind)
	accessRelation := common.AuthFuncForTagKind(names.RelationTagKind)
	accessUnitApplicationOrMachineOrRelation := common.AuthAny(accessUnit, accessMachine, accessRelation)

	// Life() is supported for units, applications or machines.
	lifeGetter := common.NewLifeGetter(
		applicationService,
		machineService,
		st,
		accessUnitApplicationOrMachineOrRelation,
		logger,
	)
	// ModelConfig() and WatchForModelConfigChanges() are allowed
	// with unrestricted access.
	modelConfigWatcher := commonmodel.NewModelConfigWatcher(
		modelConfigService,
		watcherRegistry,
	)
	// WatchModelMachines() is allowed with unrestricted access.
	machinesWatcher := commonmodel.NewModelMachinesWatcher(
		st,
		machineService,
		watcherRegistry,
		authorizer,
	)
	// InstanceId() is supported for machines.
	instanceIdGetter := common.NewInstanceIdGetter(
		machineService,
		accessMachine,
	)

	return &FirewallerAPI{
		LifeGetter:                               lifeGetter,
		ModelConfigWatcher:                       modelConfigWatcher,
		ModelMachinesWatcher:                     machinesWatcher,
		InstanceIdGetter:                         instanceIdGetter,
		ControllerConfigAPI:                      controllerConfigAPI,
		st:                                       st,
		resources:                                resources,
		watcherRegistry:                          watcherRegistry,
		authorizer:                               authorizer,
		accessUnit:                               accessUnit,
		accessMachine:                            accessMachine,
		accessUnitApplicationOrMachineOrRelation: accessUnitApplicationOrMachineOrRelation,
		controllerConfigService:                  controllerConfigService,
		modelConfigService:                       modelConfigService,
		networkService:                           networkService,
		applicationService:                       applicationService,
		machineService:                           machineService,
		modelInfoService:                         modelInfoService,
		logger:                                   logger,
	}, nil
}

// Life returns the life status of the specified entities.
func (f *FirewallerAPI) Life(ctx context.Context, args params.Entities) (params.LifeResults, error) {
	result := params.LifeResults{
		Results: make([]params.LifeResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canRead, err := f.accessUnitApplicationOrMachineOrRelation(ctx)
	if err != nil {
		return params.LifeResults{}, errors.Errorf("getting auth function: %w", err)
	}
	// Entities will be machine, relation, or unit.
	// For units, we use the domain application service.
	// The other entity types are not ported across to dqlite yet.
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canRead(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		var lifeValue life.Value
		switch tag.Kind() {
		case names.UnitTagKind:
			var unitName coreunit.Name
			unitName, err = coreunit.NewName(tag.Id())
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			lifeValue, err = f.applicationService.GetUnitLife(ctx, unitName)
			if errors.Is(err, applicationerrors.UnitNotFound) {
				err = jujuerrors.NotFoundf("unit %q", unitName)
			}
		default:
			lifeValue, err = f.LifeGetter.OneLife(ctx, tag)
		}
		result.Results[i].Life = lifeValue
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// ModelFirewallRules returns the firewall rules that this model is
// configured to open
func (f *FirewallerAPI) ModelFirewallRules(ctx context.Context) (params.IngressRulesResult, error) {
	cfg, err := f.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return params.IngressRulesResult{Error: apiservererrors.ServerError(err)}, nil
	}
	ctrlCfg, err := f.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return params.IngressRulesResult{Error: apiservererrors.ServerError(err)}, nil
	}

	isController, err := f.modelInfoService.IsControllerModel(ctx)
	if err != nil {
		return params.IngressRulesResult{Error: apiservererrors.ServerError(err)}, nil
	}

	var rules []params.IngressRule
	sshAllow := cfg.SSHAllow()
	if len(sshAllow) != 0 {
		portRange := params.FromNetworkPortRange(network.MustParsePortRange("22"))
		rules = append(rules, params.IngressRule{PortRange: portRange, SourceCIDRs: sshAllow})
	}
	if isController {
		portRange := params.FromNetworkPortRange(network.MustParsePortRange(strconv.Itoa(ctrlCfg.APIPort())))
		rules = append(rules, params.IngressRule{PortRange: portRange, SourceCIDRs: []string{"0.0.0.0/0", "::/0"}})
	}
	if isController && ctrlCfg.AutocertDNSName() != "" {
		portRange := params.FromNetworkPortRange(network.MustParsePortRange("80"))
		rules = append(rules, params.IngressRule{PortRange: portRange, SourceCIDRs: []string{"0.0.0.0/0", "::/0"}})
	}
	return params.IngressRulesResult{
		Rules: rules,
	}, nil
}

// WatchModelFirewallRules returns a NotifyWatcher that notifies of
// potential changes to a model's configured firewall rules
func (f *FirewallerAPI) WatchModelFirewallRules(ctx context.Context) (params.NotifyWatchResult, error) {
	watch, err := NewModelFirewallRulesWatcher(f.modelConfigService)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}
	watcherId, _, err := internal.EnsureRegisterWatcher[struct{}](ctx, f.watcherRegistry, watch)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.NotifyWatchResult{NotifyWatcherId: watcherId}, nil
}

// WatchEgressAddressesForRelations creates a watcher that notifies when addresses, from which
// connections will originate for the relation, change.
// Each event contains the entire set of addresses which are required for ingress for the relation.
func (f *FirewallerAPI) WatchEgressAddressesForRelations(ctx context.Context, relations params.Entities) (params.StringsWatchResults, error) {
	return params.StringsWatchResults{}, nil
}

// WatchIngressAddressesForRelations creates a watcher that returns the ingress networks
// that have been recorded against the specified relations.
func (f *FirewallerAPI) WatchIngressAddressesForRelations(ctx context.Context, relations params.Entities) (params.StringsWatchResults, error) {
	return params.StringsWatchResults{}, nil
}

// MacaroonForRelations returns the macaroon for the specified relations.
func (f *FirewallerAPI) MacaroonForRelations(ctx context.Context, args params.Entities) (params.MacaroonResults, error) {
	var result params.MacaroonResults
	result.Results = make([]params.MacaroonResult, len(args.Entities))
	for i, entity := range args.Entities {
		relationTag, err := names.ParseRelationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		mac, err := f.st.GetMacaroon(relationTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = mac
	}
	return result, nil
}

// SetRelationsStatus sets the status for the specified relations.
func (f *FirewallerAPI) SetRelationsStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	var result params.ErrorResults
	result.Results = make([]params.ErrorResult, len(args.Entities))
	for i, entity := range args.Entities {
		relationTag, err := names.ParseRelationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		rel, err := f.st.KeyRelation(relationTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = rel.SetStatus(status.StatusInfo{
			Status:  status.Status(entity.Status),
			Message: entity.Info,
		})
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// AreManuallyProvisioned returns whether each given entity is
// manually provisioned or not. Only machine tags are accepted.
func (f *FirewallerAPI) AreManuallyProvisioned(ctx context.Context, args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	canAccess, err := f.accessMachine(ctx)
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		machineTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if !canAccess(machineTag) {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		manual, err := f.machineService.IsMachineManuallyProvisioned(ctx, machine.Name(machineTag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(jujuerrors.NotFoundf("machine %q", machineTag.Id()))
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = manual
	}
	return result, nil
}

// GetExposeInfo returns the expose flag and per-endpoint expose settings
// for the specified applications.
func (f *FirewallerAPI) GetExposeInfo(ctx context.Context, args params.Entities) (params.ExposeInfoResults, error) {
	canAccess, err := f.accessApplication(ctx)
	if err != nil {
		return params.ExposeInfoResults{}, err
	}

	result := params.ExposeInfoResults{
		Results: make([]params.ExposeInfoResult, len(args.Entities)),
	}

	for i, entity := range args.Entities {
		tag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		isExposed, err := f.applicationService.IsApplicationExposed(ctx, tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if isExposed {
			continue
		}

		exposedEndpoints, err := f.applicationService.GetExposedEndpoints(ctx, tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].Exposed = true
		if len(exposedEndpoints) != 0 {
			mappedEndpoints := make(map[string]params.ExposedEndpoint)
			for endpoint, exposeDetails := range exposedEndpoints {
				mappedEndpoints[endpoint] = params.ExposedEndpoint{
					ExposeToSpaces: exposeDetails.ExposeToSpaceIDs.Values(),
					ExposeToCIDRs:  exposeDetails.ExposeToCIDRs.Values(),
				}
			}
			result.Results[i].ExposedEndpoints = mappedEndpoints
		}
	}
	return result, nil
}

// SpaceInfos returns a comprehensive representation of either all spaces or
// a filtered subset of the known spaces and their associated subnet details.
func (f *FirewallerAPI) SpaceInfos(ctx context.Context, args params.SpaceInfosParams) (params.SpaceInfos, error) {
	if !f.authorizer.AuthController() {
		return params.SpaceInfos{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}

	allSpaces, err := f.networkService.GetAllSpaces(ctx)
	if err != nil {
		return params.SpaceInfos{}, apiservererrors.ServerError(err)
	}
	// Apply filtering if required
	if len(args.FilterBySpaceIDs) != 0 {
		var (
			filteredList network.SpaceInfos
			selectList   = set.NewStrings(args.FilterBySpaceIDs...)
		)
		for _, si := range allSpaces {
			if selectList.Contains(si.ID.String()) {
				filteredList = append(filteredList, si)
			}
		}

		allSpaces = filteredList
	}

	return params.FromNetworkSpaceInfos(allSpaces), nil
}

// WatchSubnets returns a new StringsWatcher that watches the specified
// subnet tags or all tags if no entities are specified.
func (f *FirewallerAPI) WatchSubnets(ctx context.Context, args params.Entities) (params.StringsWatchResult, error) {
	if !f.authorizer.AuthController() {
		return params.StringsWatchResult{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}

	var subnetsToWatch set.Strings
	if len(args.Entities) != 0 {
		subnetsToWatch = set.NewStrings()
		for _, arg := range args.Entities {
			subnetTag, err := names.ParseSubnetTag(arg.Tag)
			if err != nil {
				return params.StringsWatchResult{}, apiservererrors.ServerError(err)
			}
			subnetsToWatch.Add(subnetTag.Id())
		}
	}

	watch, err := f.networkService.WatchSubnets(ctx, subnetsToWatch)
	if err != nil {
		return params.StringsWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}

	watcherId, initial, err := internal.EnsureRegisterWatcher[[]string](ctx, f.watcherRegistry, watch)
	if err != nil {
		return params.StringsWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.StringsWatchResult{StringsWatcherId: watcherId, Changes: initial}, nil
}

func setEquals(a, b set.Strings) bool {
	if a.Size() != b.Size() {
		return false
	}
	return a.Intersection(b).Size() == a.Size()
}
