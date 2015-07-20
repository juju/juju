// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/service"
)

// UpgradeCharm is responsible for upgrading a service's charm.
type UpgradeCharmCommand struct {
	envcmd.EnvCommandBase
	ServiceName string
	Force       bool
	RepoPath    string // defaults to JUJU_REPOSITORY
	SwitchURL   string
	Revision    int // defaults to -1 (latest)
}

const upgradeCharmDoc = `
When no flags are set, the service's charm will be upgraded to the latest
revision available in the repository from which it was originally deployed. An
explicit revision can be chosen with the --revision flag.

If the charm came from a local repository, its path will be assumed to be
$JUJU_REPOSITORY unless overridden by --repository.

The local repository behaviour is tuned specifically to the workflow of a charm
author working on a single client machine; use of local repositories from
multiple clients is not supported and may lead to confusing behaviour. Each
local charm gets uploaded with the revision specified in the charm, if possible,
otherwise it gets a unique revision (highest in state + 1).

The --switch flag allows you to replace the charm with an entirely different
one. The new charm's URL and revision are inferred as they would be when running
a deploy command.

Please note that --switch is dangerous, because juju only has limited
information with which to determine compatibility; the operation will succeed,
regardless of potential havoc, so long as the following conditions hold:

- The new charm must declare all relations that the service is currently
participating in.
- All config settings shared by the old and new charms must
have the same types.

The new charm may add new relations and configuration settings.

--switch and --revision are mutually exclusive. To specify a given revision
number with --switch, give it in the charm URL, for instance "cs:wordpress-5"
would specify revision number 5 of the wordpress charm.

Use of the --force flag is not generally recommended; units upgraded while in an
error state will not have upgrade-charm hooks executed, and may cause unexpected
behavior.
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
	f.BoolVar(&c.Force, "force", false, "upgrade all units immediately, even if in error state")
	f.StringVar(&c.RepoPath, "repository", os.Getenv("JUJU_REPOSITORY"), "local charm repository path")
	f.StringVar(&c.SwitchURL, "switch", "", "crossgrade to a different charm")
	f.IntVar(&c.Revision, "revision", -1, "explicit revision of current charm")
}

func (c *UpgradeCharmCommand) Init(args []string) error {
	switch len(args) {
	case 1:
		if !names.IsValidService(args[0]) {
			return fmt.Errorf("invalid service name %q", args[0])
		}
		c.ServiceName = args[0]
	case 0:
		return fmt.Errorf("no service specified")
	default:
		return cmd.CheckEmpty(args[1:])
	}
	if c.SwitchURL != "" && c.Revision != -1 {
		return fmt.Errorf("--switch and --revision are mutually exclusive")
	}
	return nil
}

// Run connects to the specified environment and starts the charm
// upgrade process.
func (c *UpgradeCharmCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()
	oldURL, err := client.ServiceGetCharmURL(c.ServiceName)
	if err != nil {
		return err
	}

	conf, err := service.GetClientConfig(client)
	if err != nil {
		return errors.Trace(err)
	}

	var newRef *charm.Reference
	if c.SwitchURL != "" {
		newRef, err = charm.ParseReference(c.SwitchURL)
		if err != nil {
			return err
		}
	} else {
		// No new URL specified, but revision might have been.
		newRef = oldURL.WithRevision(c.Revision).Reference()
	}

	csClient, err := newCharmStoreClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer csClient.jar.Save()
	newURL, repo, err := resolveCharmURL(newRef.String(), csClient.params, ctx.AbsPath(c.RepoPath), conf)
	if err != nil {
		return errors.Trace(err)
	}

	// If no explicit revision was set with either SwitchURL
	// or Revision flags, discover the latest.
	if *newURL == *oldURL {
		if newRef.Revision != -1 {
			return fmt.Errorf("already running specified charm %q", newURL)
		}
		if newURL.Schema == "cs" {
			// No point in trying to upgrade a charm store charm when
			// we just determined that's the latest revision
			// available.
			return fmt.Errorf("already running latest charm %q", newURL)
		}
	}

	addedURL, err := addCharmViaAPI(client, ctx, newURL, repo, csClient)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	return block.ProcessBlockedError(client.ServiceSetCharm(c.ServiceName, addedURL.String(), c.Force), block.BlockChange)
}
