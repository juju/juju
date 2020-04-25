// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"os"

	"github.com/juju/charm/v7"
	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/charmrepo/v5"
	csclientparams "github.com/juju/charmrepo/v5/csclient/params"
	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"github.com/juju/version"
	"github.com/juju/worker/v2/catacomb"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/api/spaces"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/storage"
)

func newUpgradeCharmCommand() *upgradeCharmCommand {
	return &upgradeCharmCommand{
		DeployResources: resourceadapters.DeployResources,
		ResolveCharm:    resolveCharm,
		NewCharmAdder:   newCharmAdder,
		NewCharmClient: func(conn base.APICallCloser) CharmClient {
			return charms.NewClient(conn)
		},
		NewCharmUpgradeClient: func(conn base.APICallCloser) CharmAPIClient {
			return application.NewClient(conn)
		},
		NewResourceLister: func(conn base.APICallCloser) (ResourceLister, error) {
			resclient, err := resourceadapters.NewAPIClient(conn)
			if err != nil {
				return nil, err
			}
			return resclient, nil
		},
		NewSpacesClient: func(conn base.APICallCloser) SpacesAPI {
			return spaces.NewAPI(conn)
		},
		CharmStoreURLGetter: getCharmStoreAPIURL,
		NewCharmStore: func(
			bakeryClient *httpbakery.Client,
			csURL string,
			channel csclientparams.Channel,
		) charmrepoForDeploy {
			return getCharmStore(bakeryClient, csURL, channel)
		},
	}
}

// NewUpgradeCharmCommand returns a command which upgrades application's charm.
func NewUpgradeCharmCommand() cmd.Command {
	return modelcmd.Wrap(newUpgradeCharmCommand())
}

// CharmAPIClient defines a subset of the application facade that deals with
// charm related upgrades.
type CharmAPIClient interface {
	CharmUpgradeClient
}

// CharmUpgradeClient defines a subset of the application facade, as required
// by the upgrade-charm command.
type CharmUpgradeClient interface {
	GetCharmURL(string, string) (*charm.URL, error)
	Get(string, string) (*params.ApplicationGetResults, error)
	SetCharm(string, application.SetCharmConfig) error
}

// CharmClient defines a subset of the charms facade, as required
// by the upgrade-charm command.
type CharmClient interface {
	CharmInfo(string) (*charms.CharmInfo, error)
}

// ResourceLister defines a subset of the resources facade, as required
// by the upgrade-charm command.
type ResourceLister interface {
	ListResources([]string) ([]resource.ApplicationResources, error)
}

// NewCharmAdderFunc is the type of a function used to construct
// a new CharmAdder.
type NewCharmAdderFunc func(
	api.Connection,
) CharmAdder

// NewCharmStoreFunc constructs a charm store client.
type NewCharmStoreFunc func(
	*httpbakery.Client,
	string, // Charmstore API URL
	csclientparams.Channel,
) charmrepoForDeploy

// UpgradeCharm is responsible for upgrading an application's charm.
type upgradeCharmCommand struct {
	modelcmd.ModelCommandBase

	DeployResources       resourceadapters.DeployResourcesFunc
	ResolveCharm          ResolveCharmFunc
	NewCharmAdder         NewCharmAdderFunc
	NewCharmStore         NewCharmStoreFunc
	NewCharmClient        func(base.APICallCloser) CharmClient
	NewCharmUpgradeClient func(base.APICallCloser) CharmAPIClient
	NewResourceLister     func(base.APICallCloser) (ResourceLister, error)
	NewSpacesClient       func(base.APICallCloser) SpacesAPI
	CharmStoreURLGetter   func(base.APICallCloser) (string, error)

	ApplicationName string
	// Force should be ubiquitous and we should eventually deprecate both
	// ForceUnits and ForceSeries; instead just using "force"
	Force       bool
	ForceUnits  bool
	ForceSeries bool
	SwitchURL   string
	CharmPath   string
	Revision    int // defaults to -1 (latest)

	BindToSpaces string
	Bindings     map[string]string

	// Resources is a map of resource name to filename to be uploaded on upgrade.
	Resources map[string]string

	// Channel holds the charmstore channel to use when obtaining
	// the charm to be upgraded to.
	Channel csclientparams.Channel

	// Config is a config file variable, pointing at a YAML file containing
	// the application config to update.
	Config cmd.FileVar

	// Storage is a map of storage constraints, keyed on the storage name
	// defined in charm storage metadata, to add or update during upgrade.
	Storage map[string]storage.Constraints

	catacomb catacomb.Catacomb
	plan     catacomb.Plan
}

const upgradeCharmDoc = `
When no options are set, the application's charm will be upgraded to the latest
revision available in the repository from which it was originally deployed. An
explicit revision can be chosen with the --revision option.

A path will need to be supplied to allow an updated copy of the charm
to be located.

Deploying from a path is intended to suit the workflow of a charm author working
on a single client machine; use of this deployment method from multiple clients
is not supported and may lead to confusing behaviour. Each local charm gets
uploaded with the revision specified in the charm, if possible, otherwise it
gets a unique revision (highest in state + 1).

When deploying from a path, the --path option is used to specify the location from
which to load the updated charm. Note that the directory containing the charm must
match what was originally used to deploy the charm as a superficial check that the
updated charm is compatible.

Resources may be uploaded at upgrade time by specifying the --resource option.
Following the resource option should be name=filepath pair.  This option may be
repeated more than once to upload more than one resource.

  juju upgrade-charm foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml

Where bar and baz are resources named in the metadata for the foo charm.

Storage constraints may be added or updated at upgrade time by specifying
the --storage option, with the same format as specified in "juju deploy".
If new required storage is added by the new charm revision, then you must
specify constraints or the defaults will be applied.

  juju upgrade-charm foo --storage cache=ssd,10G

Charm settings may be added or updated at upgrade time by specifying the
--config option, pointing to a YAML-encoded application config file.

  juju upgrade-charm foo --config config.yaml

If the new version of a charm does not explicitly support the application's series, the
upgrade is disallowed unless the --force-series option is used. This option should be
used with caution since using a charm on a machine running an unsupported series may
cause unexpected behavior.

The --switch option allows you to replace the charm with an entirely different one.
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

Use of the --force-units option is not generally recommended; units upgraded while in an
error state will not have upgrade-charm hooks executed, and may cause unexpected
behavior.

--force option for LXD Profiles is not generally recommended when upgrading an 
application; overriding profiles on the container may cause unexpected 
behavior. 
`

func (c *upgradeCharmCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "upgrade-charm",
		Args:    "<application>",
		Purpose: "Upgrade an application's charm.",
		Doc:     upgradeCharmDoc,
	})
}

func (c *upgradeCharmCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "Allow a charm to be upgraded which bypasses LXD profile allow list")
	f.BoolVar(&c.ForceUnits, "force-units", false, "Upgrade all units immediately, even if in error state")
	f.StringVar((*string)(&c.Channel), "channel", "", "Channel to use when getting the charm or bundle from the charm store")
	f.BoolVar(&c.ForceSeries, "force-series", false, "Upgrade even if series of deployed applications are not supported by the new charm")
	f.StringVar(&c.SwitchURL, "switch", "", "Crossgrade to a different charm")
	f.StringVar(&c.CharmPath, "path", "", "Upgrade to a charm located at path")
	f.IntVar(&c.Revision, "revision", -1, "Explicit revision of current charm")
	f.Var(stringMap{&c.Resources}, "resource", "Resource to be uploaded to the controller")
	f.Var(storageFlag{&c.Storage, nil}, "storage", "Charm storage constraints")
	f.Var(&c.Config, "config", "Path to yaml-formatted application config")
	f.StringVar(&c.BindToSpaces, "bind", "", "Configure application endpoint bindings to spaces")
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

// Run connects to the specified environment and starts the charm
// upgrade process.
func (c *upgradeCharmCommand) Run(ctx *cmd.Context) error {
	apiRoot, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = apiRoot.Close() }()

	// If the user has specified config or storage constraints,
	// make sure the server has facade version 2 at a minimum.
	if c.Config.Path != "" || len(c.Storage) > 0 {
		action := "updating config"
		if c.Config.Path == "" {
			action = "updating storage constraints"
		}
		if err := c.checkApplicationFacadeSupport(apiRoot, action, 2); err != nil {
			return err
		}
	}

	generation, err := c.ActiveBranch()
	if err != nil {
		return errors.Trace(err)
	}
	charmUpgradeClient := c.NewCharmUpgradeClient(apiRoot)
	oldURL, err := charmUpgradeClient.GetCharmURL(generation, c.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}

	if c.BindToSpaces != "" {
		if err := c.checkApplicationFacadeSupport(apiRoot, "specifying bindings", 11); err != nil {
			return err
		}

		if err := c.parseBindFlag(apiRoot); err != nil {
			return err
		}
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

	// First, ensure the charm is added to the model.
	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}
	conAPIRoot, err := c.NewControllerAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	csURL, err := c.CharmStoreURLGetter(conAPIRoot)
	if err != nil {
		return errors.Trace(err)
	}

	applicationInfo, err := charmUpgradeClient.Get(generation, c.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}

	if c.Channel == "" {
		c.Channel = csclientparams.Channel(applicationInfo.Channel)
	}

	chID, csMac, err := c.addCharm(addCharmParams{
		charmAdder:     c.NewCharmAdder(apiRoot),
		charmRepo:      c.NewCharmStore(bakeryClient, csURL, c.Channel),
		authorizer:     newCharmStoreClient(bakeryClient, csURL),
		oldURL:         oldURL,
		newCharmRef:    newRef,
		deployedSeries: applicationInfo.Series,
		force:          c.Force,
	})
	if err != nil {
		if termErr, ok := errors.Cause(err).(*common.TermsRequiredError); ok {
			return errors.Trace(termErr.UserErr())
		}
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	ctx.Infof("Added charm %q to the model.", chID.URL)

	// Next, upgrade resources.
	charmsClient := c.NewCharmClient(apiRoot)
	resourceLister, err := c.NewResourceLister(apiRoot)
	if err != nil {
		return errors.Trace(err)
	}
	meta, err := getMetaResources(chID.URL, charmsClient)
	if err != nil {
		return errors.Trace(err)
	}
	ids, err := c.upgradeResources(apiRoot, resourceLister, chID, csMac, meta)
	if err != nil {
		return errors.Trace(err)
	}

	var bindingsChangelog []string
	if apiRoot.BestFacadeVersion("Application") >= 11 {
		// Fetch information about the charm we want to upgrade to and
		// print out the updated endpoint binding plan.
		charmInfo, err := c.NewCharmClient(apiRoot).CharmInfo(chID.URL.String())
		if err != nil {
			return errors.Trace(err)
		}

		curBindings := applicationInfo.EndpointBindings
		appDefaultSpace := curBindings[""]
		newCharmEndpoints := allEndpoints(charmInfo)
		if err := c.validateEndpointNames(newCharmEndpoints, curBindings, c.Bindings); err != nil {
			return errors.Trace(err)
		}
		c.Bindings, bindingsChangelog = mergeBindings(newCharmEndpoints, curBindings, c.Bindings, appDefaultSpace)
	}

	// Finally, upgrade the application.
	var configYAML []byte
	if c.Config.Path != "" {
		configYAML, err = c.Config.Read(ctx)
		if err != nil {
			return errors.Trace(err)
		}
	}
	cfg := application.SetCharmConfig{
		ApplicationName:    c.ApplicationName,
		CharmID:            chID,
		ConfigSettingsYAML: string(configYAML),
		Force:              c.Force,
		ForceSeries:        c.ForceSeries,
		ForceUnits:         c.ForceUnits,
		ResourceIDs:        ids,
		StorageConstraints: c.Storage,
		EndpointBindings:   c.Bindings,
	}

	if err := block.ProcessBlockedError(charmUpgradeClient.SetCharm(generation, cfg), block.BlockChange); err != nil {
		return err
	}

	// Emit binding changelog after a successful call to SetCharm.
	for _, change := range bindingsChangelog {
		ctx.Infof(change)
	}

	return nil
}

func (c *upgradeCharmCommand) validateEndpointNames(newCharmEndpoints set.Strings, oldEndpointsMap, userBindings map[string]string) error {
	for epName := range userBindings {
		if _, exists := oldEndpointsMap[epName]; exists || epName == "" {
			continue
		}

		if !newCharmEndpoints.Contains(epName) {
			return errors.NotFoundf("endpoint %q", epName)
		}
	}
	return nil
}

func (c *upgradeCharmCommand) parseBindFlag(apiRoot base.APICallCloser) error {
	if c.BindToSpaces == "" {
		return nil
	}

	// Fetch known spaces from server
	knownSpaceList, err := c.NewSpacesClient(apiRoot).ListSpaces()
	if err != nil {
		return errors.Trace(err)
	}

	knownSpaces := make([]string, 0, len(knownSpaceList))
	for _, sp := range knownSpaceList {
		knownSpaces = append(knownSpaces, sp.Name)
	}

	// Parse expression
	bindings, err := parseBindExpr(c.BindToSpaces, knownSpaces)
	if err != nil {
		return errors.Trace(err)
	}

	c.Bindings = bindings
	return nil
}

type versionQuerier interface {
	BestFacadeVersion(string) int
	ServerVersion() (version.Number, bool)
}

func (c *upgradeCharmCommand) checkApplicationFacadeSupport(verQuerier versionQuerier, action string, minVersion int) error {
	if verQuerier.BestFacadeVersion("Application") >= minVersion {
		return nil
	}

	suffix := "this server"
	if version, ok := verQuerier.ServerVersion(); ok {
		suffix = fmt.Sprintf("server version %s", version)
	}

	return errors.New(action + " at upgrade-charm time is not supported by " + suffix)
}

// upgradeResources pushes metadata up to the server for each resource defined
// in the new charm's metadata and returns a map of resource names to pending
// IDs to include in the upgrage-charm call.
//
// TODO(axw) apiRoot is passed in here because DeployResources requires it,
// DeployResources should accept a resource-specific client instead.
func (c *upgradeCharmCommand) upgradeResources(
	apiRoot base.APICallCloser,
	resourceLister ResourceLister,
	chID charmstore.CharmID,
	csMac *macaroon.Macaroon,
	meta map[string]charmresource.Meta,
) (map[string]string, error) {
	filtered, err := getUpgradeResources(
		resourceLister,
		c.ApplicationName,
		c.Resources,
		meta,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(filtered) == 0 {
		return nil, nil
	}

	// Note: the validity of user-supplied resources to be uploaded will be
	// checked further down the stack.
	ids, err := c.DeployResources(
		c.ApplicationName,
		chID,
		csMac,
		c.Resources,
		filtered,
		apiRoot,
	)
	return ids, errors.Trace(err)
}

func getUpgradeResources(
	resourceLister ResourceLister,
	applicationID string,
	cliResources map[string]string,
	meta map[string]charmresource.Meta,
) (map[string]charmresource.Meta, error) {
	if len(meta) == 0 {
		return nil, nil
	}

	current, err := getResources(applicationID, resourceLister)
	if err != nil {
		return nil, errors.Trace(err)
	}
	filtered := filterResources(meta, current, cliResources)
	return filtered, nil
}

func getMetaResources(charmURL *charm.URL, client CharmClient) (map[string]charmresource.Meta, error) {
	charmInfo, err := client.CharmInfo(charmURL.String())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return charmInfo.Meta.Resources, nil
}

func getResources(applicationID string, resourceLister ResourceLister) (map[string]resource.Resource, error) {
	svcs, err := resourceLister.ListResources([]string{applicationID})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return resource.AsMap(svcs[0].Resources), nil
}

func filterResources(
	meta map[string]charmresource.Meta,
	current map[string]resource.Resource,
	uploads map[string]string,
) map[string]charmresource.Meta {
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

func newCharmAdder(
	api api.Connection,
) CharmAdder {
	return &apiClient{Client: api.Client()}
}

func getCharmStore(
	bakeryClient *httpbakery.Client,
	csURL string,
	channel csclientparams.Channel,
) charmrepoForDeploy {
	csClient := newCharmStoreClient(bakeryClient, csURL).WithChannel(channel)
	return charmrepo.NewCharmStoreFromClient(csClient)
}

// getCharmStoreAPIURL consults the controller config for the charmstore api url to use.
var getCharmStoreAPIURL = func(conAPIRoot base.APICallCloser) (string, error) {
	controllerAPI := controller.NewClient(conAPIRoot)
	controllerCfg, err := controllerAPI.ControllerConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	return controllerCfg.CharmStoreURL(), nil
}

type addCharmParams struct {
	charmAdder     CharmAdder
	authorizer     macaroonGetter
	charmRepo      charmrepoForDeploy
	oldURL         *charm.URL
	newCharmRef    string
	deployedSeries string
	force          bool
}

// addCharm interprets the new charmRef and adds the specified charm if
// the new charm is different to what's already deployed as specified by
// oldURL.
func (c *upgradeCharmCommand) addCharm(params addCharmParams) (charmstore.CharmID, *macaroon.Macaroon, error) {
	var id charmstore.CharmID
	// Charm may have been supplied via a path reference. If so, build a
	// local charm URL from the deployed series.
	ch, newURL, err := charmrepo.NewCharmAtPathForceSeries(params.newCharmRef, params.deployedSeries, c.ForceSeries)
	if err == nil {
		newName := ch.Meta().Name
		if newName != params.oldURL.Name {
			return id, nil, errors.Errorf("cannot upgrade %q to %q", params.oldURL.Name, newName)
		}
		addedURL, err := params.charmAdder.AddLocalCharm(newURL, ch, params.force)
		id.URL = addedURL
		return id, nil, err
	}
	if _, ok := err.(*charmrepo.NotFoundError); ok {
		return id, nil, errors.Errorf("no charm found at %q", params.newCharmRef)
	}
	// If we get a "not exists" or invalid path error then we attempt to interpret
	// the supplied charm reference as a URL below, otherwise we return the error.
	if err != os.ErrNotExist && !charmrepo.IsInvalidPathError(err) {
		return id, nil, err
	}

	refURL, err := charm.ParseURL(params.newCharmRef)
	if err != nil {
		return id, nil, errors.Trace(err)
	}

	// Charm has been supplied as a URL so we resolve and deploy using the store.
	newURL, channel, supportedSeries, err := c.ResolveCharm(params.charmRepo.ResolveWithPreferredChannel, refURL, c.Channel)
	if err != nil {
		return id, nil, errors.Trace(err)
	}
	id.Channel = channel
	_, seriesSupportedErr := charm.SeriesForCharm(params.deployedSeries, supportedSeries)
	if !c.ForceSeries && params.deployedSeries != "" && newURL.Series == "" && seriesSupportedErr != nil {
		series := []string{"no series"}
		if len(supportedSeries) > 0 {
			series = supportedSeries
		}
		return id, nil, errors.Errorf(
			"cannot upgrade from single series %q charm to a charm supporting %q. Use --force-series to override.",
			params.deployedSeries, series,
		)
	}
	// If no explicit revision was set with either SwitchURL
	// or Revision flags, discover the latest.
	if *newURL == *params.oldURL {
		if refURL.Revision != -1 {
			return id, nil, errors.Errorf("already running specified charm %q", newURL)
		}
		// No point in trying to upgrade a charm store charm when
		// we just determined that's the latest revision
		// available.
		return id, nil, errors.Errorf("already running latest charm %q", newURL)
	}

	curl, csMac, err := addCharmFromURL(params.charmAdder, params.authorizer, newURL, channel, params.force)
	if err != nil {
		return id, nil, errors.Trace(err)
	}
	id.URL = curl
	return id, csMac, nil
}

func allEndpoints(ci *charms.CharmInfo) set.Strings {
	epSet := set.NewStrings()
	for n := range ci.Meta.ExtraBindings {
		epSet.Add(n)
	}
	for n := range ci.Meta.Provides {
		epSet.Add(n)
	}
	for n := range ci.Meta.Peers {
		epSet.Add(n)
	}
	for n := range ci.Meta.Requires {
		epSet.Add(n)
	}

	return epSet
}
