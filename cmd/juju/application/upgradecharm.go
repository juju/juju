// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable"
	csclientparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourceadapters"
)

// NewUpgradeCharmCommand returns a command which upgrades application's charm.
func NewUpgradeCharmCommand() cmd.Command {
	return modelcmd.Wrap(&upgradeCharmCommand{})
}

// UpgradeCharm is responsible for upgrading an application's charm.
type upgradeCharmCommand struct {
	modelcmd.ModelCommandBase
	ApplicationName string
	ForceUnits      bool
	ForceSeries     bool
	SwitchURL       string
	CharmPath       string
	Revision        int // defaults to -1 (latest)
	// Resources is a map of resource name to filename to be uploaded on upgrade.
	Resources map[string]string

	// Channel holds the charmstore channel to use when obtaining
	// the charm to be upgraded to.
	Channel csclientparams.Channel
}

const upgradeCharmDoc = `
When no flags are set, the application's charm will be upgraded to the latest
revision available in the repository from which it was originally deployed. An
explicit revision can be chosen with the --revision flag.

A path will need to be supplied to allow an updated copy of the charm
to be located.

Deploying from a path is intended to suit the workflow of a charm author working
on a single client machine; use of this deployment method from multiple clients
is not supported and may lead to confusing behaviour. Each local charm gets
uploaded with the revision specified in the charm, if possible, otherwise it
gets a unique revision (highest in state + 1).

When deploying from a path, the --path flag is used to specify the location from
which to load the updated charm. Note that the directory containing the charm must
match what was originally used to deploy the charm as a superficial check that the
updated charm is compatible.

Resources may be uploaded at upgrade time by specifying the --resource flag.
Following the resource flag should be name=filepath pair.  This flag may be
repeated more than once to upload more than one resource.

  juju upgrade-charm foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml

Where bar and baz are resources named in the metadata for the foo charm.

If the new version of a charm does not explicitly support the application's series, the
upgrade is disallowed unless the --force-series flag is used. This option should be
used with caution since using a charm on a machine running an unsupported series may
cause unexpected behavior.

The --switch flag allows you to replace the charm with an entirely different one.
The new charm's URL and revision are inferred as they would be when running a
deploy command.

Please note that --switch is dangerous, because juju only has limited
information with which to determine compatibility; the operation will succeed,
regardless of potential havoc, so long as the following conditions hold:

- The new charm must declare all relations that the application is currently
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
		Args:    "<application>",
		Purpose: "Upgrade an application's charm.",
		Doc:     upgradeCharmDoc,
	}
}

func (c *upgradeCharmCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.ForceUnits, "force-units", false, "Upgrade all units immediately, even if in error state")
	f.StringVar((*string)(&c.Channel), "channel", "", "Channel to use when getting the charm or bundle from the charm store")
	f.BoolVar(&c.ForceSeries, "force-series", false, "Upgrade even if series of deployed applications are not supported by the new charm")
	f.StringVar(&c.SwitchURL, "switch", "", "Crossgrade to a different charm")
	f.StringVar(&c.CharmPath, "path", "", "Upgrade to a charm located at path")
	f.IntVar(&c.Revision, "revision", -1, "Explicit revision of current charm")
	f.Var(stringMap{&c.Resources}, "resource", "Resource to be uploaded to the controller")
}

func (c *upgradeCharmCommand) Init(args []string) error {
	switch len(args) {
	case 1:
		if !names.IsValidApplication(args[0]) {
			return errors.Errorf("invalid application name %q", args[0])
		}
		c.ApplicationName = args[0]
	case 0:
		return errors.Errorf("no application specified")
	default:
		return cmd.CheckEmpty(args[1:])
	}
	if c.SwitchURL != "" && c.Revision != -1 {
		return errors.Errorf("--switch and --revision are mutually exclusive")
	}
	if c.CharmPath != "" && c.Revision != -1 {
		return errors.Errorf("--path and --revision are mutually exclusive")
	}
	if c.SwitchURL != "" && c.CharmPath != "" {
		return errors.Errorf("--switch and --path are mutually exclusive")
	}
	return nil
}

func (c *upgradeCharmCommand) newServiceAPIClient() (*application.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (c *upgradeCharmCommand) newModelConfigAPIClient() (*modelconfig.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(root), nil
}

// Run connects to the specified environment and starts the charm
// upgrade process.
func (c *upgradeCharmCommand) Run(ctx *cmd.Context) error {
	apiRoot, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	defer apiRoot.Close()

	serviceClient, err := c.newServiceAPIClient()
	if err != nil {
		return err
	}
	defer serviceClient.Close()

	oldURL, err := serviceClient.GetCharmURL(c.ApplicationName)
	if err != nil {
		return err
	}

	newRef := c.SwitchURL
	if newRef == "" {
		newRef = c.CharmPath
	}
	if c.SwitchURL == "" && c.CharmPath == "" {
		// If the charm we are upgrading is local, then we must
		// specify a path or switch url to upgrade with.
		if oldURL.Schema == "local" {
			return errors.New("upgrading a local charm requires either --path or --switch")
		}
		// No new URL specified, but revision might have been.
		newRef = oldURL.WithRevision(c.Revision).String()
	}

	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}
	csClient := newCharmStoreClient(bakeryClient).WithChannel(c.Channel)

	modelConfigClient, err := c.newModelConfigAPIClient()
	if err != nil {
		return err
	}
	defer modelConfigClient.Close()
	conf, err := getModelConfig(modelConfigClient)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(katco): This is a hack and a half. Get rid of this
	// nonsense. Only remains here because tests strewn about the
	// codebase are likely using this to not hit the real charmstore.
	charmRepo := config.SpecializeCharmRepo(
		charmrepo.NewCharmStoreFromClient(csClient),
		conf,
	).(*charmrepo.CharmStore)

	// TODO(katco): This anonymous adapter should go away in favor of
	// a comprehensive API passed into the upgrade-charm command.
	charmstoreAdapter := &struct {
		*charmstoreClient
		*apiClient
	}{
		charmstoreClient: &charmstoreClient{Client: csClient},
		apiClient:        &apiClient{Client: apiRoot.Client()},
	}

	chID, csMac, err := c.addCharm(charmRepo, conf, oldURL, newRef, charmstoreAdapter)
	if err != nil {
		if err1, ok := errors.Cause(err).(*termsRequiredError); ok {
			terms := strings.Join(err1.Terms, " ")
			return errors.Errorf(`Declined: please agree to the following terms %s. Try: "juju agree %s"`, terms, terms)
		}
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	ctx.Infof("Added charm %q to the model.", chID.URL)

	ids, err := c.upgradeResources(apiRoot, chID, csMac)
	if err != nil {
		return errors.Trace(err)
	}

	cfg := application.SetCharmConfig{
		ApplicationName: c.ApplicationName,
		CharmID:         chID,
		ForceSeries:     c.ForceSeries,
		ForceUnits:      c.ForceUnits,
		ResourceIDs:     ids,
	}

	return block.ProcessBlockedError(serviceClient.SetCharm(cfg), block.BlockChange)
}

// upgradeResources pushes metadata up to the server for each resource defined
// in the new charm's metadata and returns a map of resource names to pending
// IDs to include in the upgrage-charm call.
func (c *upgradeCharmCommand) upgradeResources(apiRoot base.APICallCloser, chID charmstore.CharmID, csMac *macaroon.Macaroon) (map[string]string, error) {
	filtered, err := getUpgradeResources(apiRoot, c.ApplicationName, chID.URL, c.Resources)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(filtered) == 0 {
		return nil, nil
	}

	// Note: the validity of user-supplied resources to be uploaded will be
	// checked further down the stack.
	return handleResources(apiRoot, c.Resources, c.ApplicationName, chID, csMac, filtered)
}

// TODO(ericsnow) Move these helpers into handleResources()?

func getUpgradeResources(
	apiRoot base.APICallCloser,
	serviceID string,
	cURL *charm.URL,
	cliResources map[string]string,
) (map[string]charmresource.Meta, error) {
	charmsClient := charms.NewClient(apiRoot)
	meta, err := getMetaResources(cURL, charmsClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(meta) == 0 {
		return nil, nil
	}

	current, err := getResources(serviceID, apiRoot)
	if err != nil {
		return nil, errors.Trace(err)
	}
	filtered := filterResources(meta, current, cliResources)
	return filtered, nil
}

func getMetaResources(cURL *charm.URL, client *charms.Client) (map[string]charmresource.Meta, error) {
	// this gets the charm info that was added to the controller using addcharm.
	charmInfo, err := client.CharmInfo(cURL.String())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return charmInfo.Meta.Resources, nil
}

func getResources(serviceID string, apiRoot base.APICallCloser) (map[string]resource.Resource, error) {
	resclient, err := resourceadapters.NewAPIClient(apiRoot)
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
func (c *upgradeCharmCommand) addCharm(
	charmRepo *charmrepo.CharmStore,
	config *config.Config,
	oldURL *charm.URL,
	charmRef string,
	charmAdder CharmAdder,
) (charmstore.CharmID, *macaroon.Macaroon, error) {
	var id charmstore.CharmID
	// Charm may have been supplied via a path reference.
	ch, newURL, err := charmrepo.NewCharmAtPathForceSeries(charmRef, oldURL.Series, c.ForceSeries)
	if err == nil {
		_, newName := filepath.Split(charmRef)
		if newName != oldURL.Name {
			return id, nil, errors.Errorf("cannot upgrade %q to %q", oldURL.Name, newName)
		}
		addedURL, err := charmAdder.AddLocalCharm(newURL, ch)
		id.URL = addedURL
		return id, nil, err
	}
	if _, ok := err.(*charmrepo.NotFoundError); ok {
		return id, nil, errors.Errorf("no charm found at %q", charmRef)
	}
	// If we get a "not exists" or invalid path error then we attempt to interpret
	// the supplied charm reference as a URL below, otherwise we return the error.
	if err != os.ErrNotExist && !charmrepo.IsInvalidPathError(err) {
		return id, nil, err
	}

	refURL, err := charm.ParseURL(charmRef)
	if err != nil {
		return id, nil, errors.Trace(err)
	}

	// Charm has been supplied as a URL so we resolve and deploy using the store.
	newURL, channel, supportedSeries, err := resolveCharm(charmRepo.ResolveWithChannel, config, refURL)
	if err != nil {
		return id, nil, errors.Trace(err)
	}
	id.Channel = channel
	if !c.ForceSeries && oldURL.Series != "" && newURL.Series == "" && !isSeriesSupported(oldURL.Series, supportedSeries) {
		series := []string{"no series"}
		if len(supportedSeries) > 0 {
			series = supportedSeries
		}
		return id, nil, errors.Errorf(
			"cannot upgrade from single series %q charm to a charm supporting %q. Use --force-series to override.",
			oldURL.Series, series,
		)
	}
	// If no explicit revision was set with either SwitchURL
	// or Revision flags, discover the latest.
	if *newURL == *oldURL {
		if refURL.Revision != -1 {
			return id, nil, errors.Errorf("already running specified charm %q", newURL)
		}
		// No point in trying to upgrade a charm store charm when
		// we just determined that's the latest revision
		// available.
		return id, nil, errors.Errorf("already running latest charm %q", newURL)
	}

	curl, csMac, err := addCharmFromURL(charmAdder, newURL, channel)
	if err != nil {
		return id, nil, errors.Trace(err)
	}
	id.URL = curl
	return id, csMac, nil
}
