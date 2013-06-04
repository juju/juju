package statecmd

import (
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
)

// ServiceUpgradeCharm upgrades a service to the given charm.
func ServiceUpgradeCharm(st *state.State, service *state.Service, conn *juju.Conn, curl *charm.URL, repo charm.Repository, force bool, bumpRevision bool) error {
	oldURL, _ := service.CharmURL()
	if *curl == *oldURL {
		return fmt.Errorf("already running specified charm %q", curl)
	}
	sch, err := conn.PutCharm(curl, repo, bumpRevision)
	if err != nil {
		return err
	}
	return service.SetCharm(sch, force)
}
