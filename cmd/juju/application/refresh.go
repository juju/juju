// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	"github.com/juju/charmrepo/v7"
	csparams "github.com/juju/charmrepo/v7/csclient/params"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/application"
	apicharms "github.com/juju/juju/api/client/charms"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/api/client/resources"
	"github.com/juju/juju/api/client/spaces"
	commoncharm "github.com/juju/juju/api/common/charm"
	apicommoncharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/charmhub"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/application/deployer"
	"github.com/juju/juju/cmd/juju/application/refresher"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

func newRefreshCommand() *refreshCommand {
	return &refreshCommand{
		DeployResources: deployer.DeployResources,
		NewCharmAdder:   newCharmAdder,
		NewCharmClient: func(conn base.APICallCloser) utils.CharmClient {
			return apicharms.NewClient(conn)
		},
		NewCharmRefreshClient: func(conn base.APICallCloser) CharmRefreshClient {
			return application.NewClient(conn)
		},
		NewResourceLister: func(conn base.APICallCloser) (utils.ResourceLister, error) {
			resclient, err := resources.NewClient(conn)
			if err != nil {
				return nil, err
			}
			return resclient, nil
		},
		NewSpacesClient: func(conn base.APICallCloser) SpacesAPI {
			return spaces.NewAPI(conn)
		},
		ModelConfigClient: func(api base.APICallCloser) ModelConfigClient {
			return modelconfig.NewClient(api)
		},
		NewCharmHubClient: func(url string) (store.DownloadBundleClient, error) {
			return charmhub.NewClient(charmhub.Config{
				URL:    url,
				Logger: logger,
			})
		},
		CharmStoreURLGetter: getCharmStoreAPIURL,
		NewCharmStore: func(
			bakeryClient *httpbakery.Client,
			csURL string,
			channel csparams.Channel,
		) (store.MacaroonGetter, store.CharmrepoForDeploy) {
			return getCharmStore(bakeryClient, csURL, channel)
		},
		NewCharmResolver: func(apiRoot base.APICallCloser, charmrepo store.CharmrepoForDeploy, downloadClient store.DownloadBundleClient) CharmResolver {
			return store.NewCharmAdaptor(apicharms.NewClient(apiRoot),
				func() (store.CharmrepoForDeploy, error) {
					return charmrepo, nil
				},
				func() (store.DownloadBundleClient, error) {
					return downloadClient, nil
				},
			)
		},
		NewRefresherFactory: refresher.NewRefresherFactory,
	}
}

// CharmResolver defines methods required to resolve charms, as required
// by the refresh command.
type CharmResolver interface {
	ResolveCharm(url *charm.URL, preferredOrigin commoncharm.Origin, switchCHarm bool) (*charm.URL, commoncharm.Origin, []string, error)
}

// NewRefreshCommand returns a command which upgrades application's charm.
func NewRefreshCommand() cmd.Command {
	return modelcmd.Wrap(newRefreshCommand())
}

// CharmRefreshClient defines a subset of the application facade, as required
// by the refresh command.
type CharmRefreshClient interface {
	GetCharmURLOrigin(string, string) (*charm.URL, commoncharm.Origin, error)
	Get(string, string) (*params.ApplicationGetResults, error)
	SetCharm(string, application.SetCharmConfig) error
}

// NewCharmAdderFunc is the type of a function used to construct
// a new CharmAdder.
type NewCharmAdderFunc func(
	api.Connection,
) store.CharmAdder

// NewCharmStoreFunc constructs a charm store client.
type NewCharmStoreFunc func(
	*httpbakery.Client,
	string, // Charmstore API URL
	csparams.Channel,
) (store.MacaroonGetter, store.CharmrepoForDeploy)

// NewCharmResolverFunc returns a client implementing CharmResolver.
type NewCharmResolverFunc func(base.APICallCloser, store.CharmrepoForDeploy, store.DownloadBundleClient) CharmResolver

// RefreshCharm is responsible for upgrading an application's charm.
type refreshCommand struct {
	modelcmd.ModelCommandBase

	DeployResources       deployer.DeployResourcesFunc
	NewCharmAdder         NewCharmAdderFunc
	NewCharmStore         NewCharmStoreFunc
	NewCharmResolver      NewCharmResolverFunc
	NewCharmClient        func(base.APICallCloser) utils.CharmClient
	NewCharmRefreshClient func(base.APICallCloser) CharmRefreshClient
	NewResourceLister     func(base.APICallCloser) (utils.ResourceLister, error)
	NewSpacesClient       func(base.APICallCloser) SpacesAPI
	CharmStoreURLGetter   func(base.APICallCloser) (string, error)
	ModelConfigClient     func(base.APICallCloser) ModelConfigClient
	NewCharmHubClient     func(string) (store.DownloadBundleClient, error)
	NewRefresherFactory   func(refresher.RefresherDependencies) refresher.RefresherFactory

	ApplicationName string
	// Force should be ubiquitous and we should eventually deprecate both
	// ForceUnits and ForceBase; instead just using "force"
	Force      bool
	ForceUnits bool
	ForceBase  bool
	SwitchURL  string
	CharmPath  string
	Revision   int // defaults to -1 (latest)

	BindToSpaces string
	Bindings     map[string]string

	// Resources is a map of resource name to filename to be uploaded on upgrade.
	Resources map[string]string

	// Channel holds the charmstore or charmhub channel to use when obtaining
	// the charm to be refreshed to.
	Channel    charm.Channel
	channelStr string

	// Config is a config file variable, pointing at a YAML file containing
	// the application config to update.
	Config cmd.FileVar

	// Storage is a map of storage constraints, keyed on the storage name
	// defined in charm storage metadata, to add or update during upgrade.
	Storage map[string]storage.Constraints
}

const refreshDoc = `
When no options are set, the application's charm will be refreshed to the latest
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

  juju refresh foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml

Where bar and baz are resources named in the metadata for the foo charm.

Storage constraints may be added or updated at upgrade time by specifying
the --storage option, with the same format as specified in "juju deploy".
If new required storage is added by the new charm revision, then you must
specify constraints or the defaults will be applied.

  juju refresh foo --storage cache=ssd,10G

Charm settings may be added or updated at upgrade time by specifying the
--config option, pointing to a YAML-encoded application config file.

  juju refresh foo --config config.yaml

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
- Charms changing from CharmStore (cs: prefix) to CharmHub require a 
  homogeneous architecture for applications.

The new charm may add new relations and configuration settings.

--switch and --path are mutually exclusive.

--path and --revision are mutually exclusive. The revision of the updated charm
is determined by the contents of the charm at the specified path.

--switch and --revision are mutually exclusive. To specify a given revision
number with --switch, give it in the charm URL, for instance "cs:wordpress-5"
would specify revision number 5 of the wordpress charm.

Use of the --force-units option is not generally recommended; units upgraded
while in an error state will not have refreshed hooks executed, and may cause
unexpected behavior.

--force option for LXD Profiles is not generally recommended when upgrading an
application; overriding profiles on the container may cause unexpected
behavior.
`

func (c *refreshCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "refresh",
		Args:    "<application>",
		Purpose: "Refresh an application's charm.",
		Doc:     refreshDoc,
	})
}

func (c *refreshCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "Allow a charm to be refreshed which bypasses LXD profile allow list")
	f.BoolVar(&c.ForceUnits, "force-units", false, "Refresh all units immediately, even if in error state")
	f.StringVar(&c.channelStr, "channel", "", "Channel to use when getting the charm or bundle from the charm store or charm hub")
	f.BoolVar(&c.ForceBase, "force-series", false, "Refresh even if series of deployed applications are not supported by the new charm")
	f.StringVar(&c.SwitchURL, "switch", "", "Crossgrade to a different charm")
	f.StringVar(&c.CharmPath, "path", "", "Refresh to a charm located at path")
	f.IntVar(&c.Revision, "revision", -1, "Explicit revision of current charm")
	f.Var(stringMap{&c.Resources}, "resource", "Resource to be uploaded to the controller")
	f.Var(storageFlag{&c.Storage, nil}, "storage", "Charm storage constraints")
	f.Var(&c.Config, "config", "Path to yaml-formatted application config")
	f.StringVar(&c.BindToSpaces, "bind", "", "Configure application endpoint bindings to spaces")
}

func (c *refreshCommand) Init(args []string) error {
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
func (c *refreshCommand) Run(ctx *cmd.Context) error {
	apiRoot, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = apiRoot.Close() }()

	generation, err := c.ActiveBranch()
	if err != nil {
		return errors.Trace(err)
	}
	charmRefreshClient := c.NewCharmRefreshClient(apiRoot)
	oldURL, oldOrigin, err := charmRefreshClient.GetCharmURLOrigin(generation, c.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}

	// There is a timing window where deploy has been called and the charm
	// is not yet downloaded. Check here to verify the origin has an ID,
	// otherwise refresh result may be in accurate. We could use the
	// retry package, but the issue is only seen in automated test due to
	// speed. Can always use retry if it becomes an issue.
	if oldOrigin.Source == commoncharm.OriginCharmHub && oldOrigin.ID == "" {
		return errors.Errorf("%q deploy incomplete, please try refresh again in a little bit.", c.ApplicationName)
	}

	// Set a default URL schema for charm URLs that don't provide one.
	var defaultCharmSchema = charm.CharmHub

	// Ensure that the switchURL (if provided) always contains a schema. If
	// one is missing inject the default value we selected above.
	if c.SwitchURL != "" {
		// Don't prepend `ch:` when referring to a local charm
		if !refresher.IsLocalURL(c.SwitchURL) {
			if c.SwitchURL, err = charm.EnsureSchema(c.SwitchURL, defaultCharmSchema); err != nil {
				return errors.Trace(err)
			}
		}
	}

	if c.BindToSpaces != "" {
		if err := c.parseBindFlag(apiRoot); err != nil && errors.IsNotSupported(err) {
			ctx.Infof("Spaces not supported by this model's cloud, ignoring bindings.")
		} else if err != nil {
			return err
		}
	}

	newRef := c.SwitchURL
	if newRef == "" {
		newRef = c.CharmPath
	}
	if c.SwitchURL == "" && c.CharmPath == "" {
		// If the charm we are refreshing is local, then we must
		// specify a path or switch url to upgrade with.
		if oldURL.Schema == charm.Local.String() {
			return errors.New("upgrading a local charm requires either --path or --switch")
		}
		// No new URL specified, but revision might have been.
		newRef = oldURL.WithRevision(c.Revision).String()
	}

	applicationInfo, err := charmRefreshClient.Get(generation, c.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}

	// Only parse the channel here.  If the channel is normalized, the refresher
	// cannot determine the difference between the "latest" track and the current
	// track if only risk is specified.
	if c.channelStr == "" {
		c.Channel, _ = charm.ParseChannel(applicationInfo.Channel)
	} else {
		c.Channel, err = charm.ParseChannel(c.channelStr)
		if err != nil {
			return errors.Trace(err)
		}
	}

	chBase, err := series.ParseBase(applicationInfo.Base.Name, applicationInfo.Base.Channel)
	if err != nil {
		return errors.Trace(err)
	}
	cfg := refresher.RefresherConfig{
		ApplicationName: c.ApplicationName,
		CharmURL:        oldURL,
		CharmOrigin:     oldOrigin.CoreCharmOrigin(),
		CharmRef:        newRef,
		Channel:         c.Channel,
		DeployedBase:    chBase,
		Force:           c.Force,
		ForceBase:       c.ForceBase,
		Switch:          c.SwitchURL != "",
		Logger:          ctx,
	}
	factory, err := c.getRefresherFactory(apiRoot)
	if err != nil {
		return errors.Trace(err)
	}
	charmID, err := factory.Run(cfg)
	switch {
	case err == nil:
		curl := charmID.URL
		charmOrigin := charmID.Origin
		if charmOrigin.Source == corecharm.CharmStore {
			ctx.Warningf("Charm store charms, those with cs: before the charm name, will not be supported in juju 3.1.\n\tMigration of this model to a juju 3.1 controller will be prohibited.\n\tUse the --switch flag to refresh to a non charm store version of the charm.")
		}
		// The current charm URL that's been found and selected.
		channel := ""
		if charmOrigin.Source == corecharm.CharmHub || charmOrigin.Source == corecharm.CharmStore {
			channel = fmt.Sprintf(" in channel %s", charmID.Origin.Channel.String())
		}
		ctx.Infof("Added %s charm %q, revision %d%s, to the model", charmOrigin.Source, curl.Name, curl.Revision, channel)
	case errors.Is(err, refresher.ErrAlreadyUpToDate) && c.Channel.String() != oldOrigin.CoreCharmOrigin().Channel.String():
		ctx.Infof("%s. Note: all future refreshes will now use channel %q", err.Error(), charmID.Origin.Channel.String())
	case errors.Is(err, refresher.ErrAlreadyUpToDate) && len(c.Resources) == 0:
		// Charm already up-to-date and no resources to refresh.
		ctx.Infof(err.Error())
		return nil
	case errors.Is(err, refresher.ErrAlreadyUpToDate) && len(c.Resources) > 0:
		ctx.Infof("%s. Attempt to update resources requested.", err.Error())
	default:
		if termErr, ok := errors.Cause(err).(*common.TermsRequiredError); ok {
			return errors.Trace(termErr.UserErr())
		}
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	// Next, upgrade resources.
	resourceLister, err := c.NewResourceLister(apiRoot)
	if err != nil {
		return errors.Trace(err)
	}
	curl := charmID.URL
	charmsClient := c.NewCharmClient(apiRoot)
	charmInfo, err := charmsClient.CharmInfo(curl.String())
	if err != nil {
		return errors.Trace(err)
	}
	origin, err := commoncharm.CoreCharmOrigin(charmID.Origin)
	if err != nil {
		return errors.Trace(err)
	}
	chID := application.CharmID{
		URL:    curl,
		Origin: origin,
	}
	resourceIDs, err := c.upgradeResources(apiRoot, resourceLister, chID, charmID.Macaroon, charmInfo.Meta.Resources)
	if err != nil {
		return errors.Trace(err)
	}

	// Print out the updated endpoint binding plan.
	var bindingsChangelog []string
	curBindings := applicationInfo.EndpointBindings
	appDefaultSpace := curBindings[""]
	newCharmEndpoints := allEndpoints(charmInfo)
	if err := c.validateEndpointNames(newCharmEndpoints, curBindings, c.Bindings); err != nil {
		return errors.Trace(err)
	}
	c.Bindings, bindingsChangelog = mergeBindings(newCharmEndpoints, curBindings, c.Bindings, appDefaultSpace)

	// Finally, upgrade the application.
	var configYAML []byte
	if c.Config.Path != "" {
		configYAML, err = c.Config.Read(ctx)
		if err != nil {
			return errors.Trace(err)
		}
	}
	charmCfg := application.SetCharmConfig{
		ApplicationName:    c.ApplicationName,
		CharmID:            chID,
		ConfigSettingsYAML: string(configYAML),
		Force:              c.Force,
		ForceBase:          c.ForceBase,
		ForceUnits:         c.ForceUnits,
		ResourceIDs:        resourceIDs,
		StorageConstraints: c.Storage,
		EndpointBindings:   c.Bindings,
	}

	if err := block.ProcessBlockedError(charmRefreshClient.SetCharm(generation, charmCfg), block.BlockChange); err != nil {
		return err
	}

	// Emit binding changelog after a successful call to SetCharm.
	for _, change := range bindingsChangelog {
		ctx.Infof(change)
	}

	return nil
}

func (c *refreshCommand) validateEndpointNames(newCharmEndpoints set.Strings, oldEndpointsMap, userBindings map[string]string) error {
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

func (c *refreshCommand) parseBindFlag(apiRoot base.APICallCloser) error {
	if c.BindToSpaces == "" {
		return nil
	}

	// Fetch known spaces from server
	knownSpaces, err := c.NewSpacesClient(apiRoot).ListSpaces()
	if err != nil {
		return errors.Trace(err)
	}

	knownSpaceNames := set.NewStrings()
	for _, space := range knownSpaces {
		knownSpaceNames.Add(space.Name)
	}

	// Parse expression
	bindings, err := parseBindExpr(c.BindToSpaces, knownSpaceNames)
	if err != nil {
		return errors.Trace(err)
	}

	c.Bindings = bindings
	return nil
}

// upgradeResources pushes metadata up to the server for each resource defined
// in the new charm's metadata and returns a map of resource names to pending
// IDs to include in the refresh call.
//
// TODO(axw) apiRoot is passed in here because DeployResources requires it,
// DeployResources should accept a resource-specific client instead.
func (c *refreshCommand) upgradeResources(
	apiRoot base.APICallCloser,
	resourceLister utils.ResourceLister,
	chID application.CharmID,
	csMac *macaroon.Macaroon,
	meta map[string]charmresource.Meta,
) (map[string]string, error) {
	filtered, err := utils.GetUpgradeResources(
		chID.URL,
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
		resources.CharmID{
			URL:    chID.URL,
			Origin: chID.Origin,
		},
		csMac,
		c.Resources,
		filtered,
		apiRoot,
		c.Filesystem(),
	)
	return ids, errors.Trace(err)
}

func newCharmAdder(
	conn api.Connection,
) store.CharmAdder {
	return &charmAdderShim{
		api:               &apiClient{Client: apiclient.NewClient(conn)},
		charmsClient:      &charmsClient{Client: apicharms.NewClient(conn)},
		modelConfigClient: &modelConfigClient{Client: modelconfig.NewClient(conn)},
	}
}

type charmAdderShim struct {
	*charmsClient
	*modelConfigClient
	api *apiClient
}

func (c *charmAdderShim) AddLocalCharm(curl *charm.URL, ch charm.Charm, force bool) (*charm.URL, error) {
	agentVersion, err := agentVersion(c.modelConfigClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.charmsClient.AddLocalCharm(curl, ch, force, agentVersion)
}

func getCharmStore(
	bakeryClient *httpbakery.Client,
	csURL string,
	channel csparams.Channel,
) (store.MacaroonGetter, store.CharmrepoForDeploy) {
	csClient := store.NewCharmStoreClient(bakeryClient, csURL).WithChannel(channel)
	return csClient, charmrepo.NewCharmStoreFromClient(csClient)
}

// getCharmStoreAPIURL consults the controller config for the charmstore api url
// to use.
var getCharmStoreAPIURL = func(conAPIRoot base.APICallCloser) (string, error) {
	controllerAPI := controller.NewClient(conAPIRoot)
	controllerCfg, err := controllerAPI.ControllerConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	return controllerCfg.CharmStoreURL(), nil
}

func allEndpoints(ci *apicommoncharms.CharmInfo) set.Strings {
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

func (c *refreshCommand) getRefresherFactory(apiRoot api.Connection) (refresher.RefresherFactory, error) {
	// First, ensure the charm is added to the model.
	conAPIRoot, err := c.NewControllerAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	csURL, err := c.CharmStoreURLGetter(conAPIRoot)
	if err != nil {
		return nil, errors.Trace(err)
	}
	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	csClient, charmStore := c.NewCharmStore(bakeryClient, csURL, csparams.Channel(c.Channel.Risk))

	charmHubURL, err := c.getCharmHubURL(apiRoot)
	if err != nil {
		return nil, errors.Trace(err)
	}

	downloadClient, err := c.NewCharmHubClient(charmHubURL)
	if err != nil {
		return nil, errors.Trace(err)
	}

	deps := refresher.RefresherDependencies{
		Authorizer:    csClient,
		CharmAdder:    c.NewCharmAdder(apiRoot),
		CharmResolver: c.NewCharmResolver(apiRoot, charmStore, downloadClient),
	}
	return c.NewRefresherFactory(deps), nil
}

func (c *refreshCommand) getCharmHubURL(apiRoot base.APICallCloser) (string, error) {
	modelConfigClient := c.ModelConfigClient(apiRoot)

	attrs, err := modelConfigClient.ModelGet()
	if err != nil {
		return "", errors.Trace(err)
	}

	config, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return "", errors.Trace(err)
	}

	charmHubURL, _ := config.CharmHubURL()
	return charmHubURL, nil
}
