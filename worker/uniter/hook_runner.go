package uniter

import (
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/uniter"
	"github.com/juju/utils/proxy"
)

type HookRunner struct {
	// unit is the current unit.
	unit *uniter.Unit

	// privateAddress is the cached value of the unit's private
	// address.
	privateAddress string

	// publicAddress is the cached value of the unit's public
	// address.
	publicAddress string

	// id identifies the context.
	id string

	// uuid is the unique identifier for the Uniter.
	uuid string

	// envName is the human friendly name of the environment.
	envName string

	// relations contains the context for every relation the unit is a member
	// of, keyed on relation id.
	relations map[int]*ContextRelation

	// apiAddrs contains the API server addresses.
	apiAddrs []string

	// serviceOwner contains the owner of the service
	serviceOwner string

	// proxySettings are the current proxy settings that the uniter knows about
	proxySettings proxy.Settings
}

func NewHookRunner(unit *uniter.Unit, id, uuid, envName string, relations map[int]*ContextRelation,
	apiAddrs []string, serviceOwner string, proxySettings proxy.Settings) (*HookContext, error) {

	// Get and cache the addresses.
	var err error
	ctx.publicAddress, err = unit.PublicAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return nil, err
	}
	ctx.privateAddress, err = unit.PrivateAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return nil, err
	}

	ctx := &HookRunner{
		unit:          unit,
		id:            id,
		uuid:          uuid,
		envName:       envName,
		relations:     relations,
		apiAddrs:      apiAddrs,
		serviceOwner:  serviceOwner,
		proxySettings: proxySettings,
	}
	return ctx, nil
}
