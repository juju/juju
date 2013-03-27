package main

import (
	"errors"
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"os"
)

// UpgradeCharm is responsible for upgrading a service's charm.
type UpgradeCharmCommand struct {
	EnvCommandBase
	ServiceName string
	RepoPath    string // defaults to JUJU_REPOSITORY
}

const upgradeCharmDoc = `
When no flags are set, the service's charm will be upgraded to the latest
revision available in the repository from which it was originally deployed.

If the charm came from a local repository, its path will be assumed to be
$JUJU_REPOSITORY unless overridden by --repository. If there is no newer
revision of a local charm directory, the local directory's revision will be
automatically incremented to create a newer charm.

The local repository behaviour is tuned specifically to the workflow of a charm
author working on a single client machine; use of local repositories from
multiple clients is not supported and may lead to confusing behaviour.
`

func (c *UpgradeCharmCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upgrade-charm",
		Args:    "<service>",
		Purpose: "upgrade a service's charm",
		Doc:     upgradeCharmDoc,
	}
}

func (c *UpgradeCharmCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.StringVar(&c.RepoPath, "repository", os.Getenv("JUJU_REPOSITORY"), "local charm repository path")
}

func (c *UpgradeCharmCommand) Init(args []string) error {
	switch len(args) {
	case 1:
		if !state.IsServiceName(args[0]) {
			return fmt.Errorf("invalid service name %q", args[0])
		}
		c.ServiceName = args[0]
	case 0:
		return errors.New("no service specified")
	default:
		return cmd.CheckEmpty(args[1:])
	}
	// TODO(dimitern): add the other flags --switch, --force and --revision.
	return nil
}

// Run connects to the specified environment and starts the charm
// upgrade process.
func (c *UpgradeCharmCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	service, err := conn.State.Service(c.ServiceName)
	if err != nil {
		return err
	}
	curl, _ := service.CharmURL()
	repo, err := charm.InferRepository(curl, ctx.AbsPath(c.RepoPath))
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
	// TODO(dimitern): get this from the --force flag
	forced := false
	return service.SetCharm(sch, forced)
}
