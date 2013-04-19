// The statecmd package is a temporary package
// to put code that's used by both cmd/juju and state/api.
// It is intended to wither away to nothing as functionality
// gets absorbed into state and state/api as appropriate
// when the command-line commands can invoke the
// API directly.
package statecmd

import (
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// ServiceGet returns the configuration for the named service.
func ServiceGet(st *state.State, p params.ServiceGet) (params.ServiceGetResults, error) {
	svc, err := st.Service(p.ServiceName)
	if err != nil {
		return params.ServiceGetResults{}, err
	}
	svcCfg, err := svc.Config()
	if err != nil {
		return params.ServiceGetResults{}, err
	}
	charm, _, err := svc.Charm()
	if err != nil {
		return params.ServiceGetResults{}, err
	}
	charmCfg := charm.Config().Options

	var constraints constraints.Value
	if svc.IsPrincipal() {
		constraints, err = svc.Constraints()
		if err != nil {
			return params.ServiceGetResults{}, err
		}
	}

	return params.ServiceGetResults{
		Service:     p.ServiceName,
		Charm:       charm.Meta().Name,
		Config:      merge(svcCfg.Map(), charmCfg),
		Constraints: constraints,
	}, nil
}

// merge returns the service settings merged with the charm
// schema, taking default values from the configuration
// in the charm metadata.
func merge(serviceCfg map[string]interface{}, charmCfg map[string]charm.Option) map[string]interface{} {
	results := make(map[string]interface{})
	for k, v := range charmCfg {
		m := map[string]interface{}{
			"description": v.Description,
			"type":        v.Type,
		}
		if s, ok := serviceCfg[k]; ok {
			m["value"] = s
		} else {
			m["value"] = v.Default
			// This breaks compatibility with py/juju, which will set
			// default to whether the value matches, not whether
			// it is set in the service confguration.
			m["default"] = true
		}
		results[k] = m
	}
	return results
}
