// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
)

// Factory represents a long-lived object that can create execution contexts
// relevant to a specific unit. In its current state, it is somewhat bizarre
// and inconsistent; its main value is as an evolutionary step towards a better
// division of responsibilities across worker/uniter and its subpackages.
type Factory interface {

	// NewRunContext returns an execution context suitable for running an
	// arbitrary script.
	NewRunContext() (*HookContext, error)

	// NewHookContext returns an execution context suitable for running the
	// supplied hook definition (which must be valid).
	NewHookContext(hookInfo hook.Info) (*HookContext, error)

	// NewActionContext returns an execution context suitable for running the
	// supplied action (which is assumed to be already validated).
	NewActionContext(tag names.ActionTag, name string, params map[string]interface{}) (*HookContext, error)
}

type RelationsFunc func() map[int]*ContextRelation

// NewFactory returns a Factory capable of creating execution contexts backed
// by the supplied unit's supplied API connection.
func NewFactory(
	state *uniter.State, unitTag names.UnitTag, getRelations RelationsFunc,
) (
	Factory, error,
) {
	unit, err := state.Unit(unitTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	service, err := state.Service(unit.ServiceTag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	ownerTag, err := service.OwnerTag()
	if err != nil {
		return nil, errors.Trace(err)
	}
	machineTag, err := unit.AssignedMachine()
	if err != nil {
		return nil, errors.Trace(err)
	}
	environment, err := state.Environment()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &factory{
		unit:         unit,
		state:        state,
		envUUID:      environment.UUID(),
		envName:      environment.Name(),
		machineTag:   machineTag,
		ownerTag:     ownerTag,
		getRelations: getRelations,
		rand:         rand.New(rand.NewSource(time.Now().Unix())),
	}, nil
}

type factory struct {
	// API connection fields; unit should be deprecated, but isn't yet.
	unit  *uniter.Unit
	state *uniter.State

	// Fields that shouldn't change in a factory's lifetime.
	envUUID    string
	envName    string
	machineTag names.MachineTag
	ownerTag   names.UserTag

	// Callback to get relation state snapshot.
	getRelations RelationsFunc

	// For generating unique context ids.
	rand *rand.Rand
}

// NewRunContext exists to satisfy the Factory interface.
func (f *factory) NewRunContext() (*HookContext, error) {
	ctx, err := f.coreContext()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctx.id = f.newId("run-commands")
	return ctx, nil
}

// NewHookContext exists to satisfy the Factory interface.
func (f *factory) NewHookContext(hookInfo hook.Info) (*HookContext, error) {
	if err := hookInfo.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	ctx, err := f.coreContext()
	if err != nil {
		return nil, errors.Trace(err)
	}

	hookName := string(hookInfo.Kind)
	if hookInfo.Kind.IsRelation() {
		ctx.relationId = hookInfo.RelationId
		ctx.remoteUnitName = hookInfo.RemoteUnit
		relation, found := ctx.relations[hookInfo.RelationId]
		if !found {
			return nil, fmt.Errorf("unknown relation id: %v", hookInfo.RelationId)
		}
		if hookInfo.Kind == hooks.RelationDeparted {
			relation.DeleteMember(hookInfo.RemoteUnit)
		} else if hookInfo.RemoteUnit != "" {
			// Clear remote settings cache for changing remote unit.
			relation.UpdateMembers(SettingsMap{hookInfo.RemoteUnit: nil})
		}
		hookName = fmt.Sprintf("%s-%s", relation.Name(), hookInfo.Kind)
	}
	ctx.id = f.newId(hookName)
	return ctx, nil
}

// NewActionContext exists to satisfy the Factory interface.
func (f *factory) NewActionContext(tag names.ActionTag, name string, params map[string]interface{}) (*HookContext, error) {
	ctx, err := f.coreContext()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctx.actionData = NewActionData(&tag, params)
	ctx.id = f.newId(name)
	return ctx, nil
}

// newId returns a probably-unique identifier for a new context, containing the
// supplied string.
func (f *factory) newId(name string) string {
	return fmt.Sprintf("%s-%s-%d", f.unit.Name(), name, f.rand.Int63())
}

// coreContext creates a new context with all unspecialised fields filled in.
func (f *factory) coreContext() (*HookContext, error) {
	ctx := &HookContext{
		unit:          f.unit,
		state:         f.state,
		uuid:          f.envUUID,
		envName:       f.envName,
		serviceOwner:  f.ownerTag,
		relations:     f.getRelations(),
		relationId:    -1,
		canAddMetrics: true,
		pendingPorts:  make(map[PortRange]PortRangeInfo),
	}
	if err := f.updateContext(ctx); err != nil {
		return nil, err
	}
	return ctx, nil
}

// updateContext fills in all unspecialized fields that require an API call to
// discover.
//
// Approximately *every* line of code in this function represents a bug: ie, some
// piece of information we expose to the charm but which we fail to report changes
// to via hooks. Furthermore, the fact that we make multiple API calls at this
// time, rather than grabbing everything we need in one go, is unforgivably yucky.
func (f *factory) updateContext(ctx *HookContext) (err error) {
	defer errors.Trace(err)

	ctx.apiAddrs, err = f.state.APIAddresses()
	if err != nil {
		return err
	}
	ctx.machinePorts, err = f.state.AllMachinePorts(f.machineTag)
	if err != nil {
		return errors.Trace(err)
	}

	statusCode, statusInfo, err := f.unit.MeterStatus()
	if err != nil {
		return errors.Annotate(err, "could not retrieve meter status for unit")
	}
	ctx.meterStatus = &meterStatus{
		code: statusCode,
		info: statusInfo,
	}

	// GAAAAAAAH. Nothing here should ever be getting the environ config directly.
	environConfig, err := f.state.EnvironConfig()
	if err != nil {
		return err
	}
	ctx.proxySettings = environConfig.ProxySettings()

	// Calling these last, because there's a potential race: they're not guaranteed
	// to be set in time to be needed for a hook. If they're not, we just leave them
	// unset as we always have; this isn't great but it's about behaviour preservation.
	ctx.publicAddress, err = f.unit.PublicAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return err
	}
	ctx.privateAddress, err = f.unit.PrivateAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return err
	}
	return nil
}
