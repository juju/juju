// Code shared by the CLI and API for the ServiceAddUnit function.

package statecmd

import (
	"log"
	"errors"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

func ServiceAddUnits(state *state.State, args params.ServiceAddUnits) error {
	log.Print("=========== Made it to the statecmd")
	conn, err := juju.NewConnFromState(state)
	if err != nil {
		return err
	}
	service, err := state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	if args.NumUnits < 1 {
		log.Print("=========== Too few units")
		return errors.New("must add at least one unit")
	}
	log.Print("=========== Enough units")
	_, err = conn.AddUnits(service, args.NumUnits)
	return err
}
