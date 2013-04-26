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
	Force       bool
	RepoPath    string // defaults to JUJU_REPOSITORY
	SwitchURL   string
	Revision    int // defaults to -1 (latest)
}

const upgradeCharmDoc = `
When no flags are set, the service's charm will be upgraded to the
latest revision available in the repository from which it was
originally deployed.

If the charm came from a local repository, its path will be assumed to
be $JUJU_REPOSITORY unless overridden by --repository. If there is no
newer revision of a local charm directory, the local directory's
revision will be automatically incremented to create a newer charm.

The local repository behaviour is tuned specifically to the workflow
of a charm author working on a single client machine; use of local
repositories from multiple clients is not supported and may lead to
confusing behaviour.

The --switch flag specifies a particular charm URL to use. This is
potentially dangerous as the new charm may not be fully compatible
with the old one. To make it a little safer, the following checks are
made:

- The new charm must declare all relations that the service is
currently participating in.

- The new charm must declare all the configuration settings of the old
charm and they must all have the same types as the old charm.

The new charm may add new relations and configuration settings.

In addition, you can specify --revision to select a specific revision
number to upgrade to, rather than the newest one. This cannot be
combined with --switch. To specify a given revision number with
--switch, give it in the charm URL, for instance "cs:wordpress-5" would
specify revision number 5 of the wordpress charm.

Use of the --force flag is not generally recommended; units upgraded
while in an error state will not have upgrade-charm hooks executed,
and may cause unexpected behavior.
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
	f.BoolVar(&c.Force, "force", false, "upgrade all units immediately, even if in error state")
	f.StringVar(&c.RepoPath, "repository", os.Getenv("JUJU_REPOSITORY"), "local charm repository path")
	f.StringVar(&c.SwitchURL, "switch", "", "charm URL to upgrade to")
	f.IntVar(&c.Revision, "revision", -1, "revision number to upgrade to")
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
	var curl *charm.URL
	var scurl *charm.URL // service's current charm URL when using --switch
	if c.SwitchURL != "" {
		if c.Revision >= 0 {
			return fmt.Errorf("cannot specify --switch and --revision together")
		}
		var err error
		conf, err := conn.State.EnvironConfig()
		if err != nil {
			return err
		}
		curl, err = charm.InferURL(c.SwitchURL, conf.DefaultSeries())
		if err != nil {
			return err
		}
		scurl, _ = service.CharmURL()
	} else {
		curl, _ = service.CharmURL()
	}
	repo, err := charm.InferRepository(curl, ctx.AbsPath(c.RepoPath))
	if err != nil {
		return err
	}
	bumpRevision := false
	explicitRevision := false
	rev := -1
	if c.SwitchURL != "" && curl.Revision != -1 {
		// Respect user's explicit revision when switching.
		rev = curl.Revision
		explicitRevision = true
	} else if c.Revision >= 0 {
		// Respect user's explicit revision as specified.
		rev = c.Revision
		explicitRevision = true
	} else {
		// No explicit revision set, use the latest available.
		latest, err := repo.Latest(curl)
		if err != nil {
			return err
		}
		rev = latest
	}
	// Only try bumping the revision when no explicit one is given and
	// the inferred latest matches the current one.
	considerBumpRevision := curl.Revision == rev && !explicitRevision
	if scurl != nil &&
		(scurl.WithRevision(-1).String() == curl.WithRevision(-1).String()) &&
		scurl.Revision == rev {
		// We have --switch, but the old charm is the same and no
		// explicit revision is given for the new one, so we need to
		// bump.
		curl.Revision = rev
		considerBumpRevision = true
	}
	if considerBumpRevision {
		// Only try bumping the revision when necessary (local dir charm).
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
	return service.SetCharm(sch, c.Force)
}
