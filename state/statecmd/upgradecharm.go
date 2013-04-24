// Code shared by the CLI and API for the ServiceUpgradeCharm function.

package statecmd

import (
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// ServiceUpgradeCharm upgrades a service's charm to the latest revision.
func ServiceUpgradeCharm(state *state.State, args params.ServiceUpgradeCharm) error {
	conn, err := juju.NewConnFromState(state)
	if err != nil {
		return err
	}
	service, err := state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	curl, _ := service.CharmURL()
	repo, err := charm.InferRepository(curl, args.RepoPath)
	if err != nil {
		return err
	}
	rev, err := repo.Latest(curl)
	if err != nil {
		return err
	}
	bumpRevision := false
	if curl.Revision == rev {
		if _, isLocal := repo.(*charm.LocalRepository); !isLocal {
			return fmt.Errorf("already running latest charm %q", curl)
		}
		// This is a local repository.
		if ch, err := repo.Get(curl); err != nil {
			return err
		} else if _, bumpRevision = ch.(*charm.Dir); !bumpRevision {
			// Only bump the revision when it's a directory.
			return fmt.Errorf("already running latest charm %q", curl)
		}
	}
	sch, err := conn.PutCharm(curl.WithRevision(rev), repo, bumpRevision)
	if err != nil {
		return err
	}
	return service.SetCharm(sch, args.Force)
}
