// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import "fmt"

type reservation struct {
	id        string
	instances map[string]*Instance
	groups    []*securityGroup
}

func (srv *Server) newReservation(groups []*securityGroup) *reservation {
	r := &reservation{
		id:        fmt.Sprintf("r-%d", srv.reservationId.next()),
		instances: make(map[string]*Instance),
		groups:    groups,
	}

	srv.reservations[r.id] = r
	return r
}

func (r *reservation) hasRunningMachine() bool {
	for _, inst := range r.instances {
		if inst.state == ShuttingDown {
			// The instance is shutting down: tell the client that
			// it's still running, but transition it to terminated
			// so another query will not find it running.
			inst.state = Terminated
			return true
		}
		if inst.state != Terminated {
			return true
		}
	}
	return false
}
