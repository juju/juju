// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"
	"strconv"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/api"
	apifirewaller "github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

// MachineDomainService provides access to machine domain operations.
type MachineDomainService interface {
	GetMachineLife(ctx context.Context, machineName coremachine.Name) (life.Value, error)
	GetInstanceID(ctx context.Context, machineUUID coremachine.UUID) (instance.Id, error)
	IsMachineManuallyProvisioned(ctx context.Context, machineName coremachine.Name) (bool, error)
	GetMachineUUID(ctx context.Context, name coremachine.Name) (coremachine.UUID, error)
	WatchModelMachines(ctx context.Context) (watcher.StringsWatcher, error)
}

// ModelConfigDomainService provides access to model configuration.
type ModelConfigDomainService interface {
	ModelConfig(ctx context.Context) (*config.Config, error)
	Watch(ctx context.Context) (watcher.StringsWatcher, error)
}

// ControllerConfigDomainService provides access to controller configuration.
type ControllerConfigDomainService interface {
	ControllerConfig(ctx context.Context) (controller.Config, error)
}

// NetworkDomainService provides access to network domain operations.
type NetworkDomainService interface {
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	WatchSubnets(ctx context.Context, subnetUUIDsToWatch set.Strings) (watcher.StringsWatcher, error)
}

// RelationDomainService provides access to relation domain operations.
type RelationDomainService interface {
	GetRelationUUIDByKey(ctx context.Context, relationKey corerelation.Key) (corerelation.UUID, error)
	GetRelationDetails(ctx context.Context, relationUUID corerelation.UUID) (domainrelation.RelationDetails, error)
	SetRelationErrorStatus(ctx context.Context, relationUUID corerelation.UUID, message string) error
}

// ExternalControllerDomainService provides access to external controller
// operations.
type ExternalControllerDomainService interface {
	ControllerForModel(ctx context.Context, modelUUID string) (*crossmodel.ControllerInfo, error)
}

// ApplicationDomainService provides access to application domain operations.
type ApplicationDomainService interface {
	GetUnitLife(ctx context.Context, unitName coreunit.Name) (life.Value, error)
}

// ModelInfoDomainService provides access to model info operations.
type ModelInfoDomainService interface {
	IsControllerModel(ctx context.Context) (bool, error)
}

// firewallerAPIAdapter implements FirewallerAPI using domain services.
type firewallerAPIAdapter struct {
	machineSvc       MachineDomainService
	modelConfigSvc   ModelConfigDomainService
	ctrlConfigSvc    ControllerConfigDomainService
	networkSvc       NetworkDomainService
	relationSvc      RelationDomainService
	extControllerSvc ExternalControllerDomainService
	appSvc           ApplicationDomainService
	modelInfoSvc     ModelInfoDomainService
}

// WatchModelMachines implements FirewallerAPI.
func (a *firewallerAPIAdapter) WatchModelMachines(ctx context.Context) (watcher.StringsWatcher, error) {
	return a.machineSvc.WatchModelMachines(ctx)
}

// WatchModelFirewallRules implements FirewallerAPI.
func (a *firewallerAPIAdapter) WatchModelFirewallRules(ctx context.Context) (watcher.NotifyWatcher, error) {
	return newModelFirewallRulesWatcher(a.modelConfigSvc)
}

// ModelFirewallRules implements FirewallerAPI.
func (a *firewallerAPIAdapter) ModelFirewallRules(ctx context.Context) (firewall.IngressRules, error) {
	cfg, err := a.modelConfigSvc.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctrlCfg, err := a.ctrlConfigSvc.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	isController, err := a.modelInfoSvc.IsControllerModel(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var rules firewall.IngressRules
	sshAllow := cfg.SSHAllow()
	if len(sshAllow) != 0 {
		rules = append(rules, firewall.NewIngressRule(
			network.MustParsePortRange("22"),
			sshAllow...,
		))
	}
	if isController {
		rules = append(rules, firewall.NewIngressRule(
			network.MustParsePortRange(strconv.Itoa(ctrlCfg.APIPort())),
			"0.0.0.0/0", "::/0",
		))
	}
	if isController && ctrlCfg.AutocertDNSName() != "" {
		rules = append(rules, firewall.NewIngressRule(
			network.MustParsePortRange("80"),
			"0.0.0.0/0", "::/0",
		))
	}
	return rules, nil
}

// ModelConfig implements FirewallerAPI.
func (a *firewallerAPIAdapter) ModelConfig(ctx context.Context) (*config.Config, error) {
	return a.modelConfigSvc.ModelConfig(ctx)
}

// Machine implements FirewallerAPI.
func (a *firewallerAPIAdapter) Machine(ctx context.Context, tag names.MachineTag) (Machine, error) {
	machineName := coremachine.Name(tag.Id())
	machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machineName)
	if err != nil {
		return nil, translateFacadeError(err)
	}
	machineLife, err := a.machineSvc.GetMachineLife(ctx, machineName)
	if err != nil {
		return nil, translateFacadeError(err)
	}
	return &machineAdapter{
		tag:         tag,
		machineUUID: machineUUID,
		machineName: machineName,
		life:        machineLife,
		machineSvc:  a.machineSvc,
	}, nil
}

// Unit implements FirewallerAPI.
func (a *firewallerAPIAdapter) Unit(ctx context.Context, tag names.UnitTag) (Unit, error) {
	unitName := coreunit.Name(tag.Id())
	unitLife, err := a.appSvc.GetUnitLife(ctx, unitName)
	if err != nil {
		return nil, translateFacadeError(err)
	}
	return &unitAdapter{
		tag:    tag,
		name:   unitName,
		life:   unitLife,
		appSvc: a.appSvc,
	}, nil
}

// Relation implements FirewallerAPI.
func (a *firewallerAPIAdapter) Relation(ctx context.Context, tag names.RelationTag) (*apifirewaller.Relation, error) {
	relationKey, err := corerelation.NewKeyFromString(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	relationUUID, err := a.relationSvc.GetRelationUUIDByKey(ctx, relationKey)
	if err != nil {
		return nil, translateFacadeError(err)
	}
	details, err := a.relationSvc.GetRelationDetails(ctx, relationUUID)
	if err != nil {
		return nil, translateFacadeError(err)
	}
	return apifirewaller.NewRelation(tag, details.Life), nil
}

func translateFacadeError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, machineerrors.MachineNotFound),
		errors.Is(err, applicationerrors.UnitNotFound),
		errors.Is(err, relationerrors.RelationNotFound),
		errors.Is(err, errors.NotFound):
		return &params.Error{Code: params.CodeNotFound, Message: err.Error()}
	default:
		return errors.Trace(err)
	}
}

// ControllerAPIInfoForModel implements FirewallerAPI.
func (a *firewallerAPIAdapter) ControllerAPIInfoForModel(ctx context.Context, modelUUID string) (*api.Info, error) {
	info, err := a.extControllerSvc.ControllerForModel(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &api.Info{
		Addrs:    info.Addrs,
		CACert:   info.CACert,
		ModelTag: names.NewModelTag(modelUUID),
	}, nil
}

// SetRelationStatus implements FirewallerAPI.
func (a *firewallerAPIAdapter) SetRelationStatus(ctx context.Context, relationKey string, status corerelation.Status, message string) error {
	rKey, err := corerelation.NewKeyFromString(relationKey)
	if err != nil {
		return errors.Trace(err)
	}
	relationUUID, err := a.relationSvc.GetRelationUUIDByKey(ctx, rKey)
	if err != nil {
		return errors.Trace(err)
	}
	return a.relationSvc.SetRelationErrorStatus(ctx, relationUUID, message)
}

// AllSpaceInfos implements FirewallerAPI.
func (a *firewallerAPIAdapter) AllSpaceInfos(ctx context.Context) (network.SpaceInfos, error) {
	return a.networkSvc.GetAllSpaces(ctx)
}

// WatchSubnets implements FirewallerAPI.
func (a *firewallerAPIAdapter) WatchSubnets(ctx context.Context) (watcher.StringsWatcher, error) {
	return a.networkSvc.WatchSubnets(ctx, set.NewStrings())
}

// machineAdapter implements the Machine interface using domain services.
type machineAdapter struct {
	tag         names.MachineTag
	machineUUID coremachine.UUID
	machineName coremachine.Name
	life        life.Value
	machineSvc  MachineDomainService
}

func (m *machineAdapter) Tag() names.MachineTag {
	return m.tag
}

func (m *machineAdapter) InstanceId(ctx context.Context) (instance.Id, error) {
	return m.machineSvc.GetInstanceID(ctx, m.machineUUID)
}

func (m *machineAdapter) Life() life.Value {
	return m.life
}

func (m *machineAdapter) IsManual(ctx context.Context) (bool, error) {
	return m.machineSvc.IsMachineManuallyProvisioned(ctx, m.machineName)
}

// unitAdapter implements the Unit interface using domain services.
type unitAdapter struct {
	tag    names.UnitTag
	name   coreunit.Name
	life   life.Value
	appSvc ApplicationDomainService
}

func (u *unitAdapter) Name() string {
	return u.name.String()
}

func (u *unitAdapter) Life() life.Value {
	return u.life
}

func (u *unitAdapter) Refresh(ctx context.Context) error {
	unitLife, err := u.appSvc.GetUnitLife(ctx, u.name)
	if err != nil {
		return errors.Trace(err)
	}
	u.life = unitLife
	return nil
}

func (u *unitAdapter) Application() (Application, error) {
	appName, _ := names.UnitApplication(u.tag.Id())
	return &applicationAdapter{
		name: appName,
		tag:  names.NewApplicationTag(appName),
	}, nil
}

// applicationAdapter implements the Application interface.
type applicationAdapter struct {
	name string
	tag  names.ApplicationTag
}

func (a *applicationAdapter) Name() string {
	return a.name
}

func (a *applicationAdapter) Tag() names.ApplicationTag {
	return a.tag
}

// setEquals checks if two sets of strings are equal.
func setEquals(a, b set.Strings) bool {
	if a.Size() != b.Size() {
		return false
	}
	return a.Intersection(b).Size() == a.Size()
}

// modelFirewallRulesWatcher watches for changes to model firewall rules
// (currently only ssh-allow config changes). This mirrors the server-side
// implementation in apiserver/facades/controller/firewaller/modelfirewallruleswatcher.go.
type modelFirewallRulesWatcher struct {
	catacomb       catacomb.Catacomb
	modelConfigSvc ModelConfigDomainService
	out            chan struct{}
	sshAllowCache  set.Strings
}

func newModelFirewallRulesWatcher(modelConfigSvc ModelConfigDomainService) (watcher.NotifyWatcher, error) {
	w := &modelFirewallRulesWatcher{
		modelConfigSvc: modelConfigSvc,
		out:            make(chan struct{}),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "model-firewall-rules-watcher",
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, err
}

func (w *modelFirewallRulesWatcher) loop() error {
	defer close(w.out)

	ctx, cancel := w.scopedContext()
	defer cancel()

	configWatcher, err := w.modelConfigSvc.Watch(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(configWatcher); err != nil {
		return errors.Trace(err)
	}

	var out chan struct{}
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case out <- struct{}{}:
			out = nil
		case _, ok := <-configWatcher.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}
			sshAllow, err := w.getSSHAllow(ctx)
			if err != nil {
				return errors.Trace(err)
			}
			if !setEquals(sshAllow, w.sshAllowCache) {
				out = w.out
				w.sshAllowCache = sshAllow
			}
		}
	}
}

func (w *modelFirewallRulesWatcher) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}

func (w *modelFirewallRulesWatcher) getSSHAllow(ctx context.Context) (set.Strings, error) {
	cfg, err := w.modelConfigSvc.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return set.NewStrings(cfg.SSHAllow()...), nil
}

func (w *modelFirewallRulesWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *modelFirewallRulesWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *modelFirewallRulesWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *modelFirewallRulesWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *modelFirewallRulesWatcher) Err() error {
	return w.catacomb.Err()
}
