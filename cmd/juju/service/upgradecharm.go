// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	apiservice "github.com/juju/juju/api/service"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewUpgradeCharmCommand returns a command which upgrades service's charm.
func NewUpgradeCharmCommand() cmd.Command {
	return modelcmd.Wrap(&upgradeCharmCommand{})
}

// UpgradeCharm is responsible for upgrading a service's charm.
type upgradeCharmCommand struct {
	modelcmd.ModelCommandBase
	ServiceName string
	ForceUnits  bool
	ForceSeries bool
	RepoPath    string // defaults to JUJU_REPOSITORY
	SwitchURL   string
	CharmPath   string
	Revision    int // defaults to -1 (latest)
	// Resources is a map of resource name to filename to be uploaded on upgrade.
	Resources map[string]string
}

const upgradeCharmDoc = `
When no flags are set, the service's charm will be upgraded to the latest
revision available in the repository from which it was originally deployed. An
explicit revision can be chosen with the --revision flag.

If the charm was not originally deployed from a repository, but from a path,
then a path will need to be supplied to allow an updated copy of the charm
to be located.

If the charm came from a local repository, its path will be assumed to be
$JUJU_REPOSITORY unless overridden by --repository. Note that deploying from
a local repository is deprecated in favour of deploying from a path.

Deploying from a path or local repository is intended to suit the workflow of a charm
author working on a single client machine; use of this deployment method from
multiple clients is not supported and may lead to confusing behaviour. Each
local charm gets uploaded with the revision specified in the charm, if possible,
otherwise it gets a unique revision (highest in state + 1).

When deploying from a path, the --path flag is used to specify the location from
which to load the updated charm. Note that the directory containing the charm must
match what was originally used to deploy the charm as a superficial check that the
updated charm is compatible.

Resources may be uploaded at upgrade time by specifying the --resource flag.
Following the resource flag should be name=filepath pair.  This flag may be
repeated more than once to upload more than one resource.

  juju upgrade-charm foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml

Where bar and baz are resources named in the metadata for the foo charm.

If the new version of a charm does not explicitly support the service's series, the
upgrade is disallowed unless the --force-series flag is used. This option should be
used with caution since using a charm on a machine running an unsupported series may
cause unexpected behavior.

When using a local repository, the --switch flag allows you to replace the charm
with an entirely different one. The new charm's URL and revision are inferred as
they would be when running a deploy command.

Please note that --switch is dangerous, because juju only has limited
information with which to determine compatibility; the operation will succeed,
regardless of potential havoc, so long as the following conditions hold:

- The new charm must declare all relations that the service is currently
participating in.
- All config settings shared by the old and new charms must
have the same types.

The new charm may add new relations and configuration settings.

--switch and --path are mutually exclusive.

--path and --revision are mutually exclusive. The revision of the updated charm
is determined by the contents of the charm at the specified path.

--switch and --revision are mutually exclusive. To specify a given revision
number with --switch, give it in the charm URL, for instance "cs:wordpress-5"
would specify revision number 5 of the wordpress charm.

Use of the --force-units flag is not generally recommended; units upgraded while in an
error state will not have upgrade-charm hooks executed, and may cause unexpected
behavior.
`

func (c *upgradeCharmCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upgrade-charm",
		Args:    "<service>",
		Purpose: "upgrade a service's charm",
		Doc:     upgradeCharmDoc,
	}
}

func (c *upgradeCharmCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.ForceUnits, "force-units", false, "upgrade all units immediately, even if in error state")
	f.BoolVar(&c.ForceSeries, "force-series", false, "upgrade even if series of deployed services are not supported by the new charm")
	f.StringVar(&c.RepoPath, "repository", os.Getenv("JUJU_REPOSITORY"), "local charm repository path")
	f.StringVar(&c.SwitchURL, "switch", "", "crossgrade to a different charm")
	f.StringVar(&c.CharmPath, "path", "", "upgrade to a charm located at path")
	f.IntVar(&c.Revision, "revision", -1, "explicit revision of current charm")
	f.Var(stringMap{&c.Resources}, "resource", "resource to be uploaded to the controller")
}

func (c *upgradeCharmCommand) Init(args []string) error {
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
	if c.CharmPath != "" && c.Revision != -1 {
		return fmt.Errorf("--path and --revision are mutually exclusive")
	}
	if c.SwitchURL != "" && c.CharmPath != "" {
		return fmt.Errorf("--switch and --path are mutually exclusive")
	}
	return nil
}

func (c *upgradeCharmCommand) newServiceAPIClient() (*apiservice.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiservice.NewClient(root), nil
}

// Run connects to the specified environment and starts the charm
// upgrade process.
func (c *upgradeCharmCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()

	serviceClient, err := c.newServiceAPIClient()
	if err != nil {
		return err
	}

	oldURL, err := serviceClient.GetCharmURL(c.ServiceName)
	if err != nil {
		return err
	}

	newRef := c.SwitchURL
	if newRef == "" {
		newRef = c.CharmPath
	}
	if c.SwitchURL == "" && c.CharmPath == "" {
		// No new URL specified, but revision might have been.
		newRef = oldURL.WithRevision(c.Revision).String()
	}

	httpClient, err := c.HTTPClient()
	if err != nil {
		return errors.Trace(err)
	}
	csClient := newCharmStoreClient(httpClient)

	addedURL, err := c.addCharm(oldURL, newRef, ctx, client, csClient)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	charmInfo, err := client.CharmInfo(addedURL.String())
	if err != nil {
		return err
	}

	var ids map[string]string

	if len(c.Resources) > 0 {
		metaRes := charmInfo.Meta.Resources
		// only include resource metadata for the files we're actually uploading,
		// otherwise the server will create empty resources that'll overwrite any
		// existing resources.
		for name, _ := range charmInfo.Meta.Resources {
			if _, ok := c.Resources[name]; !ok {
				delete(metaRes, name)
			}
		}

		ids, err = handleResources(c, c.Resources, c.ServiceName, metaRes)
		if err != nil {
			return errors.Trace(err)
		}
	}

	cfg := apiservice.SetCharmConfig{
		ServiceName: c.ServiceName,
		CharmUrl:    addedURL.String(),
		ForceSeries: c.ForceSeries,
		ForceUnits:  c.ForceUnits,
		ResourceIDs: ids,
	}

	return block.ProcessBlockedError(serviceClient.SetCharm(cfg), block.BlockChange)
}

// addCharm interprets the new charmRef and adds the specified charm if the new charm is different
// to what's already deployed as specified by oldURL.
func (c *upgradeCharmCommand) addCharm(oldURL *charm.URL, charmRef string, ctx *cmd.Context,
	client *api.Client, csClient *csClient,
) (*charm.URL, error) {
	// Charm may have been supplied via a path reference.
	ch, newURL, err := charmrepo.NewCharmAtPathForceSeries(charmRef, oldURL.Series, c.ForceSeries)
	if err == nil {
		_, newName := filepath.Split(charmRef)
		if newName != oldURL.Name {
			return nil, fmt.Errorf("cannot upgrade %q to %q", oldURL.Name, newName)
		}
		return client.AddLocalCharm(newURL, ch)
	}
	if _, ok := err.(*charmrepo.NotFoundError); ok {
		return nil, errors.Errorf("no charm found at %q", charmRef)
	}
	// If we get a "not exists" or invalid path error then we attempt to interpret
	// the supplied charm reference as a URL below, otherwise we return the error.
	if err != os.ErrNotExist && !charmrepo.IsInvalidPathError(err) {
		return nil, err
	}

	// Charm has been supplied as a URL so we resolve and deploy using the store.
	conf, err := getClientConfig(client)
	if err != nil {
		return nil, err
	}

	newURL, supportedSeries, repo, err := resolveCharmStoreEntityURL(resolveCharmStoreEntityParams{
		urlStr:          charmRef,
		requestedSeries: oldURL.Series,
		forceSeries:     c.ForceSeries,
		csParams:        csClient.params,
		repoPath:        ctx.AbsPath(c.RepoPath),
		conf:            conf,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !c.ForceSeries && oldURL.Series != "" && newURL.Series == "" && !isSeriesSupported(oldURL.Series, supportedSeries) {
		series := []string{"no series"}
		if len(supportedSeries) > 0 {
			series = supportedSeries
		}
		return nil, errors.Errorf(
			"cannot upgrade from single series %q charm to a charm supporting %q. Use --force-series to override.",
			oldURL.Series, series,
		)
	}
	// If no explicit revision was set with either SwitchURL
	// or Revision flags, discover the latest.
	if *newURL == *oldURL {
		newRef, _ := charm.ParseURL(charmRef)
		if newRef.Revision != -1 {
			return nil, fmt.Errorf("already running specified charm %q", newURL)
		}
		if newURL.Schema == "cs" {
			// No point in trying to upgrade a charm store charm when
			// we just determined that's the latest revision
			// available.
			return nil, fmt.Errorf("already running latest charm %q", newURL)
		}
	}

	addedURL, err := addCharmFromURL(client, newURL, repo, csClient)
	if err != nil {
		return nil, err
	}
	ctx.Infof("Added charm %q to the model.", addedURL)
	return addedURL, nil
}
