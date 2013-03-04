// Code shared by the CLI and API for the ServiceDeploy function.

package statecmd

import (
	"code.google.com/p/go.net/websocket"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
)

// ServiceDeployParams are parameters for making the ServiceDeploy call.
type ServiceDeployParams struct {
	charmName   string
	serviceName string
	numUnits    int
	config      string
}

// ServiceDeploy deploys the named service
func ServiceDeploy(conn *juju.Conn, curl *charm.URL, repo charm.Repository,
	bumpRevision bool, serviceName string, numUnits int) error {
	charm, err := conn.PutCharm(curl, repo, bumpRevision)
	state = conn.State
	if err != nil {
		return err
	}
	svcName := serviceName
	if svcName == "" {
		svcName = curl.Name
	}
	svc, err := state.AddService(svcName, charm)
	if err != nil {
		return err
	}
	if charm.Meta().Subordinate {
		return nil
	}
	_, err = conn.AddUnits(svc, numUnits)
	return err

}
