// The statecmd package is a temporary package
// to put code that's used by both cmd/juju and state/api.
// It is intended to wither away to nothing as functionality
// gets absorbed into state and state/api as appropriate
// when the command-line commands can invoke the
// API directly.
package statecmd

import (
	"reflect"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// ServiceGet returns the configuration for the named service.
func ServiceGet(st *state.State, p params.ServiceGet) (params.ServiceGetResults, error) {
	svc, err := st.Service(p.ServiceName)
	if err != nil {
		return params.ServiceGetResults{}, err
	}
	svcfg, err := svc.Config()
	if err != nil {
		return params.ServiceGetResults{}, err
	}
	charm, _, err := svc.Charm()
	if err != nil {
		return params.ServiceGetResults{}, err
	}
	chcfg := charm.Config().Options

	return params.ServiceGetResults{
		Service:  p.ServiceName,
		Charm:    charm.Meta().Name,
		Settings: merge(svcfg.Map(), chcfg),
	}, nil
}

// Merge service settings and charm schema.
func merge(serviceCfg map[string]interface{}, charmCfg map[string]charm.Option) map[string]interface{} {
	results := make(map[string]interface{})
	for k, v := range charmCfg {
		m := map[string]interface{}{
			"description": v.Description,
			"type":        v.Type,
		}
		s, ok := serviceCfg[k]
		if ok {
			m["value"] = s
		} else {
			// Breaks compatibility with py/juju.
			m["value"] = nil
		}
		if v.Default != nil {
			if reflect.DeepEqual(v.Default, s) {
				m["default"] = true
			}
		}
		results[k] = m
	}
	return results
}
