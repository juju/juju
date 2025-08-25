// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strconv"
	"strings"

	"github.com/juju/charm/v12"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/annotations"
	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/applicationoffers"
	apicharms "github.com/juju/juju/api/client/charms"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/machinemanager"
	"github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/api/client/spaces"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/charmhub"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/application/deployer"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	apiparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

// SpacesAPI defines the necessary API methods needed for listing spaces.
type SpacesAPI interface {
	ListSpaces() ([]apiparams.Space, error)
}

type CharmsAPI interface {
	store.CharmsAPI
}

type (
	charmsClient         = apicharms.Client
	localCharmsClient    = apicharms.LocalCharmClient
	applicationClient    = application.Client
	modelConfigClient    = modelconfig.Client
	annotationsClient    = annotations.Client
	offerClient          = applicationoffers.Client
	spacesClient         = spaces.API
	machineManagerClient = machinemanager.Client
)

type deployAPIAdapter struct {
	api.Connection
	*charmsClient
	*localCharmsClient
	*applicationClient
	*modelConfigClient
	*annotationsClient
	*offerClient
	*spacesClient
	*machineManagerClient
	legacyClient *apiclient.Client
}

func (a *deployAPIAdapter) ModelUUID() (string, bool) {
	tag, ok := a.ModelTag()
	return tag.Id(), ok
}

func (a *deployAPIAdapter) WatchAll() (api.AllWatch, error) {
	return a.legacyClient.WatchAll()
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

func (a *deployAPIAdapter) GetModelConstraints() (constraints.Value, error) {
	return a.modelConfigClient.GetModelConstraints()
}

func (a *deployAPIAdapter) AddCharm(curl *charm.URL, origin commoncharm.Origin, force bool) (commoncharm.Origin, error) {
	return a.charmsClient.AddCharm(curl, origin, force)
}

type modelGetter interface {
	ModelGet() (map[string]interface{}, error)
}

func agentVersion(c modelGetter) (version.Number, error) {
	attrs, err := c.ModelGet()
	if err != nil {
		return version.Zero, errors.Trace(err)
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return version.Zero, errors.Trace(err)
	}
	agentVersion, ok := cfg.AgentVersion()
	if !ok {
		return version.Zero, errors.New("model config missing agent version")
	}
	return agentVersion, nil
}

func (a *deployAPIAdapter) AddLocalCharm(url *charm.URL, c charm.Charm, b bool) (*charm.URL, error) {
	agentVersion, err := agentVersion(a.modelConfigClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return a.localCharmsClient.AddLocalCharm(url, c, b, agentVersion)
}

func (a *deployAPIAdapter) Status(opts *apiclient.StatusArgs) (*apiparams.FullStatus, error) {
	return a.legacyClient.Status(opts)
}

// NewDeployCommand returns a command to deploy applications.
func NewDeployCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(newDeployCommand())
}

func newDeployCommand() *DeployCommand {
	deployCmd := &DeployCommand{}
	deployCmd.NewModelConfigAPI = func(api base.APICallCloser) ModelConfigGetter {
		return modelconfig.NewClient(api)
	}
	deployCmd.NewCharmsAPI = func(api base.APICallCloser) CharmsAPI {
		return apicharms.NewClient(api)
	}
	deployCmd.NewDownloadClient = func() (store.DownloadBundleClient, error) {
		apiRoot, err := deployCmd.newAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		modelConfigClient := deployCmd.NewModelConfigAPI(apiRoot)
		charmHubURL, err := deployCmd.getCharmHubURL(modelConfigClient)
		if err != nil {
			return nil, errors.Trace(err)
		}

		return charmhub.NewClient(charmhub.Config{
			URL:    charmHubURL,
			Logger: logger,
		})
	}
	deployCmd.NewDeployAPI = func() (deployer.DeployerAPI, error) {
		apiRoot, err := deployCmd.newAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		controllerAPIRoot, err := deployCmd.newControllerAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		localCharmsClient, err := apicharms.NewLocalCharmClient(apiRoot)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return &deployAPIAdapter{
			Connection:           apiRoot,
			legacyClient:         apiclient.NewClient(apiRoot, logger),
			charmsClient:         apicharms.NewClient(apiRoot),
			localCharmsClient:    localCharmsClient,
			applicationClient:    application.NewClient(apiRoot),
			machineManagerClient: machinemanager.NewClient(apiRoot),
			modelConfigClient:    modelconfig.NewClient(apiRoot),
			annotationsClient:    annotations.NewClient(apiRoot),
			offerClient:          applicationoffers.NewClient(controllerAPIRoot),
			spacesClient:         spaces.NewAPI(apiRoot),
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
	deployCmd.NewResolver = func(charmsAPI store.CharmsAPI, downloadClientFn store.DownloadBundleClientFunc) deployer.Resolver {
		return store.NewCharmAdaptor(charmsAPI, downloadClientFn)
	}
	return deployCmd
}
func (c *DeployCommand) newAPIRoot() (api.Connection, error) {
	if c.apiRoot == nil {
		var err error
		c.apiRoot, err = c.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return c.apiRoot, nil
}

func (c *DeployCommand) newControllerAPIRoot() (api.Connection, error) {
	if c.controllerAPIRoot == nil {
		var err error
		c.controllerAPIRoot, err = c.NewControllerAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return c.controllerAPIRoot, nil
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
	Channel charm.Channel

	channelStr string

	// Revision is the revision of the charm to deploy.
	Revision int

	// Series is the series of the charm to deploy.
	// DEPRECATED: Use --base instead.
	Series string

	// Base is the base of the charm to deploy.
	Base string

	// Force is used to allow a charm/bundle to be deployed onto a machine
	// running an unsupported series.
	Force bool

	// DryRun is used to specify that the bundle shouldn't actually be
	// deployed but just output the changes.
	DryRun bool

	ApplicationName  string
	ConfigOptions    common.ConfigFlag
	ConstraintsStr   common.ConstraintsFlag
	Constraints      constraints.Value
	ModelConstraints constraints.Value
	BindToSpaces     string

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

	// UseExisting machines when deploying the bundle.
	UseExisting bool

	// BundleMachines is a mapping for machines in the bundle to machines
	// in the model.
	BundleMachines map[string]string

	// NewDeployAPI stores a function which returns a new deploy client.
	NewDeployAPI func() (deployer.DeployerAPI, error)

	// NewDownloadClient stores a function for getting a charm/bundle.
	NewDownloadClient func() (store.DownloadBundleClient, error)

	// NewModelConfigAPI stores a function which returns a new model config
	// client. This is used to get the model config.
	NewModelConfigAPI func(base.APICallCloser) ModelConfigGetter

	// NewCharmsAPI stores a function for getting info about charms.
	NewCharmsAPI func(caller base.APICallCloser) CharmsAPI

	// NewResolver stores a function which returns a charm adaptor.
	NewResolver func(store.CharmsAPI, store.DownloadBundleClientFunc) deployer.Resolver

	// NewDeployerFactory stores a function which returns a deployer factory.
	NewDeployerFactory func(dep deployer.DeployerDependencies) deployer.DeployerFactory

	// NewConsumeDetailsAPI stores a function which will return a new API
	// for consume details API using the url as the source.
	NewConsumeDetailsAPI func(url *charm.OfferURL) (deployer.ConsumeDetails, error)

	// DeployResources stores a function which deploys charm resources.
	DeployResources deployer.DeployResourcesFunc

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

	controllerAPIRoot api.Connection
	apiRoot           api.Connection
}

const deployDoc = `
A charm or bundle can be referred to by its simple name and a base, revision,
or channel can optionally be specified:

    juju deploy postgresql
    juju deploy ch:postgresql --base ubuntu@22.04
    juju deploy ch:postgresql --channel edge
    juju deploy ch:ubuntu --revision 17 --channel edge

All the above deployments use remote charms found in Charmhub, denoted by the
` + "`ch:`" + ` prefix.  Remote charms with no prefix will be deployed from Charmhub.

If a channel is specified, it will be used as the source for looking up the
charm or bundle from Charmhub. When used in a bundle deployment context,
the specified channel is only used for retrieving the bundle and is ignored when
looking up the charms referenced by the bundle. However, each charm within a
bundle is allowed to explicitly specify the channel used to look it up.

If a revision is specified, a channel must also be specified for Charmhub charms
and bundles.  The charm will be deployed with revision.  The channel will be used
when refreshing the application in the future.

A local charm may be deployed by giving the path to its directory:

    juju deploy /path/to/charm
    juju deploy /path/to/charm --base ubuntu@22.04

You will need to be explicit if there is an ambiguity between a local and a
remote charm:

    juju deploy ./pig
    juju deploy ch:pig

A bundle can be expressed similarly to a charm:

    juju deploy mediawiki-single
    juju deploy mediawiki-single --base ubuntu@22.04
    juju deploy ch:mediawiki-single

A local bundle may be deployed by specifying the path to its YAML file:

    juju deploy /path/to/bundle.yaml

The final charm/machine base is determined using an order of precedence (most
preferred to least):

- the ` + "`--base`" + ` command option
- for a bundle, the series stated in each charm URL (in the bundle file)
- for a bundle, the series given at the top level (in the bundle file)
- the ` + "`default-base`" + ` model configuration key
- the first base specified in the charm's manifest file

An 'application name' provides an alternate name for the application. It works
only for charms; it is silently ignored for bundles (although the same can be
done at the bundle file level). Such a name must consist only of lower-case
letters (a-z), numbers (0-9), and single hyphens (-). The name must begin with
a letter and not have a group of all numbers follow a hyphen:

- Valid:  ` + "`myappname`" + `, ` + "`custom-app`" + `, ` + "`app2-scat-23skidoo`" + `
- Invalid: ` + "`myAppName`" + `, ` + "`custom--app`" + `, ` + "`app2-scat-23`" + `, ` + "`areacode-555-info`" + `

Use the ` + "`--constraints`" + ` option to specify hardware requirements for new machines.
These become the application's default constraints (i.e. they are used if the
application is later scaled out with the ` + "`add-unit`" + ` command). To overcome this
behaviour use the ` + "`set-constraints`" + ` command to change the application's default
constraints or add a machine (` + "`add-machine`" + `) with a certain constraint and then
target that machine with ` + "`add-unit`" + ` by using the ` + "`--to`" + `option.

Use the ` + "`--device`" + ` option to specify GPU device requirements (with Kubernetes).
The below format is used for this option's value, where the 'label' is named in
the charm metadata file:

    <label>=[<count>,]<device-class>|<vendor/type>[,<attributes>]

Use the ` + "`--config`" + ` option to specify application configuration values. This
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

Similar to the ` + "`juju config`" + ` command, if the value begins with an '@' character,
it will be treated as a path to a config file and its contents will be assigned
to the specified key. For example,

    juju deploy mediawiki --config name='@wiki-name.txt"

will set the 'name' key to the contents of file 'wiki-name.txt'.

If mycfg.yaml contains a value for 'name', it will override the earlier 'my
media wiki' value. The same applies to single value options. For example,

    juju deploy mediawiki --config name='a media wiki' --config name='my wiki'

the value of 'my wiki' will be used.

Use the ` + "`--resource`" + ` option to specify the resources you want to use for your charm.
The format is

    --resource <resource name>=<resource>

where the resource name is the name from the ` + "`metadata.yaml`" + ` file of the charm
and where, depending on the type of the resource, the resource can be specified
as follows:

- If the resource is type ` + "`file`" + `, you can specify it by providing one of the following:

    a. the resource revision number.

    b. a path to a local file. Caveat: If you choose this, you will not be able
	 to go back to using a resource from Charmhub.

- If the resource is type ` + "`oci-image`" + `, you can specify it by providing one of the following:

    a. the resource revision number.

	b. a path to the local file for your private OCI image as well as the
	username and password required to access the private OCI image.
	Caveat: If you choose this, you will not be able to go back to using a
	resource from Charmhub.

    c. a link to a public OCI image. Caveat: If you choose this, you will not be
	 able to go back to using a resource from Charmhub.


Note: If multiple resources are needed, repeat the option.


Use the ` + "`--to`" + ` option to deploy to an existing machine or container by
specifying a "placement directive". The ` + "`status`" + ` command should be used for
guidance on how to refer to machines. A few placement directives are
provider-dependent (e.g.: ` + "`zone`" + `).

In more complex scenarios, network spaces are used to partition the cloud
networking layer into sets of subnets. Instances hosting units inside the same
space can communicate with each other without any firewalls. Traffic crossing
space boundaries could be subject to firewall and access restrictions. Using
spaces as deployment targets, rather than their individual subnets, allows Juju
to perform automatic distribution of units across availability zones to support
high availability for applications. Spaces help isolate applications and their
units, both for security purposes and to manage both traffic segregation and
congestion.

When deploying an application or adding machines, the ` + "`spaces `" + ` constraint can
be used to define a comma-delimited list of required and forbidden spaces (the
latter prefixed with '^', similar to the 'tags' constraint).

When deploying bundles, machines specified in the bundle are added to the model
as new machines. Use the ` + "`--map-machines=existing`" + ` option to make use of any
existing machines. To map particular existing machines to machines defined in
the bundle, multiple comma separated values of the form ` + "`bundle-id=existing-id`" + `
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
the ` + "`--force`" + ` option to bypass this check. Doing so is not recommended as it
can lead to unexpected behaviour.

`

const deployExamples = `
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

Deploy a Kubernetes charm that requires a single Nvidia GPU:

    juju deploy mycharm --device miner=1,nvidia.com/gpu

Deploy a Kubernetes charm that requires two Nvidia GPUs that have an
attribute of 'gpu=nvidia-tesla-p100':

    juju deploy mycharm --device \
       twingpu=2,nvidia.com/gpu,gpu=nvidia-tesla-p100

Deploy with specific resources:

    juju deploy foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml
`

func (c *DeployCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "deploy",
		Args:     "<charm or bundle> [<application name>]",
		Purpose:  "Deploys a new application or bundle.",
		Doc:      deployDoc,
		Examples: deployExamples,
		SeeAlso: []string{
			"integrate",
			"add-unit",
			"config",
			"expose",
			"constraints",
			"refresh",
			"set-constraints",
			"spaces",
			"charm-resources",
		},
	})
}

func (c *DeployCommand) SetFlags(f *gnuflag.FlagSet) {
	// Keep CharmOnlyFlags and BundleOnlyFlags lists updated when adding
	// new flags.
	c.UnitCommandBase.SetFlags(f)
	c.ModelCommandBase.SetFlags(f)
	f.IntVar(&c.NumUnits, "n", 1, "Number of application units to deploy for principal charms")
	f.StringVar(&c.channelStr, "channel", "", "Channel to use when deploying a charm or bundle from Charmhub")
	f.Var(&c.ConfigOptions, "config", "Either a path to yaml-formatted application config file or a key=value pair ")

	f.BoolVar(&c.Trust, "trust", false, "Allows charm to run hooks that require access credentials")

	f.Var(cmd.NewAppendStringsValue(&c.BundleOverlayFile), "overlay", "Bundles to overlay on the primary bundle, applied in order")
	f.Var(&c.ConstraintsStr, "constraints", "Set application constraints")
	f.StringVar(&c.Series, "series", "", "The series on which to deploy. DEPRECATED: use `--base`")
	f.StringVar(&c.Base, "base", "", "The base on which to deploy")
	f.IntVar(&c.Revision, "revision", -1, "The revision to deploy")
	f.BoolVar(&c.DryRun, "dry-run", false, "Just show what the deploy would do")
	f.BoolVar(&c.Force, "force", false, "Allow a charm/bundle to be deployed which bypasses checks such as supported base or LXD profile allow list")
	f.Var(storageFlag{&c.Storage, &c.BundleStorage}, "storage", "Charm storage constraints")
	f.Var(devicesFlag{&c.Devices, &c.BundleDevices}, "device", "Charm device constraints")
	f.Var(stringMap{&c.Resources}, "resource", "Resource to be uploaded to the controller")
	f.StringVar(&c.BindToSpaces, "bind", "", "Configure application endpoint bindings to spaces")
	f.StringVar(&c.machineMap, "map-machines", "", "Specify the existing machines to use for bundle deployments")

	c.flagSet = f
}

// Init validates the flags.
func (c *DeployCommand) Init(args []string) error {
	if c.Base != "" && c.Series != "" {
		return errors.New("--series and --base cannot be specified together")
	}
	// NOTE: For deploying a charm with the revision flag, a channel is
	// also required. It's required to ensure that juju knows which channel
	// should be used for refreshing/upgrading the charm in the future.However
	// a bundle does not require a channel, today you cannot refresh/upgrade
	// a bundle, only the components. These flags will be verified in the
	// GetDeployer instead.
	if err := c.validateStorageByModelType(); err != nil {
		if !errors.Is(err, errors.NotFound) {
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
		c.Channel, err = charm.ParseChannelNormalize(c.channelStr)
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
	var (
		base corebase.Base
		err  error
	)
	// Note: we validated that both series and base cannot be specified in
	// Init(), so it's safe to assume that only one of them is set here.
	if c.Series != "" {
		if c.Series == "kubernetes" {
			ctx.Warningf("using kubernetes as a series flag is deprecated, use --base instead")
			base = corebase.LegacyKubernetesBase()
		} else {
			ctx.Warningf("series flag is deprecated, use --base instead")
			if base, err = corebase.GetBaseFromSeries(c.Series); err != nil {
				return errors.Annotatef(err, "attempting to convert %q to a base", c.Series)
			}
		}
		c.Base = base.String()
		c.Series = ""
	}
	if c.Base != "" {
		if base, err = corebase.ParseBaseFromString(c.Base); err != nil {
			return errors.Trace(err)
		}
	}

	if c.unknownModel {
		if err := c.validateStorageByModelType(); err != nil {
			return errors.Trace(err)
		}
		if err := c.validatePlacementByModelType(); err != nil {
			return errors.Trace(err)
		}
	}
	if c.Constraints, err = common.ParseConstraints(ctx, strings.Join(c.ConstraintsStr, " ")); err != nil {
		return errors.Trace(err)
	}

	deployAPI, err := c.NewDeployAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		if c.apiRoot != nil {
			_ = c.apiRoot.Close()
		}
		if c.controllerAPIRoot != nil {
			_ = c.controllerAPIRoot.Close()
		}
	}()

	if c.ModelConstraints, err = deployAPI.GetModelConstraints(); err != nil {
		return errors.Trace(err)
	}

	if err := c.parseBindFlag(deployAPI); err != nil {
		return errors.Trace(err)
	}

	downloadClientFn := func() (store.DownloadBundleClient, error) {
		return c.NewDownloadClient()
	}

	charmAPIClient := c.NewCharmsAPI(c.apiRoot)
	charmAdapter := c.NewResolver(charmAPIClient, downloadClientFn)

	factory, cfg := c.getDeployerFactory(base, charm.CharmHub)
	deploy, err := factory.GetDeployer(cfg, deployAPI, charmAdapter)
	if err != nil {
		return errors.Trace(err)
	}

	return block.ProcessBlockedError(deploy.PrepareAndDeploy(ctx, deployAPI, charmAdapter), block.BlockChange)
}

func (c *DeployCommand) parseBindFlag(api SpacesAPI) error {
	if c.BindToSpaces == "" {
		return nil
	}

	// Fetch known spaces from server
	knownSpaces, err := api.ListSpaces()
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

func (c *DeployCommand) getDeployerFactory(base corebase.Base, defaultCharmSchema charm.Schema) (deployer.DeployerFactory, deployer.DeployerConfig) {
	dep := deployer.DeployerDependencies{
		Model:                c,
		FileSystem:           c.ModelCommandBase.Filesystem(),
		CharmReader:          defaultCharmReader{},
		NewConsumeDetailsAPI: c.NewConsumeDetailsAPI, // only used here
	}
	cfg := deployer.DeployerConfig{
		ApplicationName:    c.ApplicationName,
		AttachStorage:      c.AttachStorage,
		Bindings:           c.Bindings,
		BundleDevices:      c.BundleDevices,
		BundleMachines:     c.BundleMachines,
		BundleOverlayFile:  c.BundleOverlayFile,
		BundleStorage:      c.BundleStorage,
		Channel:            c.Channel,
		CharmOrBundle:      c.CharmOrBundle,
		DefaultCharmSchema: defaultCharmSchema,
		ConfigOptions:      c.ConfigOptions,
		Constraints:        c.Constraints,
		ModelConstraints:   c.ModelConstraints,
		Devices:            c.Devices,
		DryRun:             c.DryRun,
		FlagSet:            c.flagSet,
		Force:              c.Force,
		NumUnits:           c.NumUnits,
		PlacementSpec:      c.PlacementSpec,
		Placement:          c.Placement,
		Resources:          c.Resources,
		Revision:           c.Revision,
		Base:               base,
		Storage:            c.Storage,
		Trust:              c.Trust,
		UseExisting:        c.UseExisting,
	}
	return c.NewDeployerFactory(dep), cfg
}

func (c *DeployCommand) getCharmHubURL(modelConfigClient ModelConfigGetter) (string, error) {
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

type defaultCharmReader struct{}

// NewCharmAtPath returns the charm represented by this path,
// and a URL that describes it.
func (defaultCharmReader) NewCharmAtPath(path string) (charm.Charm, *charm.URL, error) {
	return corecharm.NewCharmAtPath(path)
}
