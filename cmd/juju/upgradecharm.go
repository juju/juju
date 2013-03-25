package main

import (
	"errors"
	"fmt"
	"io/ioutil"
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
<service> needs to be an existing deployed service, whose charm you want to upgrade.
--repository defaults to $JUJU_REPOSITORY when not set explicitly.

The given <service>'s charm will be upgraded to the latest available revision, either
in the charm store or from a specified local repository.
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
	if _, err := ioutil.ReadDir(c.RepoPath); err != nil {
		return fmt.Errorf("invalid repository path specified: %s", c.RepoPath)
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
		if _, bumpRevision = repo.(*charm.LocalRepository); !bumpRevision {
			return fmt.Errorf("already running latest charm %q", curl)
		} else {
			// This is a local repository.
			if ch, err := repo.Get(curl); err != nil {
				return err
			} else if _, bumpRevision = ch.(*charm.Dir); !bumpRevision {
				// Only bump the revision when it's a directory.
				return fmt.Errorf("already running latest charm %q", curl)
			}
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
