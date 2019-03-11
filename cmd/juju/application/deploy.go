// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"archive/zip"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/romulus"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/charmrepo.v3/csclient"
	"gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	"gopkg.in/macaroon.v2-unstable"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/annotations"
	"github.com/juju/juju/api/application"
	apicharms "github.com/juju/juju/api/charms"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/api/modelconfig"
	app "github.com/juju/juju/apiserver/facades/client/application"
	apiparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/storage"
)

type CharmAdder interface {
	AddLocalCharm(*charm.URL, charm.Charm, bool) (*charm.URL, error)
	AddCharm(*charm.URL, params.Channel, bool) error
	AddCharmWithAuthorization(*charm.URL, params.Channel, *macaroon.Macaroon, bool) error
	AuthorizeCharmstoreEntity(*charm.URL) (*macaroon.Macaroon, error)
}

type ApplicationAPI interface {
	AddMachines(machineParams []apiparams.AddMachineParams) ([]apiparams.AddMachinesResult, error)
	AddRelation(endpoints, viaCIDRs []string) (*apiparams.AddRelationResults, error)
	AddUnits(application.AddUnitsParams) ([]string, error)
	Expose(application string) error
	GetAnnotations(tags []string) ([]apiparams.AnnotationsGetResult, error)
	GetConfig(generation model.GenerationVersion, appNames ...string) ([]map[string]interface{}, error)
	GetConstraints(appNames ...string) ([]constraints.Value, error)
	SetAnnotation(annotations map[string]map[string]string) ([]apiparams.ErrorResult, error)
	SetCharm(model.GenerationVersion, application.SetCharmConfig) error
	SetConstraints(application string, constraints constraints.Value) error
	Update(apiparams.ApplicationUpdate) error
	ScaleApplication(application.ScaleApplicationParams) (apiparams.ScaleApplicationResult, error)
}

type ModelAPI interface {
	ModelUUID() (string, bool)
	ModelGet() (map[string]interface{}, error)
	Sequences() (map[string]int, error)
}

// MeteredDeployAPI represents the methods of the API the deploy
// command needs for metered charms.
type MeteredDeployAPI interface {
	IsMetered(charmURL string) (bool, error)
	SetMetricCredentials(application string, credentials []byte) error
}

// CharmDeployAPI represents the methods of the API the deploy
// command needs for charms.
type CharmDeployAPI interface {
	CharmInfo(string) (*apicharms.CharmInfo, error)
}

// DeployAPI represents the methods of the API the deploy
// command needs.
type DeployAPI interface {
	// TODO(katco): Pair DeployAPI down to only the methods required
	// by the deploy command.
	api.Connection
	CharmAdder
	MeteredDeployAPI
	CharmDeployAPI
	ApplicationAPI
	ModelAPI

	// ApplicationClient
	Deploy(application.DeployArgs) error
	Status(patterns []string) (*apiparams.FullStatus, error)

	ResolveWithChannel(*charm.URL) (*charm.URL, params.Channel, []string, error)

	GetBundle(*charm.URL) (charm.Bundle, error)

	WatchAll() (*api.AllWatcher, error)

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

type charmRepoClient struct {
	*charmrepo.CharmStore
}

type charmstoreClient struct {
	*csclient.Client
}

type annotationsClient struct {
	*annotations.Client
}

type plansClient struct {
	planURL string
}

func (a *charmstoreClient) AuthorizeCharmstoreEntity(url *charm.URL) (*macaroon.Macaroon, error) {
	return authorizeCharmStoreEntity(a.Client, url)
}

func (c *plansClient) PlanURL() string {
	return c.planURL
}

type deployAPIAdapter struct {
	api.Connection
	*apiClient
	*charmsClient
	*applicationClient
	*modelConfigClient
	*charmRepoClient
	*charmstoreClient
	*annotationsClient
	*plansClient
}

func (a *deployAPIAdapter) Client() *api.Client {
	return a.apiClient.Client
}

func (a *deployAPIAdapter) ModelUUID() (string, bool) {
	return a.apiClient.ModelUUID()
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

func (a *deployAPIAdapter) Resolve(cfg *config.Config, url *charm.URL) (
	*charm.URL,
	params.Channel,
	[]string,
	error,
) {
	return resolveCharm(a.charmRepoClient.ResolveWithChannel, url)
}

func (a *deployAPIAdapter) Get(url *charm.URL) (charm.Charm, error) {
	return a.charmRepoClient.Get(url)
}

func (a *deployAPIAdapter) SetAnnotation(annotations map[string]map[string]string) ([]apiparams.ErrorResult, error) {
	return a.annotationsClient.Set(annotations)
}

func (a *deployAPIAdapter) GetAnnotations(tags []string) ([]apiparams.AnnotationsGetResult, error) {
	return a.annotationsClient.Get(tags)
}

// NewDeployCommandForTest returns a command to deploy applications intended to be used only in tests.
func NewDeployCommandForTest(newAPIRoot func() (DeployAPI, error), steps []DeployStep) modelcmd.ModelCommand {
	deployCmd := &DeployCommand{
		Steps:      steps,
		NewAPIRoot: newAPIRoot,
	}
	if newAPIRoot == nil {
		deployCmd.NewAPIRoot = func() (DeployAPI, error) {
			apiRoot, err := deployCmd.ModelCommandBase.NewAPIRoot()
			if err != nil {
				return nil, errors.Trace(err)
			}
			bakeryClient, err := deployCmd.BakeryClient()
			if err != nil {
				return nil, errors.Trace(err)
			}
			controllerAPIRoot, err := deployCmd.NewControllerAPIRoot()
			if err != nil {
				return nil, errors.Trace(err)
			}
			csURL, err := getCharmStoreAPIURL(controllerAPIRoot)
			if err != nil {
				return nil, errors.Trace(err)
			}
			mURL, err := deployCmd.getMeteringAPIURL(controllerAPIRoot)
			if err != nil {
				return nil, errors.Trace(err)
			}
			cstoreClient := newCharmStoreClient(bakeryClient, csURL).WithChannel(deployCmd.Channel)

			return &deployAPIAdapter{
				Connection:        apiRoot,
				apiClient:         &apiClient{Client: apiRoot.Client()},
				charmsClient:      &charmsClient{Client: apicharms.NewClient(apiRoot)},
				applicationClient: &applicationClient{Client: application.NewClient(apiRoot)},
				modelConfigClient: &modelConfigClient{Client: modelconfig.NewClient(apiRoot)},
				charmstoreClient:  &charmstoreClient{Client: cstoreClient},
				annotationsClient: &annotationsClient{Client: annotations.NewClient(apiRoot)},
				charmRepoClient:   &charmRepoClient{CharmStore: charmrepo.NewCharmStoreFromClient(cstoreClient)},
				plansClient:       &plansClient{planURL: mURL},
			}, nil
		}
	}
	return modelcmd.Wrap(deployCmd)
}

// NewDeployCommand returns a command to deploy applications.
func NewDeployCommand() modelcmd.ModelCommand {
	steps := []DeployStep{
		&RegisterMeteredCharm{
			PlanURL:      romulus.DefaultAPIRoot,
			RegisterPath: "/plan/authorize",
			QueryPath:    "/charm",
		},
		&ValidateLXDProfileCharm{},
	}
	deployCmd := &DeployCommand{
		Steps: steps,
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
		csURL, err := getCharmStoreAPIURL(controllerAPIRoot)
		if err != nil {
			return nil, errors.Trace(err)
		}
		mURL, err := deployCmd.getMeteringAPIURL(controllerAPIRoot)
		if err != nil {
			return nil, errors.Trace(err)
		}
		bakeryClient, err := deployCmd.BakeryClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
		cstoreClient := newCharmStoreClient(bakeryClient, csURL).WithChannel(deployCmd.Channel)

		return &deployAPIAdapter{
			Connection:        apiRoot,
			apiClient:         &apiClient{Client: apiRoot.Client()},
			charmsClient:      &charmsClient{Client: apicharms.NewClient(apiRoot)},
			applicationClient: &applicationClient{Client: application.NewClient(apiRoot)},
			modelConfigClient: &modelConfigClient{Client: modelconfig.NewClient(apiRoot)},
			charmstoreClient:  &charmstoreClient{Client: cstoreClient},
			annotationsClient: &annotationsClient{Client: annotations.NewClient(apiRoot)},
			charmRepoClient:   &charmRepoClient{CharmStore: charmrepo.NewCharmStoreFromClient(cstoreClient)},
			plansClient:       &plansClient{planURL: mURL},
		}, nil
	}

	return modelcmd.Wrap(deployCmd)
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

	// Channel holds the charmstore channel to use when obtaining
	// the charm to be deployed.
	Channel params.Channel

	// Series is the series of the charm to deploy.
	Series string

	// Force is used to allow a charm to be deployed onto a machine
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
	Steps    []DeployStep

	// UseExisting machines when deploying the bundle.
	UseExisting bool
	// BundleMachines is a mapping for machines in the bundle to machines
	// in the model.
	BundleMachines map[string]string

	// NewAPIRoot stores a function which returns a new API root.
	NewAPIRoot func() (DeployAPI, error)

	// Trust signifies that the charm should be deployed with access to
	// trusted credentials. That is, hooks run by the charm can access
	// cloud credentials and other trusted access credentials.
	Trust bool

	machineMap string
	flagSet    *gnuflag.FlagSet

	unknownModel bool
}

const deployDoc = `
A charm can be referred to by its simple name and a series can optionally be
specified:

  juju deploy postgresql
  juju deploy xenial/postgresql
  juju deploy cs:postgresql
  juju deploy cs:xenial/postgresql
  juju deploy postgresql --series xenial

All the above deployments use remote charms found in the Charm Store (denoted
by 'cs') and therefore also make use of "charm URLs".

A versioned charm URL will be expanded as expected. For example, 'mysql-56'
becomes 'cs:xenial/mysql-56'.

A local charm may be deployed by giving the path to its directory:

  juju deploy /path/to/charm
  juju deploy /path/to/charm --series xenial

You will need to be explicit if there is an ambiguity between a local and a
remote charm:

  juju deploy ./pig
  juju deploy cs:pig

An error is emitted if the determined series is not supported by the charm. Use
the '--force' option to override this check:

  juju deploy charm --series xenial --force

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

if mycfg.yaml contains a value for 'name', it will override the earlier 'my
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

Further reading: https://docs.jujucharms.com/stable/charms-deploying

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

Deploy a Kubernetes charm that requires a single Nvidia GPU:

    juju deploy mycharm --device miner=1,nvidia.com/gpu

Deploy a Kubernetes charm that requires two Nvidia GPUs that have an
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

//go:generate mockgen -package mocks -destination mocks/deploystepapi_mock.go github.com/juju/juju/cmd/juju/application DeployStepAPI

// DeployStepAPI represents a API required for deploying using the step
// deployment code.
type DeployStepAPI interface {
	MeteredDeployAPI
}

// DeployStep is an action that needs to be taken during charm deployment.
type DeployStep interface {
	// SetFlags sets flags necessary for the deploy step.
	SetFlags(*gnuflag.FlagSet)

	// SetPlanURL sets the plan URL prefix.
	SetPlanURL(planURL string)

	// RunPre runs before the call is made to add the charm to the environment.
	RunPre(DeployStepAPI, *httpbakery.Client, *cmd.Context, DeploymentInfo) error

	// RunPost runs after the call is made to add the charm to the environment.
	// The error parameter is used to notify the step of a previously occurred error.
	RunPost(DeployStepAPI, *httpbakery.Client, *cmd.Context, DeploymentInfo, error) error
}

// DeploymentInfo is used to maintain all deployment information for
// deployment steps.
type DeploymentInfo struct {
	CharmID         charmstore.CharmID
	ApplicationName string
	ModelUUID       string
	CharmInfo       *apicharms.CharmInfo
	ApplicationPlan string
	Force           bool
}

func (c *DeployCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "deploy",
		Args:    "<charm or bundle> [<application name>]",
		Purpose: "Deploys a new application or bundle.",
		Doc:     deployDoc,
	})
}

var (
	// TODO(thumper): support dry-run for apps as well as bundles.
	bundleOnlyFlags = []string{
		"overlay", "dry-run", "map-machines",
	}
)

// charmOnlyFlags and bundleOnlyFlags are used to validate flags based on
// whether we are deploying a charm or a bundle.
func charmOnlyFlags() []string {
	charmOnlyFlags := []string{
		"bind", "config", "constraints", "force", "n", "num-units",
		"series", "to", "resource", "attach-storage",
	}

	charmOnlyFlags = append(charmOnlyFlags, "trust")

	return charmOnlyFlags
}

func (c *DeployCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ConfigOptions.SetPreserveStringValue(true)
	// Keep above charmOnlyFlags and bundleOnlyFlags lists updated when adding
	// new flags.
	c.UnitCommandBase.SetFlags(f)
	c.ModelCommandBase.SetFlags(f)
	f.IntVar(&c.NumUnits, "n", 1, "Number of application units to deploy for principal charms")
	f.StringVar((*string)(&c.Channel), "channel", "", "Channel to use when getting the charm or bundle from the charm store")
	f.Var(&c.ConfigOptions, "config", "Either a path to yaml-formatted application config file or a key=value pair ")

	f.BoolVar(&c.Trust, "trust", false, "Allows charm to run hooks that require access credentials")

	f.Var(cmd.NewAppendStringsValue(&c.BundleOverlayFile), "overlay", "Bundles to overlay on the primary bundle, applied in order")
	f.StringVar(&c.ConstraintsStr, "constraints", "", "Set application constraints")
	f.StringVar(&c.Series, "series", "", "The series on which to deploy")
	f.BoolVar(&c.DryRun, "dry-run", false, "Just show what the bundle deploy would do")
	f.BoolVar(&c.Force, "force", false, "Allow a charm to be deployed which bypasses checks such as supported series or LXD profile allow list")
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
		if !names.IsValidApplication(args[1]) {
			return errors.Errorf("invalid application name %q", args[1])
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

	if err := c.parseBind(); err != nil {
		return err
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
		return errors.New("--attach-storage cannot be used on kubernetes models")
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
		return errors.New("--to cannot be used on kubernetes models")
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

type ModelConfigGetter interface {
	ModelGet() (map[string]interface{}, error)
}

var getModelConfig = func(api ModelConfigGetter) (*config.Config, error) {
	// Separated into a variable for easy overrides
	attrs, err := api.ModelGet()
	if err != nil {
		return nil, errors.Wrap(err, errors.New("cannot fetch model settings"))
	}

	return config.New(config.NoDefaults, attrs)
}

func (c *DeployCommand) deployBundle(
	ctx *cmd.Context,
	filePath string,
	data *charm.BundleData,
	bundleURL *charm.URL,
	channel params.Channel,
	apiRoot DeployAPI,
	bundleStorage map[string]map[string]storage.Constraints,
	bundleDevices map[string]map[string]devices.Constraints,
) (rErr error) {
	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}
	modelUUID, ok := apiRoot.ModelUUID()
	if !ok {
		return errors.New("API connection is controller-only (should never happen)")
	}

	for application, applicationSpec := range data.Applications {
		if applicationSpec.Plan != "" {
			for _, step := range c.Steps {
				s := step
				charmURL, err := charm.ParseURL(applicationSpec.Charm)
				if err != nil {
					return errors.Trace(err)
				}

				deployInfo := DeploymentInfo{
					CharmID:         charmstore.CharmID{URL: charmURL},
					ApplicationName: application,
					ApplicationPlan: applicationSpec.Plan,
					ModelUUID:       modelUUID,
					Force:           c.Force,
				}

				err = s.RunPre(apiRoot, bakeryClient, ctx, deployInfo)
				if err != nil {
					return errors.Trace(err)
				}

				defer func() {
					err = errors.Trace(s.RunPost(apiRoot, bakeryClient, ctx, deployInfo, rErr))
					if err != nil {
						rErr = err
					}
				}()
			}
		}
	}

	// TODO(ericsnow) Do something with the CS macaroons that were returned?
	// Deploying bundles does not allow the use force, it's expected that the
	// bundle is correct and therefore the charms are also.
	if _, err := deployBundle(
		filePath,
		data,
		bundleURL,
		c.BundleOverlayFile,
		channel,
		apiRoot,
		ctx,
		bundleStorage,
		bundleDevices,
		c.DryRun,
		c.UseExisting,
		c.BundleMachines,
	); err != nil {
		return errors.Annotate(err, "cannot deploy bundle")
	}
	return nil
}

func (c *DeployCommand) deployCharm(
	id charmstore.CharmID,
	csMac *macaroon.Macaroon,
	series string,
	ctx *cmd.Context,
	apiRoot DeployAPI,
) (rErr error) {
	charmInfo, err := apiRoot.CharmInfo(id.URL.String())
	if err != nil {
		return err
	}

	if len(c.AttachStorage) > 0 && apiRoot.BestFacadeVersion("Application") < 5 {
		// DeployArgs.AttachStorage is only supported from
		// Application API version 5 and onwards.
		return errors.New("this juju controller does not support --attach-storage")
	}

	// Storage cannot be added to a container.
	if len(c.Storage) > 0 || len(c.AttachStorage) > 0 {
		for _, placement := range c.Placement {
			if t, err := instance.ParseContainerType(placement.Scope); err == nil {
				return errors.NotSupportedf("adding storage to %s container", string(t))
			}
		}
	}

	numUnits := c.NumUnits
	if charmInfo.Meta.Subordinate {
		if !constraints.IsEmpty(&c.Constraints) {
			return errors.New("cannot use --constraints with subordinate application")
		}
		if numUnits == 1 && c.PlacementSpec == "" {
			numUnits = 0
		} else {
			return errors.New("cannot use --num-units or --to with subordinate application")
		}
	}
	applicationName := c.ApplicationName
	if applicationName == "" {
		applicationName = charmInfo.Meta.Name
	}

	// Process the --config args.
	// We may have a single file arg specified, in which case
	// it points to a YAML file keyed on the charm name and
	// containing values for any charm settings.
	// We may also have key/value pairs representing
	// charm settings which overrides anything in the YAML file.
	// If more than one file is specified, that is an error.
	var configYAML []byte
	files, err := c.ConfigOptions.AbsoluteFileNames(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if len(files) > 1 {
		return errors.Errorf("only a single config YAML file can be specified, got %d", len(files))
	}
	if len(files) == 1 {
		configYAML, err = ioutil.ReadFile(files[0])
		if err != nil {
			return errors.Trace(err)
		}
	}
	attr, err := c.ConfigOptions.ReadConfigPairs(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	appConfig := make(map[string]string)
	for k, v := range attr {
		appConfig[k] = v.(string)
	}

	// Expand the trust flag into the appConfig
	if c.Trust {
		appConfig[app.TrustConfigOptionName] = strconv.FormatBool(c.Trust)
	}

	// Application facade V5 expects charm config to either all be in YAML
	// or config map. If config map is specified, that overrides YAML.
	// So we need to combine the two here to have only one.
	if apiRoot.BestFacadeVersion("Application") < 6 && len(appConfig) > 0 {
		var configFromFile map[string]map[string]string
		err := yaml.Unmarshal(configYAML, &configFromFile)
		if err != nil {
			return errors.Annotate(err, "badly formatted YAML config file")
		}
		if configFromFile == nil {
			configFromFile = make(map[string]map[string]string)
		}
		charmSettings, ok := configFromFile[applicationName]
		if !ok {
			charmSettings = make(map[string]string)
		}
		for k, v := range appConfig {
			charmSettings[k] = v
		}
		appConfig = nil
		configFromFile[applicationName] = charmSettings
		configYAML, err = yaml.Marshal(configFromFile)
		if err != nil {
			return errors.Trace(err)
		}
	}

	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}

	uuid, ok := apiRoot.ModelUUID()
	if !ok {
		return errors.New("API connection is controller-only (should never happen)")
	}

	deployInfo := DeploymentInfo{
		CharmID:         id,
		ApplicationName: applicationName,
		ModelUUID:       uuid,
		CharmInfo:       charmInfo,
		Force:           c.Force,
	}

	for _, step := range c.Steps {
		err = step.RunPre(apiRoot, bakeryClient, ctx, deployInfo)
		if err != nil {
			return errors.Trace(err)
		}
	}

	defer func() {
		for _, step := range c.Steps {
			err = errors.Trace(step.RunPost(apiRoot, bakeryClient, ctx, deployInfo, rErr))
			if err != nil {
				rErr = err
			}
		}
	}()

	if id.URL != nil && id.URL.Schema != "local" && len(charmInfo.Meta.Terms) > 0 {
		ctx.Infof("Deployment under prior agreement to terms: %s",
			strings.Join(charmInfo.Meta.Terms, " "))
	}

	ids, err := resourceadapters.DeployResources(
		applicationName,
		id,
		csMac,
		c.Resources,
		charmInfo.Meta.Resources,
		apiRoot,
	)
	if err != nil {
		return errors.Trace(err)
	}

	if len(appConfig) == 0 {
		appConfig = nil
	}

	args := application.DeployArgs{
		CharmID:          id,
		Cons:             c.Constraints,
		ApplicationName:  applicationName,
		Series:           series,
		NumUnits:         numUnits,
		ConfigYAML:       string(configYAML),
		Config:           appConfig,
		Placement:        c.Placement,
		Storage:          c.Storage,
		Devices:          c.Devices,
		AttachStorage:    c.AttachStorage,
		Resources:        ids,
		EndpointBindings: c.Bindings,
	}
	return errors.Trace(apiRoot.Deploy(args))
}

const parseBindErrorPrefix = "--bind must be in the form '[<default-space>] [<endpoint-name>=<space> ...]'. "

// parseBind parses the --bind option. Valid forms are:
// * relation-name=space-name
// * extra-binding-name=space-name
// * space-name (equivalent to binding all endpoints to the same space, i.e. application-default)
// * The above in a space separated list to specify multiple bindings,
//   e.g. "rel1=space1 ext1=space2 space3"
func (c *DeployCommand) parseBind() error {
	bindings := make(map[string]string)
	if c.BindToSpaces == "" {
		return nil
	}

	for _, s := range strings.Split(c.BindToSpaces, " ") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}

		v := strings.Split(s, "=")
		var endpoint, space string
		switch len(v) {
		case 1:
			endpoint = ""
			space = v[0]
		case 2:
			if v[0] == "" {
				return errors.New(parseBindErrorPrefix + "Found = without endpoint name. Use a lone space name to set the default.")
			}
			endpoint = v[0]
			space = v[1]
		default:
			return errors.New(parseBindErrorPrefix + "Found multiple = in binding. Did you forget to space-separate the binding list?")
		}

		if !names.IsValidSpace(space) {
			return errors.New(parseBindErrorPrefix + "Space name invalid.")
		}
		bindings[endpoint] = space
	}
	c.Bindings = bindings
	return nil
}

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
	apiRoot, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	defer apiRoot.Close()

	for _, step := range c.Steps {
		step.SetPlanURL(apiRoot.PlanURL())
	}

	deploy, err := findDeployerFIFO(
		func() (deployFn, error) { return c.maybeReadLocalBundle(ctx) },
		func() (deployFn, error) { return c.maybeReadLocalCharm(apiRoot) },
		c.maybePredeployedLocalCharm,
		c.maybeReadCharmstoreBundleFn(apiRoot),
		c.charmStoreCharm, // This always returns a deployer
	)
	if err != nil {
		return errors.Trace(err)
	}

	return block.ProcessBlockedError(deploy(ctx, apiRoot), block.BlockChange)
}

func findDeployerFIFO(maybeDeployers ...func() (deployFn, error)) (deployFn, error) {
	for _, d := range maybeDeployers {
		if deploy, err := d(); err != nil {
			return nil, errors.Trace(err)
		} else if deploy != nil {
			return deploy, nil
		}
	}
	return nil, errors.NotFoundf("suitable deployer")
}

type deployFn func(*cmd.Context, DeployAPI) error

func (c *DeployCommand) validateBundleFlags() error {
	if flags := getFlags(c.flagSet, charmOnlyFlags()); len(flags) > 0 {
		return errors.Errorf("options provided but not supported when deploying a bundle: %s", strings.Join(flags, ", "))
	}
	return nil
}

func (c *DeployCommand) validateCharmFlags() error {
	if flags := getFlags(c.flagSet, bundleOnlyFlags); len(flags) > 0 {
		return errors.Errorf("options provided but not supported when deploying a charm: %s", strings.Join(flags, ", "))
	}
	return nil
}

func (c *DeployCommand) validateCharmSeries(series string) error {
	modelType, err := c.ModelType()
	if err != nil {
		return errors.Trace(err)
	}
	return model.ValidateSeries(modelType, series)
}

func (c *DeployCommand) validateResourcesNeededForLocalDeploy(charmMeta *charm.Meta) error {
	modelType, err := c.ModelType()
	if err != nil {
		return errors.Trace(err)
	}
	if modelType != model.CAAS {
		return nil
	}
	var missingImages []string
	for resName, resMeta := range charmMeta.Resources {
		if resMeta.Type == resource.TypeContainerImage {
			if _, ok := c.Resources[resName]; !ok {
				missingImages = append(missingImages, resName)
			}
		}
	}
	if len(missingImages) > 0 {
		return errors.Errorf("local charm missing OCI images for: %v", strings.Join(missingImages, ", "))
	}
	return nil
}

func (c *DeployCommand) maybePredeployedLocalCharm() (deployFn, error) {
	// If the charm's schema is local, we should definitively attempt
	// to deploy a charm that's already deployed in the
	// environment.
	userCharmURL, err := charm.ParseURL(c.CharmOrBundle)
	if err != nil {
		return nil, errors.Trace(err)
	} else if userCharmURL.Schema != "local" {
		logger.Debugf("cannot interpret as a redeployment of a local charm from the controller")
		return nil, nil
	}

	// Avoid deploying charm if it's not valid for the model.
	if err := c.validateCharmSeries(userCharmURL.Series); err != nil {
		return nil, errors.Trace(err)
	}

	return func(ctx *cmd.Context, api DeployAPI) error {
		if err := c.validateCharmFlags(); err != nil {
			return errors.Trace(err)
		}
		charmInfo, err := api.CharmInfo(userCharmURL.String())
		if err != nil {
			return err
		}
		if err := c.validateResourcesNeededForLocalDeploy(charmInfo.Meta); err != nil {
			return errors.Trace(err)
		}
		formattedCharmURL := userCharmURL.String()
		ctx.Infof("Located charm %q.", formattedCharmURL)
		ctx.Infof("Deploying charm %q.", formattedCharmURL)
		return errors.Trace(c.deployCharm(
			charmstore.CharmID{URL: userCharmURL},
			(*macaroon.Macaroon)(nil),
			userCharmURL.Series,
			ctx,
			api,
		))
	}, nil
}

// readLocalBundle returns the bundle data and bundle dir (for
// resolving includes) for the bundleFile passed in. If the bundle
// file doesn't exist we return nil.
func readLocalBundle(ctx *cmd.Context, bundleFile string) (*charm.BundleData, string, error) {
	bundleData, err := charmrepo.ReadBundleFile(bundleFile)
	if err == nil {
		// If the bundle is defined with just a yaml file, the bundle
		// path is the directory that holds the file.
		return bundleData, filepath.Dir(ctx.AbsPath(bundleFile)), nil
	}

	// We may have been given a local bundle archive or exploded directory.
	bundle, _, pathErr := charmrepo.NewBundleAtPath(bundleFile)
	if charmrepo.IsInvalidPathError(pathErr) {
		return nil, "", pathErr
	}
	if pathErr != nil {
		// If the bundle files existed but we couldn't read them,
		// then return that error rather than trying to interpret
		// as a charm.
		if info, statErr := os.Stat(bundleFile); statErr == nil {
			if info.IsDir() {
				if _, ok := pathErr.(*charmrepo.NotFoundError); !ok {
					return nil, "", errors.Trace(pathErr)
				}
			}
		}

		logger.Debugf("cannot interpret as local bundle: %v", err)
		return nil, "", errors.NotValidf("local bundle %q", bundleFile)
	}
	bundleData = bundle.Data()

	// If we get to here bundleFile is a directory, in which case
	// we should use the absolute path as the bundFilePath, or it is
	// an archive, in which case we should pass the empty string.
	var bundleDir string
	if info, err := os.Stat(bundleFile); err == nil && info.IsDir() {
		bundleDir = ctx.AbsPath(bundleFile)
	}

	return bundleData, bundleDir, nil
}

func (c *DeployCommand) maybeReadLocalBundle(ctx *cmd.Context) (deployFn, error) {
	bundleFile := c.CharmOrBundle
	bundleData, bundleDir, err := readLocalBundle(ctx, bundleFile)
	if charmrepo.IsInvalidPathError(err) {
		return nil, errors.Errorf(""+
			"The charm or bundle %q is ambiguous.\n"+
			"To deploy a local charm or bundle, run `juju deploy ./%[1]s`.\n"+
			"To deploy a charm or bundle from the store, run `juju deploy cs:%[1]s`.",
			bundleFile,
		)
	}
	if errors.IsNotValid(err) {
		// No problem reading it, but it's not a local bundle. Return
		// nil, nil to indicate the fallback pipeline should try the
		// next possibility.
		return nil, nil
	}
	if err != nil {
		return nil, errors.Annotate(err, "cannot deploy bundle")
	}
	if err := c.validateBundleFlags(); err != nil {
		return nil, errors.Trace(err)
	}

	return func(ctx *cmd.Context, apiRoot DeployAPI) error {
		return errors.Trace(c.deployBundle(
			ctx,
			bundleDir,
			bundleData,
			nil,
			c.Channel,
			apiRoot,
			c.BundleStorage,
			c.BundleDevices,
		))
	}, nil
}

func (c *DeployCommand) maybeReadLocalCharm(apiRoot DeployAPI) (deployFn, error) {
	// NOTE: Here we select the series using the algorithm defined by
	// `seriesSelector.CharmSeries`. This serves to override the algorithm found in
	// `charmrepo.NewCharmAtPath` which is outdated (but must still be
	// called since the code is coupled with path interpretation logic which
	// cannot easily be factored out).

	// NOTE: Reading the charm here is only meant to aid in inferring the correct
	// series, if this fails we fall back to the argument series. If reading
	// the charm fails here it will also fail below (the charm is read again
	// below) where it is handled properly. This is just an expedient to get
	// the correct series. A proper refactoring of the charmrepo package is
	// needed for a more elegant fix.

	ch, err := charm.ReadCharm(c.CharmOrBundle)
	series := c.Series
	if err == nil {
		modelCfg, err := getModelConfig(apiRoot)
		if err != nil {
			return nil, errors.Trace(err)
		}

		seriesSelector := seriesSelector{
			seriesFlag:      series,
			supportedSeries: ch.Meta().Series,
			force:           c.Force,
			conf:            modelCfg,
			fromBundle:      false,
		}

		series, err = seriesSelector.charmSeries()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	// Charm may have been supplied via a path reference.
	ch, curl, err := charmrepo.NewCharmAtPathForceSeries(c.CharmOrBundle, series, c.Force)
	// We check for several types of known error which indicate
	// that the supplied reference was indeed a path but there was
	// an issue reading the charm located there.
	if charm.IsMissingSeriesError(err) {
		return nil, err
	} else if charm.IsUnsupportedSeriesError(err) {
		return nil, errors.Trace(err)
	} else if errors.Cause(err) == zip.ErrFormat {
		return nil, errors.Errorf("invalid charm or bundle provided at %q", c.CharmOrBundle)
	} else if _, ok := err.(*charmrepo.NotFoundError); ok {
		return nil, errors.Wrap(err, errors.NotFoundf("charm or bundle at %q", c.CharmOrBundle))
	} else if err != nil && err != os.ErrNotExist {
		// If we get a "not exists" error then we attempt to interpret
		// the supplied charm reference as a URL elsewhere, otherwise
		// we return the error.
		return nil, errors.Trace(err)
	} else if err != nil {
		logger.Debugf("cannot interpret as local charm: %v", err)
		return nil, nil
	}

	// Avoid deploying charm if it's not valid for the model.
	if err := c.validateCharmSeries(series); err != nil {
		return nil, errors.Trace(err)
	}
	if err := c.validateResourcesNeededForLocalDeploy(ch.Meta()); err != nil {
		return nil, errors.Trace(err)
	}

	return func(ctx *cmd.Context, apiRoot DeployAPI) error {
		if err := c.validateCharmFlags(); err != nil {
			return errors.Trace(err)
		}

		if curl, err = apiRoot.AddLocalCharm(curl, ch, c.Force); err != nil {
			return errors.Trace(err)
		}

		id := charmstore.CharmID{
			URL: curl,
			// Local charms don't need a channel.
		}

		ctx.Infof("Deploying charm %q.", curl.String())
		return errors.Trace(c.deployCharm(
			id,
			(*macaroon.Macaroon)(nil), // local charms don't need one.
			curl.Series,
			ctx,
			apiRoot,
		))
	}, nil
}

// URLResolver is the part of charmrepo.Charmstore that we need to
// resolve a charm url.
type URLResolver interface {
	ResolveWithChannel(*charm.URL) (*charm.URL, params.Channel, []string, error)
}

// resolveBundleURL tries to interpret maybeBundle as a charmstore
// bundle. If it turns out to be a bundle, the resolved URL and
// channel are returned. If it isn't but there wasn't a problem
// checking it, it returns a nil charm URL.
func resolveBundleURL(store URLResolver, maybeBundle string) (*charm.URL, params.Channel, error) {
	userRequestedURL, err := charm.ParseURL(maybeBundle)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	// Charm or bundle has been supplied as a URL so we resolve and
	// deploy using the store.
	storeCharmOrBundleURL, channel, _, err := resolveCharm(store.ResolveWithChannel, userRequestedURL)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	if storeCharmOrBundleURL.Series != "bundle" {
		logger.Debugf(
			`cannot interpret as charmstore bundle: %v (series) != "bundle"`,
			storeCharmOrBundleURL.Series,
		)
		return nil, "", errors.NotValidf("charmstore bundle %q", maybeBundle)
	}
	return storeCharmOrBundleURL, channel, nil
}

func (c *DeployCommand) maybeReadCharmstoreBundleFn(apiRoot DeployAPI) func() (deployFn, error) {
	return func() (deployFn, error) {
		bundleURL, channel, err := resolveBundleURL(apiRoot, c.CharmOrBundle)
		if charm.IsUnsupportedSeriesError(errors.Cause(err)) {
			return nil, errors.Errorf("%v. Use --force to deploy the charm anyway.", err)
		}
		if errors.IsNotValid(err) {
			// The URL resolved alright, but not to a bundle.
			return nil, nil
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := c.validateBundleFlags(); err != nil {
			return nil, errors.Trace(err)
		}

		return func(ctx *cmd.Context, apiRoot DeployAPI) error {
			bundle, err := apiRoot.GetBundle(bundleURL)
			if err != nil {
				return errors.Trace(err)
			}
			ctx.Infof("Located bundle %q", bundleURL)
			data := bundle.Data()

			return errors.Trace(c.deployBundle(
				ctx,
				"", // filepath
				data,
				bundleURL,
				channel,
				apiRoot,
				c.BundleStorage,
				c.BundleDevices,
			))
		}, nil
	}
}

func (c *DeployCommand) getMeteringAPIURL(controllerAPIRoot api.Connection) (string, error) {
	controllerAPI := controller.NewClient(controllerAPIRoot)
	controllerCfg, err := controllerAPI.ControllerConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	return controllerCfg.MeteringURL(), nil
}

func (c *DeployCommand) charmStoreCharm() (deployFn, error) {
	userRequestedURL, err := charm.ParseURL(c.CharmOrBundle)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return func(ctx *cmd.Context, apiRoot DeployAPI) error {
		// resolver.resolve potentially updates the series of anything
		// passed in. Store this for use in seriesSelector.
		userRequestedSeries := userRequestedURL.Series

		modelCfg, err := getModelConfig(apiRoot)
		if err != nil {
			return errors.Trace(err)
		}

		// Charm or bundle has been supplied as a URL so we resolve and deploy using the store.
		storeCharmOrBundleURL, channel, supportedSeries, err := resolveCharm(
			apiRoot.ResolveWithChannel, userRequestedURL,
		)
		if charm.IsUnsupportedSeriesError(err) {
			return errors.Errorf("%v. Use --force to deploy the charm anyway.", err)
		} else if err != nil {
			return errors.Trace(err)
		}

		if err := c.validateCharmFlags(); err != nil {
			return errors.Trace(err)
		}

		selector := seriesSelector{
			charmURLSeries:  userRequestedSeries,
			seriesFlag:      c.Series,
			supportedSeries: supportedSeries,
			force:           c.Force,
			conf:            modelCfg,
			fromBundle:      false,
		}

		// Get the series to use.
		series, err := selector.charmSeries()

		// Avoid deploying charm if it's not valid for the model.
		// We check this first before possibly suggesting --force.
		if err == nil {
			if err2 := c.validateCharmSeries(series); err2 != nil {
				return errors.Trace(err2)
			}
		}

		if charm.IsUnsupportedSeriesError(err) {
			return errors.Errorf("%v. Use --force to deploy the charm anyway.", err)
		}

		// Store the charm in the controller
		curl, csMac, err := addCharmFromURL(apiRoot, storeCharmOrBundleURL, channel, c.Force)
		if err != nil {
			if termErr, ok := errors.Cause(err).(*common.TermsRequiredError); ok {
				return errors.Trace(termErr.UserErr())
			}
			return errors.Annotatef(err, "storing charm for URL %q", storeCharmOrBundleURL)
		}

		formattedCharmURL := curl.String()
		ctx.Infof("Located charm %q.", formattedCharmURL)
		ctx.Infof("Deploying charm %q.", formattedCharmURL)
		id := charmstore.CharmID{
			URL:     curl,
			Channel: channel,
		}
		return errors.Trace(c.deployCharm(
			id,
			csMac,
			series,
			ctx,
			apiRoot,
		))
	}, nil
}

// getFlags returns the flags with the given names. Only flags that are set and
// whose name is included in flagNames are included.
func getFlags(flagSet *gnuflag.FlagSet, flagNames []string) []string {
	flags := make([]string, 0, flagSet.NFlag())
	flagSet.Visit(func(flag *gnuflag.Flag) {
		for _, name := range flagNames {
			if flag.Name == name {
				flags = append(flags, flagWithMinus(name))
			}
		}
	})
	return flags
}

func flagWithMinus(name string) string {
	if len(name) > 1 {
		return "--" + name
	}
	return "-" + name
}
