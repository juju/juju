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
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/macaroon.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	apiservice "github.com/juju/juju/api/service"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourceadapters"
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

	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}
	csClient := newCharmStoreClient(bakeryClient)

	addedURL, csMac, err := c.addCharm(oldURL, newRef, ctx, client, csClient)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	ids, err := c.upgradeResources(client, addedURL, csMac)
	if err != nil {
		return errors.Trace(err)
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

// upgradeResources pushes metadata up to the server for each resource defined
// in the new charm's metadata and returns a map of resource names to pending
// IDs to include in the upgrage-charm call.
func (c *upgradeCharmCommand) upgradeResources(client *api.Client, cURL *charm.URL, csMac *macaroon.Macaroon) (map[string]string, error) {
	filtered, err := getUpgradeResources(c, c.ServiceName, cURL, client, c.Resources)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(filtered) == 0 {
		return nil, nil
	}

	// Note: the validity of user-supplied resources to be uploaded will be
	// checked further down the stack.
	return handleResources(c, c.Resources, c.ServiceName, cURL, csMac, filtered)
}

// TODO(ericsnow) Move these helpers into handleResources()?

func getUpgradeResources(c APICmd, serviceID string, cURL *charm.URL, client *api.Client, cliResources map[string]string) (map[string]charmresource.Meta, error) {
	meta, err := getMetaResources(cURL, client)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(meta) == 0 {
		return nil, nil
	}

	current, err := getResources(serviceID, c.NewAPIRoot)
	if err != nil {
		return nil, errors.Trace(err)
	}
	filtered := filterResources(meta, current, cliResources)
	return filtered, nil
}

func getMetaResources(cURL *charm.URL, client *api.Client) (map[string]charmresource.Meta, error) {
	// this gets the charm info that was added to the controller using addcharm.
	charmInfo, err := client.CharmInfo(cURL.String())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return charmInfo.Meta.Resources, nil
}

func getResources(serviceID string, newAPIRoot func() (api.Connection, error)) (map[string]resource.Resource, error) {
	resclient, err := resourceadapters.NewAPIClient(newAPIRoot)
	if err != nil {
		return nil, errors.Trace(err)
	}
	svcs, err := resclient.ListResources([]string{serviceID})
	if err != nil {
		return nil, errors.Trace(err)
	}
	// ListResources guarantees a number of values returned == number of
	// services passed in.
	return resource.AsMap(svcs[0].Resources), nil
}

// TODO(ericsnow) Move filterResources() and shouldUploadMeta()
// somewhere more general under the "resource" package?

func filterResources(meta map[string]charmresource.Meta, current map[string]resource.Resource, uploads map[string]string) map[string]charmresource.Meta {
	filtered := make(map[string]charmresource.Meta)
	for name, res := range meta {
		if shouldUpgradeResource(res, uploads, current) {
			filtered[name] = res
		}
	}
	return filtered
}

// shouldUpgradeResource reports whether we should upload the metadata for the given
// resource.  This is always true for resources we're adding with the --resource
// flag. For resources we're not adding with --resource, we only upload metadata
// for charmstore resources.  Previously uploaded resources stay pinned to the
// data the user uploaded.
func shouldUpgradeResource(res charmresource.Meta, uploads map[string]string, current map[string]resource.Resource) bool {
	// Always upload metadata for resources the user is uploading during
	// upgrade-charm.
	if _, ok := uploads[res.Name]; ok {
		return true
	}
	cur, ok := current[res.Name]
	if !ok {
		// If there's no information on the server, there should be.
		return true
	}
	// Never override existing resources a user has already uploaded.
	if cur.Origin == charmresource.OriginUpload {
		return false
	}
	return true
}

// addCharm interprets the new charmRef and adds the specified charm if the new charm is different
// to what's already deployed as specified by oldURL.
func (c *upgradeCharmCommand) addCharm(oldURL *charm.URL, charmRef string, ctx *cmd.Context,
	client *api.Client, csClient *csClient,
) (*charm.URL, *macaroon.Macaroon, error) {
	// Charm may have been supplied via a path reference.
	ch, newURL, err := charmrepo.NewCharmAtPathForceSeries(charmRef, oldURL.Series, c.ForceSeries)
	if err == nil {
		_, newName := filepath.Split(charmRef)
		if newName != oldURL.Name {
			return nil, nil, fmt.Errorf("cannot upgrade %q to %q", oldURL.Name, newName)
		}
		addedURL, err := client.AddLocalCharm(newURL, ch)
		return addedURL, nil, err
	}
	if _, ok := err.(*charmrepo.NotFoundError); ok {
		return nil, nil, errors.Errorf("no charm found at %q", charmRef)
	}
	// If we get a "not exists" or invalid path error then we attempt to interpret
	// the supplied charm reference as a URL below, otherwise we return the error.
	if err != os.ErrNotExist && !charmrepo.IsInvalidPathError(err) {
		return nil, nil, err
	}

	// Charm has been supplied as a URL so we resolve and deploy using the store.
	conf, err := getClientConfig(client)
	if err != nil {
		return nil, nil, err
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
		return nil, nil, errors.Trace(err)
	}
	if !c.ForceSeries && oldURL.Series != "" && newURL.Series == "" && !isSeriesSupported(oldURL.Series, supportedSeries) {
		series := []string{"no series"}
		if len(supportedSeries) > 0 {
			series = supportedSeries
		}
		return nil, nil, errors.Errorf(
			"cannot upgrade from single series %q charm to a charm supporting %q. Use --force-series to override.",
			oldURL.Series, series,
		)
	}
	// If no explicit revision was set with either SwitchURL
	// or Revision flags, discover the latest.
	if *newURL == *oldURL {
		newRef, _ := charm.ParseURL(charmRef)
		if newRef.Revision != -1 {
			return nil, nil, fmt.Errorf("already running specified charm %q", newURL)
		}
		if newURL.Schema == "cs" {
			// No point in trying to upgrade a charm store charm when
			// we just determined that's the latest revision
			// available.
			return nil, nil, fmt.Errorf("already running latest charm %q", newURL)
		}
	}

	addedURL, csMac, err := addCharmFromURL(client, newURL, repo, csClient)
	if err != nil {
		return nil, nil, err
	}
	ctx.Infof("Added charm %q to the model.", addedURL)
	return addedURL, csMac, nil
}
