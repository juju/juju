// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statecmd

import (
	"fmt"
	"strings"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
)

// DestroyMachines1dot16 destroys the machines with the specified ids.
// This is copied from the 1.16.3 code to enable compatibility. It should be
// removed when we release a version that goes via the API only (whatever is
// after 1.18)
func DestroyMachines1dot16(st *state.State, ids ...string) (err error) {
	var errs []string
	for _, id := range ids {
		machine, err := st.Machine(id)
		switch {
		case errors.IsNotFoundError(err):
			err = fmt.Errorf("machine %s does not exist", id)
		case err != nil:
		case machine.Life() != state.Alive:
			continue
		default:
			err = machine.Destroy()
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	msg := "some machines were not destroyed"
	if len(errs) == len(ids) {
		msg = "no machines were destroyed"
	}
	return fmt.Errorf("%s: %s", msg, strings.Join(errs, "; "))
}
