// Code shared by the CLI and API for the ServiceDeploy function.

package statecmd

import (
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
)

// ServiceDeployParams are parameters for making the ServiceDeploy call.
type ServiceDeployParams struct {
	serviceName string
	config      map[string]string
	configYAML  string // Takes precedence over config if both are present.
	charmUrl    string
	numUnits    int
}

// ServiceDeploy deploys the named service
// Only one of the parameters `config` and `configYAML` should be provided. If
// both are given then configYAML takes precedence.
func ServiceDeploy(conn *juju.Conn, curl *charm.URL, repo charm.Repository,
	bumpRevision bool, serviceName string, numUnits int,
	config map[string]string, configYAML string) (state.Service, error) {
	charm, err := conn.PutCharm(curl, repo, bumpRevision)
	if err != nil {
		return nil, err
	}
	if serviceName == "" {
		serviceName = curl.Name
	}
	svc, err := conn.State.AddService(serviceName, charm)
	if err != nil {
		return nil, err
	}

	if configYAML {
		args := ServiceSetYAMLParams{
			ServiceName: serviceName,
			Config:      configYAML,
		}
		err = statecmd.ServiceSetYAML(conn.State, args)
		if err != nil {
			return err
		}
	} else if config {
		args := ServiceSetParams{
			ServiceName: serviceName,
			Options:     config,
		}
		err = statecmd.ServiceSet(conn.State, args)
		if err != nil {
			return err
		}
	}

	if charm.Meta().Subordinate {
		return svc, nil
	}
	_, err = conn.AddUnits(svc, numUnits)
	return svc, err

}
