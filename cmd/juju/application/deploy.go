// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strconv"
	"strings"

	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/annotations"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/applicationoffers"
	"github.com/juju/juju/api/base"
	apicharms "github.com/juju/juju/api/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/api/spaces"
	apiparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/application/deployer"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/storage"
)

// SpacesAPI defines the necessary API methods needed for listing spaces.
type SpacesAPI interface {
	ListSpaces() ([]apiparams.Space, error)
}

var supportedJujuSeries = series.WorkloadSeries

type DeployAPI interface {
	deployer.DeployerAPI
	SpacesAPI
	// PlanURL returns the configured URL prefix for the metering plan API.
	PlanURL() string
}

// The following structs exist purely because Go cannot create a
// struct with a field named the same as a method name. The DeployAPI
// needs to both embed a *<package>.Client and provide the
// api.Connection Client method.
//
// Once we pair down DeployAPI, this will not longer be a problem.

type apiClient struct {
	*api.Client
}

type charmsClient struct {
	*apicharms.Client
}

type applicationClient struct {
	*application.Client
}

type modelConfigClient struct {
	*modelconfig.Client
}

type annotationsClient struct {
	*annotations.Client
}

type plansClient struct {
	planURL string
}

func (c *plansClient) PlanURL() string {
	return c.planURL
}

type offerClient struct {
	*applicationoffers.Client
}

type spacesClient struct {
	*spaces.API
}

type deployAPIAdapter struct {
	charmsAPIVersion int
	api.Connection
	*apiClient
	*charmsClient
	*applicationClient
	*modelConfigClient
	*annotationsClient
	*plansClient
	*offerClient
	*spacesClient
}

func (a *deployAPIAdapter) Client() *api.Client {
	return a.apiClient.Client
}

func (a *deployAPIAdapter) ModelUUID() (string, bool) {
	return a.apiClient.ModelUUID()
}

func (a *deployAPIAdapter) WatchAll() (api.AllWatch, error) {
	return a.apiClient.WatchAll()
}

func (a *deployAPIAdapter) Deploy(args application.DeployArgs) error {
	for i, p := range args.Placement {
		if p.Scope == "model-uuid" {
			p.Scope = a.applicationClient.ModelUUID()
		}
		args.Placement[i] = p
	}

	return errors.Trace(a.applicationClient.Deploy(args))
}

func (a *deployAPIAdapter) SetAnnotation(annotations map[string]map[string]string) ([]apiparams.ErrorResult, error) {
	return a.annotationsClient.Set(annotations)
}

func (a *deployAPIAdapter) GetAnnotations(tags []string) ([]apiparams.AnnotationsGetResult, error) {
	return a.annotationsClient.Get(tags)
}

func (a *deployAPIAdapter) AddCharm(curl *charm.URL, origin commoncharm.Origin, force bool) (commoncharm.Origin, error) {
	if a.charmsAPIVersion > 2 {
		return a.charmsClient.AddCharm(curl, origin, force)
	}
	return origin, a.apiClient.AddCharm(curl, csparams.Channel(origin.Risk), force)
}

func (a *deployAPIAdapter) AddCharmWithAuthorization(curl *charm.URL, origin commoncharm.Origin, mac *macaroon.Macaroon, force bool) (commoncharm.Origin, error) {
	if a.charmsAPIVersion > 2 {
		return a.charmsClient.AddCharmWithAuthorization(curl, origin, mac, force)
	}
	return origin, a.apiClient.AddCharmWithAuthorization(curl, csparams.Channel(origin.Risk), mac, force)
}

// NewDeployCommand returns a command to deploy applications.
func NewDeployCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(newDeployCommand())
}

func newDeployCommand() *DeployCommand {
	deployCmd := &DeployCommand{
		Steps: deployer.Steps(),
	}
	deployCmd.NewCharmRepo = func() (*store.CharmStoreAdaptor, error) {
		controllerAPIRoot, err := deployCmd.NewControllerAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		url, err := getCharmStoreAPIURL(controllerAPIRoot)
		if err != nil {
			return nil, errors.Trace(err)
		}
		bakeryClient, err := deployCmd.BakeryClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return store.NewCharmStoreAdaptor(bakeryClient, url), nil
	}
	deployCmd.NewModelConfigClient = func(api base.APICallCloser) ModelConfigClient {
		return modelconfig.NewClient(api)
	}
	deployCmd.NewDownloadClient = func() (store.DownloadBundleClient, error) {
		apiRoot, err := deployCmd.ModelCommandBase.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}

		charmHubURL, err := deployCmd.getCharmHubURL(apiRoot)
		if err != nil {
			return nil, errors.Trace(err)
		}

		cfg, err := charmhub.CharmHubConfigFromURL(charmHubURL, logger)
		if err != nil {
			return nil, errors.Trace(err)
		}

		return charmhub.NewClient(cfg)
	}
	deployCmd.NewAPIRoot = func() (DeployAPI, error) {
		apiRoot, err := deployCmd.ModelCommandBase.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		controllerAPIRoot, err := deployCmd.NewControllerAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		mURL, err := deployCmd.getMeteringAPIURL(controllerAPIRoot)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return &deployAPIAdapter{
			Connection:        apiRoot,
			apiClient:         &apiClient{Client: apiRoot.Client()},
			charmsClient:      &charmsClient{Client: apicharms.NewClient(apiRoot)},
			charmsAPIVersion:  apiRoot.BestFacadeVersion("Charms"),
			applicationClient: &applicationClient{Client: application.NewClient(apiRoot)},
			modelConfigClient: &modelConfigClient{Client: modelconfig.NewClient(apiRoot)},
			annotationsClient: &annotationsClient{Client: annotations.NewClient(apiRoot)},
			plansClient:       &plansClient{planURL: mURL},
			offerClient:       &offerClient{Client: applicationoffers.NewClient(controllerAPIRoot)},
			spacesClient:      &spacesClient{API: spaces.NewAPI(apiRoot)},
		}, nil
	}
	deployCmd.NewConsumeDetailsAPI = func(url *charm.OfferURL) (deployer.ConsumeDetails, error) {
		root, err := deployCmd.CommandBase.NewAPIRoot(deployCmd.ClientStore(), url.Source, "")
		if err != nil {
			return nil, errors.Trace(err)
		}
		return applicationoffers.NewClient(root), nil
	}
	deployCmd.NewDeployerFactory = deployer.NewDeployerFactory
	deployCmd.NewResolver = func(charmsAPI store.CharmsAPI, charmRepoFn store.CharmStoreRepoFunc, downloadClientFn store.DownloadBundleClientFunc) deployer.Resolver {
		return store.NewCharmAdaptor(charmsAPI, charmRepoFn, downloadClientFn)
	}
	return deployCmd
}

type DeployCommand struct {
	modelcmd.ModelCommandBase
	UnitCommandBase

	// CharmOrBundle is either a charm URL, a path where a charm can be found,
	// or a bundle name.
	CharmOrBundle string

	// BundleOverlay refers to config files that specify additional bundle
	// configuration to be merged with the main bundle.
	BundleOverlayFile []string

	// Channel holds the channel to use when obtaining
	// the charm to be deployed.
	Channel corecharm.Channel

	channelStr string

	// Series is the series of the charm to deploy.
	Series string

	// Force is used to allow a charm/bundle to be deployed onto a machine
	// running an unsupported series.
	Force bool

	// DryRun is used to specify that the bundle shouldn't actually be
	// deployed but just output the changes.
	DryRun bool

	ApplicationName string
	ConfigOptions   common.ConfigFlag
	ConstraintsStr  string
	Constraints     constraints.Value
	BindToSpaces    string

	// TODO(axw) move this to UnitCommandBase once we support --storage
	// on add-unit too.
	//
	// Storage is a map of storage constraints, keyed on the storage name
	// defined in charm storage metadata.
	Storage map[string]storage.Constraints

	// BundleStorage maps application names to maps of storage constraints keyed on
	// the storage name defined in that application's charm storage metadata.
	BundleStorage map[string]map[string]storage.Constraints

	// Devices is a mapping of device constraints, keyed on the device name
	// defined in charm devices metadata.
	Devices map[string]devices.Constraints

	// BundleDevices maps application names to maps of device constraints keyed on
	// the device name defined in that application's charm devices metadata.
	BundleDevices map[string]map[string]devices.Constraints

	// Resources is a map of resource name to filename to be uploaded on deploy.
	Resources map[string]string

	Bindings map[string]string
	Steps    []deployer.DeployStep

	// UseExisting machines when deploying the bundle.
	UseExisting bool

	// BundleMachines is a mapping for machines in the bundle to machines
	// in the model.
	BundleMachines map[string]string

	// NewAPIRoot stores a function which returns a new API root.
	NewAPIRoot func() (DeployAPI, error)

	// NewCharmRepo stores a function which returns a charm store client.
	NewCharmRepo func() (*store.CharmStoreAdaptor, error)

	// NewDownloadClient stores a function for getting a charm/bundle.
	NewDownloadClient func() (store.DownloadBundleClient, error)

	// NewModelConfigClient stores a function which returns a new model config
	// client. This is used to get the model config.
	NewModelConfigClient func(base.APICallCloser) ModelConfigClient

	// NewResolver stores a function which returns a charm adaptor.
	NewResolver func(store.CharmsAPI, store.CharmStoreRepoFunc, store.DownloadBundleClientFunc) deployer.Resolver

	// NewDeployerFactory stores a function which returns a deployer factory.
	NewDeployerFactory func(dep deployer.DeployerDependencies) deployer.DeployerFactory

	// NewConsumeDetailsAPI stores a function which will return a new API
	// for consume details API using the url as the source.
	NewConsumeDetailsAPI func(url *charm.OfferURL) (deployer.ConsumeDetails, error)

	// DeployResources stores a function which deploys charm resources.
	DeployResources resourceadapters.DeployResourcesFunc

	// When deploying a charm, Trust signifies that the charm should be
	// deployed with access to trusted credentials. That is, hooks run by
	// the charm can access cloud credentials and other trusted access
	// credentials. On the other hand, when deploying a bundle, Trust
	// signifies that each application from the bundle that requires access
	// to trusted credentials will be granted access.
	Trust      bool
	machineMap string
	flagSet    *gnuflag.FlagSet

	unknownModel bool
}

// TODO (stickupkid): Update/re-write the following doc for charmhub related
// charm urls.
const deployDoc = `
A charm or bundle can be referred to by its simple name and a series or channel
can optionally be specified:

  juju deploy cs:postgresql
  juju deploy cs:bionic/postgresql
  juju deploy cs:postgresql --series bionic
  juju deploy cs:postgresql --channel edge

All the above deployments use remote charms found in the Charm Store (denoted
by 'cs' prefix) and therefore also make use of "charm URLs".

If a channel is specified, it will be used as the source for looking up the
charm or bundle from the Charm Store. When used in a bundle deployment context,
the specified channel is only used for retrieving the bundle and is ignored when
looking up the charms referenced by the bundle. However, each charm within a
bundle is allowed to explicitly specify the channel used to look it up.

A versioned charm URL will be expanded as expected. For example, 'mysql-56'
becomes 'cs:bionic/mysql-56'.

A local charm may be deployed by giving the path to its directory:

  juju deploy /path/to/charm
  juju deploy /path/to/charm --series bionic

You will need to be explicit if there is an ambiguity between a local and a
remote charm:

  juju deploy ./pig
  juju deploy cs:pig

An error is emitted if the determined series is not supported by the charm. Use
the '--force' option to override this check:

  juju deploy charm --series bionic --force

A bundle can be expressed similarly to a charm, but not by series:

  juju deploy mediawiki-single
  juju deploy bundle/mediawiki-single
  juju deploy cs:bundle/mediawiki-single

A local bundle may be deployed by specifying the path to its YAML file:

  juju deploy /path/to/bundle.yaml

The final charm/machine series is determined using an order of precedence (most
preferred to least):

 - the '--series' command option
 - the series stated in the charm URL
 - for a bundle, the series stated in each charm URL (in the bundle file)
 - for a bundle, the series given at the top level (in the bundle file)
 - the 'default-series' model key
 - the top-most series specified in the charm's metadata file
   (this sets the charm's 'preferred series' in the Charm Store)

An 'application name' provides an alternate name for the application. It works
only for charms; it is silently ignored for bundles (although the same can be
done at the bundle file level). Such a name must consist only of lower-case
letters (a-z), numbers (0-9), and single hyphens (-). The name must begin with
a letter and not have a group of all numbers follow a hyphen:

  Valid:   myappname, custom-app, app2-scat-23skidoo
  Invalid: myAppName, custom--app, app2-scat-23, areacode-555-info

Use the '--constraints' option to specify hardware requirements for new machines.
These become the application's default constraints (i.e. they are used if the
application is later scaled out with the ` + "`add-unit`" + ` command). To overcome this
behaviour use the ` + "`set-constraints`" + ` command to change the application's default
constraints or add a machine (` + "`add-machine`" + `) with a certain constraint and then
target that machine with ` + "`add-unit`" + ` by using the '--to' option.

Use the '--device' option to specify GPU device requirements (with Kubernetes).
The below format is used for this option's value, where the 'label' is named in
the charm metadata file:

  <label>=[<count>,]<device-class>|<vendor/type>[,<attributes>]

Use the '--config' option to specify application configuration values. This
option accepts either a path to a YAML-formatted file or a key=value pair. A
file should be of this format:

  <charm name>:
	<option name>: <option value>
	...

For example, to deploy 'mediawiki' with file 'mycfg.yaml' that contains:

  mediawiki:
	name: my media wiki
	admins: me:pwdOne
	debug: true

use

  juju deploy mediawiki --config mycfg.yaml

Key=value pairs can also be passed directly in the command. For example, to
declare the 'name' key:

  juju deploy mediawiki --config name='my media wiki'

To define multiple keys:

  juju deploy mediawiki --config name='my media wiki' --config debug=true

If a key gets defined multiple times the last value will override any earlier
values. For example,

  juju deploy mediawiki --config name='my media wiki' --config mycfg.yaml

Similar to the 'juju config' command, if the value begins with an '@' character,
it will be treated as a path to a config file and its contents will be assigned
to the specified key. For example,

  juju deploy mediawiki --config name='@wiki-name.txt"

will set the 'name' key to the contents of file 'wiki-name.txt'.

If mycfg.yaml contains a value for 'name', it will override the earlier 'my
media wiki' value. The same applies to single value options. For example,

  juju deploy mediawiki --config name='a media wiki' --config name='my wiki'

the value of 'my wiki' will be used.

Use the '--resource' option to upload resources needed by the charm. This
option may be repeated if multiple resources are needed:

  juju deploy foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml

Where 'bar' and 'baz' are named in the metadata file for charm 'foo'.

Use the '--to' option to deploy to an existing machine or container by
specifying a "placement directive". The ` + "`status`" + ` command should be used for
guidance on how to refer to machines. A few placement directives are
provider-dependent (e.g.: 'zone').

In more complex scenarios, "network spaces" are used to partition the cloud
networking layer into sets of subnets. Instances hosting units inside the same
space can communicate with each other without any firewalls. Traffic crossing
space boundaries could be subject to firewall and access restrictions. Using
spaces as deployment targets, rather than their individual subnets, allows Juju
to perform automatic distribution of units across availability zones to support
high availability for applications. Spaces help isolate applications and their
units, both for security purposes and to manage both traffic segregation and
congestion.

When deploying an application or adding machines, the 'spaces' constraint can
be used to define a comma-delimited list of required and forbidden spaces (the
latter prefixed with '^', similar to the 'tags' constraint).

When deploying bundles, machines specified in the bundle are added to the model
as new machines. Use the '--map-machines=existing' option to make use of any
existing machines. To map particular existing machines to machines defined in
the bundle, multiple comma separated values of the form 'bundle-id=existing-id'
can be passed. For example, for a bundle that specifies machines 1, 2, and 3;
and a model that has existing machines 1, 2, 3, and 4, the below deployment
would have existing machines 1 and 2 assigned to machines 1 and 2 defined in
the bundle and have existing machine 4 assigned to machine 3 defined in the
bundle.

  juju deploy mybundle --map-machines=existing,3=4

Only top level machines can be mapped in this way, just as only top level
machines can be defined in the machines section of the bundle.

When charms that include LXD profiles are deployed the profiles are validated
for security purposes by allowing only certain configurations and devices. Use
the '--force' option to bypass this check. Doing so is not recommended as it
can lead to unexpected behaviour.

Further reading: https://jaas.ai/docs/deploying-applications

Examples:

Deploy to a new machine:

    juju deploy apache2

Deploy to machine 23:

    juju deploy mysql --to 23

Deploy to a new LXD container on a new machine:

    juju deploy mysql --to lxd

Deploy to a new LXD container on machine 25:

    juju deploy mysql --to lxd:25

Deploy to LXD container 3 on machine 24:

    juju deploy mysql --to 24/lxd/3

Deploy 2 units, one on machine 3 and one to a new LXD container on machine 5:

    juju deploy mysql -n 2 --to 3,lxd:5

Deploy 3 units, one on machine 3 and the remaining two on new machines:

    juju deploy mysql -n 3 --to 3

Deploy to a machine with at least 8 GiB of memory:

    juju deploy postgresql --constraints mem=8G

Deploy to a specific availability zone (provider-dependent):

    juju deploy mysql --to zone=us-east-1a

Deploy to a specific MAAS node:

    juju deploy mysql --to host.maas

Deploy to a machine that is in the 'dmz' network space but not in either the
'cms' nor the 'database' spaces:

    juju deploy haproxy -n 2 --constraints spaces=dmz,^cms,^database

Deploy a k8s charm that requires a single Nvidia GPU:

    juju deploy mycharm --device miner=1,nvidia.com/gpu

Deploy a k8s charm that requires two Nvidia GPUs that have an
attribute of 'gpu=nvidia-tesla-p100':

    juju deploy mycharm --device \
       twingpu=2,nvidia.com/gpu,gpu=nvidia-tesla-p100

See also:
    add-relation
    add-unit
    config
    expose
    get-constraints
    set-constraints
    spaces
`

func (c *DeployCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "deploy",
		Args:    "<charm or bundle> [<application name>]",
		Purpose: "Deploys a new application or bundle.",
		Doc:     deployDoc,
	})
}

func (c *DeployCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ConfigOptions.SetPreserveStringValue(true)
	// Keep CharmOnlyFlags and BundleOnlyFlags lists updated when adding
	// new flags.
	c.UnitCommandBase.SetFlags(f)
	c.ModelCommandBase.SetFlags(f)
	f.IntVar(&c.NumUnits, "n", 1, "Number of application units to deploy for principal charms")
	f.StringVar(&c.channelStr, "channel", "", "Channel to use when deploying a charm or bundle from the charm store, or charm hub")
	f.Var(&c.ConfigOptions, "config", "Either a path to yaml-formatted application config file or a key=value pair ")

	f.BoolVar(&c.Trust, "trust", false, "Allows charm to run hooks that require access credentials")

	f.Var(cmd.NewAppendStringsValue(&c.BundleOverlayFile), "overlay", "Bundles to overlay on the primary bundle, applied in order")
	f.StringVar(&c.ConstraintsStr, "constraints", "", "Set application constraints")
	f.StringVar(&c.Series, "series", "", "The series on which to deploy")
	f.BoolVar(&c.DryRun, "dry-run", false, "Just show what the bundle deploy would do")
	f.BoolVar(&c.Force, "force", false, "Allow a charm/bundle to be deployed which bypasses checks such as supported series or LXD profile allow list")
	f.Var(storageFlag{&c.Storage, &c.BundleStorage}, "storage", "Charm storage constraints")
	f.Var(devicesFlag{&c.Devices, &c.BundleDevices}, "device", "Charm device constraints")
	f.Var(stringMap{&c.Resources}, "resource", "Resource to be uploaded to the controller")
	f.StringVar(&c.BindToSpaces, "bind", "", "Configure application endpoint bindings to spaces")
	f.StringVar(&c.machineMap, "map-machines", "", "Specify the existing machines to use for bundle deployments")

	for _, step := range c.Steps {
		step.SetFlags(f)
	}
	c.flagSet = f
}

func (c *DeployCommand) Init(args []string) error {
	if err := c.validateStorageByModelType(); err != nil {
		if !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		// It is possible that we will not be able to get model type to validate with.
		// For example, if current client does not know about a model, we
		// would have queried the controller about the model. However,
		// at Init() we do not yet have an API connection.
		// So we do not want to fail here if we encountered NotFoundErr, we want to
		// do a late validation at Run().
		c.unknownModel = true
	}
	switch len(args) {
	case 2:
		if err := names.ValidateApplicationName(args[1]); err != nil {
			return errors.Trace(err)
		}
		c.ApplicationName = args[1]
		fallthrough
	case 1:
		c.CharmOrBundle = args[0]
	case 0:
		return errors.New("no charm or bundle specified")
	default:
		return cmd.CheckEmpty(args[2:])
	}

	useExisting, mapping, err := parseMachineMap(c.machineMap)
	if err != nil {
		return errors.Annotate(err, "error in --map-machines")
	}
	c.UseExisting = useExisting
	c.BundleMachines = mapping

	if err := c.UnitCommandBase.Init(args); err != nil {
		return err
	}
	if err := c.validatePlacementByModelType(); err != nil {
		if !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		// It is possible that we will not be able to get model type to validate with.
		// For example, if current client does not know about a model, we
		// would have queried the controller about the model. However,
		// at Init() we do not yet have an API connection.
		// So we do not want to fail here if we encountered NotFoundErr, we want to
		// do a late validation at Run().
		c.unknownModel = true
	}
	if c.channelStr != "" {
		c.Channel, err = corecharm.ParseChannelNormalize(c.channelStr)
		if err != nil {
			return errors.Annotate(err, "error in --channel")
		}
	}
	return nil
}

func (c *DeployCommand) validateStorageByModelType() error {
	modelType, err := c.ModelType()
	if err != nil {
		return err
	}
	if modelType == model.IAAS {
		return nil
	}
	if len(c.AttachStorage) > 0 {
		return errors.New("--attach-storage cannot be used on k8s models")
	}
	return nil
}

func (c *DeployCommand) validatePlacementByModelType() error {
	modelType, err := c.ModelType()
	if err != nil {
		return err
	}
	if modelType == model.IAAS {
		return nil
	}
	if len(c.Placement) > 0 {
		return errors.New("--to cannot be used on k8s models")
	}
	return nil
}

func parseMachineMap(value string) (bool, map[string]string, error) {
	parts := strings.Split(value, ",")
	useExisting := false
	mapping := make(map[string]string)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch part {
		case "":
			// No-op.
		case "existing":
			useExisting = true
		default:
			otherParts := strings.Split(part, "=")
			if len(otherParts) != 2 {
				return false, nil, errors.Errorf("expected \"existing\" or \"<bundle-id>=<machine-id>\", got %q", part)
			}
			bundleID, machineID := strings.TrimSpace(otherParts[0]), strings.TrimSpace(otherParts[1])

			if i, err := strconv.Atoi(bundleID); err != nil || i < 0 {
				return false, nil, errors.Errorf("bundle-id %q is not a top level machine id", bundleID)
			}
			if i, err := strconv.Atoi(machineID); err != nil || i < 0 {
				return false, nil, errors.Errorf("machine-id %q is not a top level machine id", machineID)
			}
			mapping[bundleID] = machineID
		}
	}
	return useExisting, mapping, nil
}

// Run executes a deploy command with a given context.
func (c *DeployCommand) Run(ctx *cmd.Context) error {
	if c.unknownModel {
		if err := c.validateStorageByModelType(); err != nil {
			return errors.Trace(err)
		}
		if err := c.validatePlacementByModelType(); err != nil {
			return errors.Trace(err)
		}
	}
	var err error
	c.Constraints, err = common.ParseConstraints(ctx, c.ConstraintsStr)
	if err != nil {
		return err
	}
	cstoreAPI, err := c.NewCharmRepo()
	if err != nil {
		return errors.Trace(err)
	}
	apiRoot, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = apiRoot.Close() }()

	if err := c.parseBindFlag(apiRoot); err != nil {
		return errors.Trace(err)
	}

	for _, step := range c.Steps {
		step.SetPlanURL(apiRoot.PlanURL())
	}

	csRepoFn := func() (store.CharmrepoForDeploy, error) {
		return cstoreAPI, nil
	}
	downloadClientFn := func() (store.DownloadBundleClient, error) {
		return c.NewDownloadClient()
	}

	charmAdapter := c.NewResolver(apicharms.NewClient(apiRoot), csRepoFn, downloadClientFn)

	factory, cfg := c.getDeployerFactory()
	deploy, err := factory.GetDeployer(cfg, apiRoot, charmAdapter)
	if err != nil {
		return errors.Trace(err)
	}

	return block.ProcessBlockedError(deploy.PrepareAndDeploy(ctx, apiRoot, charmAdapter, cstoreAPI.MacaroonGetter), block.BlockChange)
}

func (c *DeployCommand) parseBindFlag(api SpacesAPI) error {
	if c.BindToSpaces == "" {
		return nil
	}

	// Fetch known spaces from server
	knownSpaceList, err := api.ListSpaces()
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

func (c *DeployCommand) getMeteringAPIURL(controllerAPIRoot api.Connection) (string, error) {
	controllerAPI := controller.NewClient(controllerAPIRoot)
	controllerCfg, err := controllerAPI.ControllerConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	return controllerCfg.MeteringURL(), nil
}

func (c *DeployCommand) getDeployerFactory() (deployer.DeployerFactory, deployer.DeployerConfig) {
	dep := deployer.DeployerDependencies{
		Model:                c,
		NewConsumeDetailsAPI: c.NewConsumeDetailsAPI, // only used here
		Steps:                c.Steps,
	}
	cfg := deployer.DeployerConfig{
		ApplicationName:   c.ApplicationName,
		AttachStorage:     c.AttachStorage,
		Bindings:          c.Bindings,
		BundleDevices:     c.BundleDevices,
		BundleMachines:    c.BundleMachines,
		BundleOverlayFile: c.BundleOverlayFile,
		BundleStorage:     c.BundleStorage,
		Channel:           c.Channel,
		CharmOrBundle:     c.CharmOrBundle,
		ConfigOptions:     c.ConfigOptions,
		Constraints:       c.Constraints,
		Devices:           c.Devices,
		DryRun:            c.DryRun,
		FlagSet:           c.flagSet,
		Force:             c.Force,
		NumUnits:          c.NumUnits,
		PlacementSpec:     c.PlacementSpec,
		Placement:         c.Placement,
		Resources:         c.Resources,
		Series:            c.Series,
		Storage:           c.Storage,
		Trust:             c.Trust,
		UseExisting:       c.UseExisting,
	}
	return c.NewDeployerFactory(dep), cfg
}

func (c *DeployCommand) getCharmHubURL(apiRoot base.APICallCloser) (string, error) {
	modelConfigClient := c.NewModelConfigClient(apiRoot)
	defer func() { _ = modelConfigClient.Close() }()

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
