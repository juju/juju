package statecmd

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// ServiceDeploy deploys a service to the environment from the given repository.
// The connection, charm URL, and repository will be provided by the caller,
// since these values already exist in the calling locations and can be 
// defaulted in different scenarios (i.e.: only the charmstore will be used 
// when called from the websocket API).
func ServiceDeploy(st *state.State, args params.ServiceDeploy, conn *juju.Conn, curl *charm.URL, repo charm.Repository) error {
	if args.ServiceName != "" && !state.IsServiceName(args.ServiceName) {
		return fmt.Errorf("invalid service name %q", args.ServiceName)
	}
	if args.ForceMachineId != "" {
		if !state.IsMachineId(args.ForceMachineId) {
			return fmt.Errorf("invalid machine id %q", args.ForceMachineId)
		}
		if args.NumUnits > 1 {
			return fmt.Errorf("force-machine cannot be used for multiple units")
		}
	}

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
	} else {
		if args.NumUnits < 1 {
			return errors.New("must deploy at least one unit")
		}
	}
	serviceName := args.ServiceName
	if serviceName == "" {
		serviceName = curl.Name
	}
	deployArgs := juju.DeployServiceParams{
		Charm:          charm,
		ServiceName:    serviceName,
		NumUnits:       args.NumUnits,
		Config:         args.Config,
		ConfigYAML:     args.ConfigYAML,
		Constraints:    args.Constraints,
		ForceMachineId: args.ForceMachineId,
	}
	_, err = conn.DeployService(deployArgs)
	return err
}
