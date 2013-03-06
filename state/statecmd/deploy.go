// Code shared by the CLI and API for the ServiceDeploy function.

package statecmd

import (
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// ServiceDeploy deploys the named service
// Only one of the parameters `config` and `configYAML` should be provided. If
// both are given then configYAML takes precedence.
func ServiceDeploy(conn *juju.Conn, curl *charm.URL, repo charm.Repository,
	bumpRevision bool, serviceName string, numUnits int,
	config map[string]string, configYAML string) (*state.Service, error) {
	charm, err := conn.PutCharm(curl, repo, bumpRevision)
	if err != nil {
		return &state.Service{}, err
	}
	if serviceName == "" {
		serviceName = curl.Name
	}
	svc, err := conn.State.AddService(serviceName, charm)
	if err != nil {
		return &state.Service{}, err
	}

	if configYAML != "" {
		args := params.ServiceSetYAML{
			ServiceName: serviceName,
			Config:      configYAML,
		}
		err = ServiceSetYAML(conn.State, args)
		if err != nil {
			return &state.Service{}, err
		}
	} else if config != nil {
		args := params.ServiceSet{
			ServiceName: serviceName,
			Options:     config,
		}
		err = ServiceSet(conn.State, args)
		if err != nil {
			return &state.Service{}, err
		}
	}

	if charm.Meta().Subordinate {
		return svc, nil
	}
	_, err = conn.AddUnits(svc, numUnits)
	return svc, err

}
