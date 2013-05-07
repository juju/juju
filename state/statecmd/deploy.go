package statecmd

import (
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

func ServiceDeploy(st *state.State, args params.ServiceDeploy, conn *juju.Conn, curl *charm.URL, repo charm.Repository) error {
	charm, err := conn.PutCharm(curl, repo, args.BumpRevision)
	if err != nil {
		return err
	}
	if charm.Meta().Subordinate {
		empty := constraints.Value{}
		if args.Constraints != empty {
			return state.ErrSubordinateConstraints
		}
		if args.ForceMachineId != "" {
			return fmt.Errorf("subordinate service cannot specify force-machine")
		}
	}
	serviceName := args.ServiceName
	if serviceName == "" {
		serviceName = curl.Name
	}
	deployArgs := juju.DeployServiceParams{
		Charm:       charm,
		ServiceName: serviceName,
		NumUnits:    args.NumUnits,
		// BUG(lp:1162122): --config has no tests.
		ConfigYAML:     args.ConfigYAML,
		Constraints:    args.Constraints,
		ForceMachineId: args.ForceMachineId,
	}
	_, err = conn.DeployService(deployArgs)
	return err
}
