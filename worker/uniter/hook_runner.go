package uniter

import (
	"github.com/juju/charm"
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

	// configSettings are the current charm.Settings for the runner.
	configSettings charm.Settings
}

func NewHookRunner(unit *uniter.Unit, uuid, envName string, relations map[int]*ContextRelation,
	apiAddrs []string, serviceOwner string, proxySettings proxy.Settings) (*HookRunner, error) {
	runner := &HookRunner{
		unit:          unit,
		uuid:          uuid,
		envName:       envName,
		relations:     relations,
		apiAddrs:      apiAddrs,
		serviceOwner:  serviceOwner,
		proxySettings: proxySettings,
	}

	// Get and cache the addresses.
	var err error
	runner.publicAddress, err = unit.PublicAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return nil, err
	}
	runner.privateAddress, err = unit.PrivateAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return nil, err
	}

	return runner, nil
}

func (runner *HookRunner) ConfigSettings() (charm.Settings, error) {
	if runner.configSettings == nil {
		var err error
		runner.configSettings, err = runner.unit.ConfigSettings()
		if err != nil {
			return nil, err
		}
	}
	result := charm.Settings{}
	for name, value := range runner.configSettings {
		result[name] = value
	}
	return result, nil
}
