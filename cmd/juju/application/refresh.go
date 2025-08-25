// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"strconv"

	"github.com/juju/charm/v12"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

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
	"github.com/juju/juju/charmhub"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/application/deployer"
	"github.com/juju/juju/cmd/juju/application/refresher"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
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
		NewCharmResolver: func(apiRoot base.APICallCloser, downloadClient store.DownloadBundleClient) CharmResolver {
			return store.NewCharmAdaptor(apicharms.NewClient(apiRoot),
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
	ResolveCharm(url *charm.URL, preferredOrigin commoncharm.Origin, switchCHarm bool) (*charm.URL, commoncharm.Origin, []corebase.Base, error)
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
) (store.CharmAdder, error)

// NewCharmResolverFunc returns a client implementing CharmResolver.
type NewCharmResolverFunc func(base.APICallCloser, store.DownloadBundleClient) CharmResolver

// RefreshCharm is responsible for upgrading an application's charm.
type refreshCommand struct {
	modelcmd.ModelCommandBase

	DeployResources       deployer.DeployResourcesFunc
	NewCharmAdder         NewCharmAdderFunc
	NewCharmResolver      NewCharmResolverFunc
	NewCharmClient        func(base.APICallCloser) utils.CharmClient
	NewCharmRefreshClient func(base.APICallCloser) CharmRefreshClient
	NewResourceLister     func(base.APICallCloser) (utils.ResourceLister, error)
	NewSpacesClient       func(base.APICallCloser) SpacesAPI
	ModelConfigClient     func(base.APICallCloser) ModelConfigClient
	NewCharmHubClient     func(string) (store.DownloadBundleClient, error)
	NewRefresherFactory   func(refresher.RefresherDependencies) refresher.RefresherFactory

	ApplicationName string

	// Base represents the base (eg ubuntu@22.04) of the new charm to use
	Base string
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

	// Channel holds the charmhub channel to use when obtaining
	// the charm to be refreshed to.
	Channel    charm.Channel
	channelStr string

	// ConfigOptions records k=v attributes from command arguments
	// and/or specified files containing key values.
	ConfigOptions common.ConfigFlag

	// Storage is a map of storage constraints, keyed on the storage name
	// defined in charm storage metadata, to add or update during upgrade.
	Storage map[string]storage.Constraints

	// Trust signifies that the charm should have access to trusted credentials.
	// That is, hooks run by the charm can access cloud credentials and other
	// trusted access credentials.
	Trust *bool
}

const refreshDoc = `
When no options are set, the application's charm will be refreshed to the latest
revision available in the repository from which it was originally deployed. An
explicit revision can be chosen with the ` + "`--revision`" + ` option.

A path will need to be supplied to allow an updated copy of the charm
to be located.

Deploying from a path is intended to suit the workflow of a charm author working
on a single client machine; use of this deployment method from multiple clients
is not supported and may lead to confusing behaviour. Each local charm gets
uploaded with the revision specified in the charm, if possible, otherwise it
gets a unique revision (highest in state + 1).

When deploying from a path, the ` + "`--path`" + ` option is used to specify the location from
which to load the updated charm. Note that the directory containing the charm must
match what was originally used to deploy the charm as a superficial check that the
updated charm is compatible.

Resources may be uploaded at upgrade time by specifying the ` + "`--resource`" + ` option.
Following the resource option should be name=filepath pair.  This option may be
repeated more than once to upload more than one resource.

    juju refresh foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml

Where bar and baz are resources named in the metadata for the foo charm.

Storage constraints may be added or updated at upgrade time by specifying
the ` + "`--storage`" + ` option, with the same format as specified in ` + "`juju deploy`" + `.
If new required storage is added by the new charm revision, then you must
specify constraints or the defaults will be applied.

    juju refresh foo --storage cache=ssd,10G

Charm settings may be added or updated at upgrade time by specifying the
` + "`--config`" + ` option, pointing to a ` + "`YAML`" + `-encoded application config file.

    juju refresh foo --config config.yaml

If the new version of a charm does not explicitly support the application's series, the
upgrade is disallowed unless the --force-series option is used. This option should be
used with caution since using a charm on a machine running an unsupported series may
cause unexpected behavior.

The ` + "`--switch`" + ` option allows you to replace the charm with an entirely different one.
The new charm's URL and revision are inferred as they would be when running a
deploy command.

Please note that ` + "`--switch`" + ` is dangerous, because juju only has limited
information with which to determine compatibility; the operation will succeed,
regardless of potential havoc, so long as the following conditions hold:

- The new charm must declare all relations that the application is currently
  participating in.
- All config settings shared by the old and new charms must
  have the same types.

The new charm may add new relations and configuration settings.

The new charm may also need to be granted access to trusted credentials.
Use ` + "`--trust`" + ` to grant such access.
Or use ` + "`--trust=false`" + ` to revoke such access.

` + "`--switch`" + ` and ` + "`--path`" + ` are mutually exclusive.

` + "`--path`" + ` and ` + "`--revision`" + ` are mutually exclusive. The revision of the updated charm
is determined by the contents of the charm at the specified path.

` + "`--switch`" + ` and ` + "`--revision`" + ` are mutually exclusive.

Use of the ` + "`--force-units`" + ` option is not generally recommended; units upgraded
while in an error state will not have ` + "`upgrade-charm`" + ` hooks executed, and may
cause unexpected behavior.

` + "`--force`" + ` option for LXD Profiles is not generally recommended when upgrading an
application; overriding profiles on the container may cause unexpected
behavior.
`

const refreshExamples = `
To refresh the storage constraints for application ` + "`foo`" + `:

	juju refresh foo --storage cache=ssd,10G

To refresh the application config from a file for application ` + "`foo`" + `:

	juju refresh foo --config config.yaml

To refresh the resources for application ` + "`foo`" + `:

	juju refresh foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml
`

const upgradedApplicationHasUnitsMessage = `
Upgrading from an older PodSpec style charm to a newer Sidecar charm requires that
the application be scaled down to 0 units.

Before refreshing the application again, you must scale it to 0 units and wait for
all those units to disappear before continuing.

	juju scale-application %s 0
`

func (c *refreshCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "refresh",
		Args:     "<application>",
		Purpose:  "Refresh an application's charm.",
		Doc:      refreshDoc,
		SeeAlso:  []string{"deploy"},
		Examples: refreshExamples,
	})
}

func (c *refreshCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "Allow a charm to be refreshed which bypasses LXD profile allow list")
	f.BoolVar(&c.ForceUnits, "force-units", false, "Refresh all units immediately, even if in error state")
	f.StringVar(&c.channelStr, "channel", "", "Channel to use when getting the charm from Charmhub")
	f.BoolVar(&c.ForceBase, "force-series", false, "Refresh even if the series of the deployed application is not supported by the new charm. DEPRECATED: use --force-base")
	f.BoolVar(&c.ForceBase, "force-base", false, "Refresh even if the base of the deployed application is not supported by the new charm")
	f.StringVar(&c.SwitchURL, "switch", "", "Crossgrade to a different charm")
	f.StringVar(&c.CharmPath, "path", "", "Refresh to a charm located at path")
	f.StringVar(&c.Base, "base", "", "Select a different base than what is currently running.")
	f.IntVar(&c.Revision, "revision", -1, "Explicit revision of current charm")
	f.Var(stringMap{&c.Resources}, "resource", "Resource to be uploaded to the controller")
	f.Var(storageFlag{&c.Storage, nil}, "storage", "Charm storage constraints")
	f.Var(&c.ConfigOptions, "config", "Either a path to yaml-formatted application config file or a key=value pair ")
	f.StringVar(&c.BindToSpaces, "bind", "", "Configure application endpoint bindings to spaces")
	f.Var(newOptBoolValue(&c.Trust), "trust", "Allows charm to run hooks that require access credentials")
}

type optBoolValue struct {
	target **bool
}

func newOptBoolValue(p **bool) *optBoolValue {
	return &optBoolValue{
		target: p,
	}
}

func (b *optBoolValue) Set(s string) error {
	v, err := strconv.ParseBool(s)
	*b.target = &v
	return err
}

func (b *optBoolValue) Get() interface{} {
	if *b.target != nil {
		return **b.target
	}
	return "unset"
}

func (b *optBoolValue) String() string {
	return fmt.Sprintf("%v", b.Get())
}

func (b *optBoolValue) IsBoolFlag() bool {
	return true
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
	newOrigin := oldOrigin
	if c.Base != "" {
		if newBase, err := corebase.ParseBaseFromString(c.Base); err != nil {
			return errors.Trace(err)
		} else {
			newOrigin.Base.OS = newBase.OS
			newOrigin.Base.Channel = newBase.Channel

		}
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
			return errors.New("refreshing a local charm requires either --path or --switch")
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

	cfg := refresher.RefresherConfig{
		ApplicationName: c.ApplicationName,
		CharmURL:        oldURL,
		CharmOrigin:     newOrigin.CoreCharmOrigin(),
		CharmRef:        newRef,
		Channel:         c.Channel,
		Force:           c.Force,
		ForceBase:       c.ForceBase,
		// If revision is supplied by the user, treat it as a switch operation,
		// the revision has already been added to the "newRef" above.
		Switch: c.SwitchURL != "" || c.Revision != -1,
		Logger: ctx,
	}
	factory, err := c.getRefresherFactory(apiRoot)
	if err != nil {
		return errors.Trace(err)
	}
	charmID, runErr := factory.Run(cfg)
	if runErr != nil && !errors.Is(runErr, refresher.ErrAlreadyUpToDate) {
		// Process errors.Is(runErr, refresher.ErrAlreadyUpToDate) after reviewing resources.
		if termErr, ok := errors.Cause(runErr).(*common.TermsRequiredError); ok {
			return errors.Trace(termErr.UserErr())
		}
		return block.ProcessBlockedError(runErr, block.BlockChange)
	}
	curl := charmID.URL
	charmOrigin := charmID.Origin
	if runErr == nil {
		// The current charm URL that's been found and selected.
		channel := ""
		if charmOrigin.Source == corecharm.CharmHub {
			channel = fmt.Sprintf(" in channel %s", charmID.Origin.Channel.String())
		}
		ctx.Infof("Added %s charm %q, revision %d%s, to the model", charmOrigin.Source, curl.Name, curl.Revision, channel)
	}

	// Next, upgrade resources.
	origin, err := commoncharm.CoreCharmOrigin(charmID.Origin)
	if err != nil {
		return errors.Trace(err)
	}
	chID := application.CharmID{
		URL:    curl.String(),
		Origin: origin,
	}
	resourceIDs, err := c.upgradeResources(apiRoot, chID)
	if err != nil {
		return errors.Trace(err)
	}
	// Process the factory Run error from above where the charm itself is
	// already up-to-date. There are 2 scenarios where we should continue.
	// 1. There is a change to the charm's channel.
	// 2. There is a resource change to process.
	if errors.Is(runErr, refresher.ErrAlreadyUpToDate) {
		ctx.Infof("%s", runErr.Error())
		if len(resourceIDs) == 0 && c.Channel.String() == oldOrigin.CoreCharmOrigin().Channel.String() {
			return nil
		}
		if c.Channel.String() != oldOrigin.CoreCharmOrigin().Channel.String() {
			ctx.Infof("Note: all future refreshes will now use channel %q", charmID.Origin.Channel.String())
		}
		if len(resourceIDs) > 0 {
			ctx.Infof("resources to be upgraded")
		}
	}

	// Print out the updated endpoint binding plan.
	charmsClient := c.NewCharmClient(apiRoot)
	charmInfo, err := charmsClient.CharmInfo(curl.String())
	if err != nil {
		return errors.Trace(err)
	}
	var bindingsChangelog []string
	curBindings := applicationInfo.EndpointBindings
	appDefaultSpace := curBindings[""]
	newCharmEndpoints := allEndpoints(charmInfo)
	if err := c.validateEndpointNames(newCharmEndpoints, curBindings, c.Bindings); err != nil {
		return errors.Trace(err)
	}
	c.Bindings, bindingsChangelog = mergeBindings(newCharmEndpoints, curBindings, c.Bindings, appDefaultSpace)

	// Finally, upgrade the application.
	appConfig, configYAML, err := utils.ProcessConfig(ctx, c.Filesystem(), &c.ConfigOptions, c.Trust)
	if err != nil {
		return errors.Trace(err)
	}
	charmCfg := application.SetCharmConfig{
		ApplicationName:    c.ApplicationName,
		CharmID:            chID,
		ConfigSettings:     appConfig,
		ConfigSettingsYAML: configYAML,
		Force:              c.Force,
		ForceBase:          c.ForceBase,
		ForceUnits:         c.ForceUnits,
		ResourceIDs:        resourceIDs,
		StorageConstraints: c.Storage,
		EndpointBindings:   c.Bindings,
	}

	err = charmRefreshClient.SetCharm(generation, charmCfg)
	err = block.ProcessBlockedError(err, block.BlockChange)
	if params.IsCodeAppShouldNotHaveUnits(err) {
		return errors.Errorf(upgradedApplicationHasUnitsMessage[1:], c.ApplicationName)
	} else if err != nil {
		return err
	}

	// Emit binding changelog after a successful call to SetCharm.
	for _, change := range bindingsChangelog {
		ctx.Infof("%s", change)
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
	chID application.CharmID,
) (map[string]string, error) {
	resourceLister, err := c.NewResourceLister(apiRoot)
	if err != nil {
		return nil, errors.Trace(err)
	}
	charmsClient := c.NewCharmClient(apiRoot)
	meta, err := utils.GetMetaResources(chID.URL, charmsClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	filtered, err := utils.GetUpgradeResources(
		chID,
		charmsClient,
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
		c.Resources,
		filtered,
		apiRoot,
		c.Filesystem(),
	)
	return ids, errors.Trace(err)
}

func newCharmAdder(
	conn api.Connection,
) (store.CharmAdder, error) {
	localCharmsClient, err := apicharms.NewLocalCharmClient(conn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &charmAdderShim{
		api:               apiclient.NewClient(conn, logger),
		charmsClient:      apicharms.NewClient(conn),
		localCharmsClient: localCharmsClient,
		modelConfigClient: modelconfig.NewClient(conn),
	}, nil
}

type charmAdderShim struct {
	*charmsClient
	*localCharmsClient
	*modelConfigClient
	api *apiclient.Client
}

func (c *charmAdderShim) AddLocalCharm(curl *charm.URL, ch charm.Charm, force bool) (*charm.URL, error) {
	agentVersion, err := agentVersion(c.modelConfigClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.localCharmsClient.AddLocalCharm(curl, ch, force, agentVersion)
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
	charmHubURL, err := c.getCharmHubURL(apiRoot)
	if err != nil {
		return nil, errors.Trace(err)
	}

	downloadClient, err := c.NewCharmHubClient(charmHubURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	charmAdder, err := c.NewCharmAdder(apiRoot)
	if err != nil {
		return nil, errors.Trace(err)
	}

	deps := refresher.RefresherDependencies{
		CharmAdder:    charmAdder,
		CharmResolver: c.NewCharmResolver(apiRoot, downloadClient),
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
