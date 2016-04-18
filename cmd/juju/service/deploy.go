// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable"
	csclientparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	apiannotations "github.com/juju/juju/api/annotations"
	apiservice "github.com/juju/juju/api/service"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/storage"
)

var planURL = "https://api.jujucharms.com/omnibus/v2"

// NewDeployCommand returns a command to deploy services.
func NewDeployCommand() cmd.Command {
	return modelcmd.Wrap(&DeployCommand{
		Steps: []DeployStep{
			&RegisterMeteredCharm{
				RegisterURL: planURL + "/plan/authorize",
				QueryURL:    planURL + "/charm",
			},
		}})
}

type DeployCommand struct {
	modelcmd.ModelCommandBase
	UnitCommandBase
	// CharmOrBundle is either a charm URL, a path where a charm can be found,
	// or a bundle name.
	CharmOrBundle string

	// Channel holds the charmstore channel to use when obtaining
	// the charm to be deployed.
	Channel csclientparams.Channel

	Series string

	// Force is used to allow a charm to be deployed onto a machine
	// running an unsupported series.
	Force bool

	ServiceName  string
	Config       cmd.FileVar
	Constraints  constraints.Value
	BindToSpaces string

	// TODO(axw) move this to UnitCommandBase once we support --storage
	// on add-unit too.
	//
	// Storage is a map of storage constraints, keyed on the storage name
	// defined in charm storage metadata.
	Storage map[string]storage.Constraints

	// BundleStorage maps service names to maps of storage constraints keyed on
	// the storage name defined in that service's charm storage metadata.
	BundleStorage map[string]map[string]storage.Constraints

	// Resources is a map of resource name to filename to be uploaded on deploy.
	Resources map[string]string

	Bindings map[string]string
	Steps    []DeployStep

	flagSet *gnuflag.FlagSet
}

const deployDoc = `
<charm or bundle> can be a charm/bundle URL, or an unambiguously condensed
form of it; assuming a current series of "trusty", the following forms will be
accepted:

For cs:trusty/mysql
  mysql
  trusty/mysql

For cs:~user/trusty/mysql
  cs:~user/mysql

For cs:bundle/mediawiki-single
  mediawiki-single
  bundle/mediawiki-single

The current series for charms is determined first by the default-series model
setting, followed by the preferred series for the charm in the charm store.

In these cases, a versioned charm URL will be expanded as expected (for example,
mysql-33 becomes cs:precise/mysql-33).

Charms may also be deployed from a user specified path. In this case, the
path to the charm is specified along with an optional series.

   juju deploy /path/to/charm --series trusty

If series is not specified, the charm's default series is used. The default series
for a charm is the first one specified in the charm metadata. If the specified series
is not supported by the charm, this results in an error, unless --force is used.

   juju deploy /path/to/charm --series wily --force

Local bundles are specified with a direct path to a bundle.yaml file.
For example:

  juju deploy /path/to/bundle/openstack/bundle.yaml

<service name>, if omitted, will be derived from <charm name>.

Constraints can be specified when using deploy by specifying the --constraints
flag.  When used with deploy, service-specific constraints are set so that later
machines provisioned with add-unit will use the same constraints (unless changed
by set-constraints).

Resources may be uploaded at deploy time by specifying the --resource flag.
Following the resource flag should be name=filepath pair.  This flag may be
repeated more than once to upload more than one resource.

  juju deploy foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml

Where bar and baz are resources named in the metadata for the foo charm.

Charms can be deployed to a specific machine using the --to argument.
If the destination is an LXC container the default is to use lxc-clone
to create the container where possible. For Ubuntu deployments, lxc-clone
is supported for the trusty OS series and later. A 'template' container is
created with the name
  juju-<series>-template
where <series> is the OS series, for example 'juju-trusty-template'.

You can override the use of clone by changing the provider configuration:
  lxc-clone: false

In more complex scenarios, Juju's network spaces are used to partition the cloud
networking layer into sets of subnets. Instances hosting units inside the
same space can communicate with each other without any firewalls. Traffic
crossing space boundaries could be subject to firewall and access restrictions.
Using spaces as deployment targets, rather than their individual subnets allows
Juju to perform automatic distribution of units across availability zones to
support high availability for services. Spaces help isolate services and their
units, both for security purposes and to manage both traffic segregation and
congestion.

When deploying a service or adding machines, the "spaces" constraint can be
used to define a comma-delimited list of required and forbidden spaces
(the latter prefixed with "^", similar to the "tags" constraint).

If you have the main container directory mounted on a btrfs partition,
then the clone will be using btrfs snapshots to create the containers.
This means that clones use up much less disk space.  If you do not have btrfs,
lxc will attempt to use aufs (an overlay type filesystem). You can
explicitly ask Juju to create full containers and not overlays by specifying
the following in the provider configuration:
  lxc-clone-aufs: false

Examples:
   juju deploy mysql --to 23       (deploy to machine 23)
   juju deploy mysql --to 24/lxc/3 (deploy to lxc container 3 on host machine 24)
   juju deploy mysql --to lxc:25   (deploy to a new lxc container on host machine 25)

   juju deploy mysql -n 5 --constraints mem=8G
   (deploy 5 instances of mysql with at least 8 GB of RAM each)

   juju deploy haproxy -n 2 --constraints spaces=dmz,^cms,^database
   (deploy 2 instances of haproxy on cloud instances being part of the dmz
    space but not of the cmd and the database space)

See Also:
   juju help spaces
   juju help constraints
   juju help set-constraints
   juju help get-constraints
`

// DeployStep is an action that needs to be taken during charm deployment.
type DeployStep interface {
	// Set flags necessary for the deploy step.
	SetFlags(*gnuflag.FlagSet)
	// RunPre runs before the call is made to add the charm to the environment.
	RunPre(api.Connection, *httpbakery.Client, *cmd.Context, DeploymentInfo) error
	// RunPost runs after the call is made to add the charm to the environment.
	// The error parameter is used to notify the step of a previously occurred error.
	RunPost(api.Connection, *httpbakery.Client, *cmd.Context, DeploymentInfo, error) error
}

// DeploymentInfo is used to maintain all deployment information for
// deployment steps.
type DeploymentInfo struct {
	CharmID     charmstore.CharmID
	ServiceName string
	ModelUUID   string
}

func (c *DeployCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "deploy",
		Args:    "<charm or bundle> [<service name>]",
		Purpose: "deploy a new service or bundle",
		Doc:     deployDoc,
	}
}

var (
	// charmOnlyFlags and bundleOnlyFlags are used to validate flags based on
	// whether we are deploying a charm or a bundle.
	charmOnlyFlags  = []string{"bind", "config", "constraints", "force", "n", "num-units", "series", "to", "resource"}
	bundleOnlyFlags = []string{}
)

func (c *DeployCommand) SetFlags(f *gnuflag.FlagSet) {
	// Keep above charmOnlyFlags and bundleOnlyFlags lists updated when adding
	// new flags.
	c.UnitCommandBase.SetFlags(f)
	f.IntVar(&c.NumUnits, "n", 1, "number of service units to deploy for principal charms")
	f.StringVar((*string)(&c.Channel), "channel", "", "channel to use when getting the charm or bundle from the charm store")
	f.Var(&c.Config, "config", "path to yaml-formatted service config")
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints}, "constraints", "set service constraints")
	f.StringVar(&c.Series, "series", "", "the series on which to deploy")
	f.BoolVar(&c.Force, "force", false, "allow a charm to be deployed to a machine running an unsupported series")
	f.Var(storageFlag{&c.Storage, &c.BundleStorage}, "storage", "charm storage constraints")
	f.Var(stringMap{&c.Resources}, "resource", "resource to be uploaded to the controller")
	f.StringVar(&c.BindToSpaces, "bind", "", "Configure service endpoint bindings to spaces")

	for _, step := range c.Steps {
		step.SetFlags(f)
	}
	c.flagSet = f
}

func (c *DeployCommand) Init(args []string) error {
	if c.Force && c.Series == "" && c.PlacementSpec == "" {
		return errors.New("--force is only used with --series")
	}
	switch len(args) {
	case 2:
		if !names.IsValidService(args[1]) {
			return fmt.Errorf("invalid service name %q", args[1])
		}
		c.ServiceName = args[1]
		fallthrough
	case 1:
		c.CharmOrBundle = args[0]
	case 0:
		return errors.New("no charm or bundle specified")
	default:
		return cmd.CheckEmpty(args[2:])
	}
	err := c.parseBind()
	if err != nil {
		return err
	}
	return c.UnitCommandBase.Init(args)
}

type ModelConfigGetter interface {
	ModelGet() (map[string]interface{}, error)
}

var getClientConfig = func(client ModelConfigGetter) (*config.Config, error) {
	// Separated into a variable for easy overrides
	attrs, err := client.ModelGet()
	if err != nil {
		return nil, err
	}

	return config.New(config.NoDefaults, attrs)
}

func (c *DeployCommand) maybeReadLocalBundleData(ctx *cmd.Context) (
	_ *charm.BundleData, bundleFile string, bundleFilePath string, _ error,
) {
	bundleFile = c.CharmOrBundle
	bundleData, err := charmrepo.ReadBundleFile(bundleFile)
	if err == nil {
		// For local bundles, we extract the local path of
		// the bundle directory.
		bundleFilePath = filepath.Dir(ctx.AbsPath(bundleFile))
	} else {
		// We may have been given a local bundle archive or exploded directory.
		if bundle, burl, pathErr := charmrepo.NewBundleAtPath(bundleFile); pathErr == nil {
			bundleData = bundle.Data()
			bundleFile = burl.String()
			if info, err := os.Stat(bundleFile); err == nil && info.IsDir() {
				bundleFilePath = bundleFile
			}
			err = nil
		} else {
			err = pathErr
		}
	}
	return bundleData, bundleFile, bundleFilePath, err
}

func (c *DeployCommand) deployCharmOrBundle(ctx *cmd.Context, client *api.Client) error {
	deployer := serviceDeployer{ctx, c}

	// We may have been given a local bundle file.
	bundleData, bundleIdent, bundleFilePath, err := c.maybeReadLocalBundleData(ctx)
	// If the bundle files existed but we couldn't read them, then
	// return that error rather than trying to interpret as a charm.
	if err != nil {
		if info, statErr := os.Stat(c.CharmOrBundle); statErr == nil {
			if info.IsDir() {
				if _, ok := err.(*charmrepo.NotFoundError); !ok {
					return err
				}
			}
		}
	}

	// If not a bundle then maybe a local charm.
	if err != nil {
		// Charm may have been supplied via a path reference.
		ch, curl, charmErr := charmrepo.NewCharmAtPathForceSeries(c.CharmOrBundle, c.Series, c.Force)
		if charmErr == nil {
			if curl, charmErr = client.AddLocalCharm(curl, ch); charmErr != nil {
				return charmErr
			}
			id := charmstore.CharmID{
				URL: curl,
				// Local charms don't need a channel.
			}
			var csMac *macaroon.Macaroon // local charms don't need one.
			return c.deployCharm(deployCharmArgs{
				id:       id,
				csMac:    csMac,
				series:   curl.Series,
				ctx:      ctx,
				client:   client,
				deployer: &deployer,
			})
		}
		// We check for several types of known error which indicate
		// that the supplied reference was indeed a path but there was
		// an issue reading the charm located there.
		if charm.IsMissingSeriesError(charmErr) {
			return charmErr
		}
		if charm.IsUnsupportedSeriesError(charmErr) {
			return errors.Errorf("%v. Use --force to deploy the charm anyway.", charmErr)
		}
		if errors.Cause(charmErr) == zip.ErrFormat {
			return errors.Errorf("invalid charm or bundle provided at %q", c.CharmOrBundle)
		}
		err = charmErr
	}
	if _, ok := err.(*charmrepo.NotFoundError); ok {
		return errors.Errorf("no charm or bundle found at %q", c.CharmOrBundle)
	}
	// If we get a "not exists" error then we attempt to interpret the supplied
	// charm or bundle reference as a URL below, otherwise we return the error.
	if err != nil && err != os.ErrNotExist {
		return err
	}

	conf, err := getClientConfig(client)
	if err != nil {
		return err
	}

	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}
	csClient := newCharmStoreClient(bakeryClient).WithChannel(c.Channel)

	resolver := newCharmURLResolver(conf, csClient)

	var storeCharmOrBundleURL *charm.URL
	var store *charmrepo.CharmStore
	var supportedSeries []string
	// If we don't already have a bundle loaded, we try the charm store for a charm or bundle.
	if bundleData == nil {
		// Charm or bundle has been supplied as a URL so we resolve and deploy using the store.
		storeCharmOrBundleURL, c.Channel, supportedSeries, store, err = resolver.resolve(c.CharmOrBundle)
		if charm.IsUnsupportedSeriesError(err) {
			return errors.Errorf("%v. Use --force to deploy the charm anyway.", err)
		}
		if err != nil {
			return errors.Trace(err)
		}
		if storeCharmOrBundleURL.Series == "bundle" {
			// Load the bundle entity.
			bundle, err := store.GetBundle(storeCharmOrBundleURL)
			if err != nil {
				return errors.Trace(err)
			}
			bundleData = bundle.Data()
			bundleIdent = storeCharmOrBundleURL.String()
		}
	}
	// Handle a bundle.
	if bundleData != nil {
		if flags := getFlags(c.flagSet, charmOnlyFlags); len(flags) > 0 {
			return errors.Errorf("Flags provided but not supported when deploying a bundle: %s.", strings.Join(flags, ", "))
		}
		// TODO(ericsnow) Do something with the CS macaroons that were returned?
		if _, err := deployBundle(
			bundleFilePath, bundleData, c.Channel, client, &deployer, resolver, ctx, c.BundleStorage,
		); err != nil {
			return errors.Trace(err)
		}
		ctx.Infof("deployment of bundle %q completed", bundleIdent)
		return nil
	}
	// Handle a charm.
	if flags := getFlags(c.flagSet, bundleOnlyFlags); len(flags) > 0 {
		return errors.Errorf("Flags provided but not supported when deploying a charm: %s.", strings.Join(flags, ", "))
	}
	// Get the series to use.
	series, message, err := charmSeries(c.Series, storeCharmOrBundleURL.Series, supportedSeries, c.Force, conf, deployFromCharm)
	if charm.IsUnsupportedSeriesError(err) {
		return errors.Errorf("%v. Use --force to deploy the charm anyway.", err)
	}
	// Store the charm in state.
	curl, csMac, err := addCharmFromURL(client, storeCharmOrBundleURL, c.Channel, csClient)
	if err != nil {
		if err1, ok := errors.Cause(err).(*termsRequiredError); ok {
			terms := strings.Join(err1.Terms, " ")
			return errors.Errorf(`Declined: please agree to the following terms %s. Try: "juju agree %s"`, terms, terms)
		}
		return errors.Annotatef(err, "storing charm for URL %q", storeCharmOrBundleURL)
	}
	ctx.Infof("Added charm %q to the model.", curl)
	ctx.Infof("Deploying charm %q %v.", curl, fmt.Sprintf(message, series))
	id := charmstore.CharmID{
		URL:     curl,
		Channel: c.Channel,
	}
	return c.deployCharm(deployCharmArgs{
		id:       id,
		csMac:    csMac,
		series:   series,
		ctx:      ctx,
		client:   client,
		deployer: &deployer,
	})
}

const (
	msgUserRequestedSeries = "with the user specified series %q"
	msgBundleSeries        = "with the series %q defined by the bundle"
	msgSingleCharmSeries   = "with the charm series %q"
	msgDefaultCharmSeries  = "with the default charm metadata series %q"
	msgDefaultModelSeries  = "with the configured model default series %q"
	msgLatestLTSSeries     = "with the latest LTS series %q"
)

const (
	// deployFromBundle is passed to charmSeries when deploying from a bundle.
	deployFromBundle = true

	// deployFromCharm is passed to charmSeries when deploying a charm.
	deployFromCharm = false
)

// charmSeries determine what series to use with a charm.
// Order of preference is:
// - user requested or defined by bundle when deploying
// - default from charm metadata supported series
// - model default
// - charm store default
func charmSeries(
	requestedSeries, seriesFromCharm string,
	supportedSeries []string,
	force bool,
	conf *config.Config,
	fromBundle bool,
) (string, string, error) {
	// User has requested a series and we have a new charm with series in metadata.
	if requestedSeries != "" && seriesFromCharm == "" {
		if !force && !isSeriesSupported(requestedSeries, supportedSeries) {
			return "", "", charm.NewUnsupportedSeriesError(requestedSeries, supportedSeries)
		}
		if fromBundle {
			return requestedSeries, msgBundleSeries, nil
		} else {
			return requestedSeries, msgUserRequestedSeries, nil
		}
	}

	// User has requested a series and it's an old charm for a single series.
	if seriesFromCharm != "" {
		if !force && requestedSeries != "" && requestedSeries != seriesFromCharm {
			return "", "", charm.NewUnsupportedSeriesError(requestedSeries, []string{seriesFromCharm})
		}
		if requestedSeries != "" {
			if fromBundle {
				return requestedSeries, msgBundleSeries, nil
			} else {
				return requestedSeries, msgUserRequestedSeries, nil
			}
		}
		return seriesFromCharm, msgSingleCharmSeries, nil
	}

	// Use charm default.
	if len(supportedSeries) > 0 {
		return supportedSeries[0], msgDefaultCharmSeries, nil
	}

	// Use model default supported series.
	if defaultSeries, ok := conf.DefaultSeries(); ok {
		if !force && !isSeriesSupported(defaultSeries, supportedSeries) {
			return "", "", charm.NewUnsupportedSeriesError(defaultSeries, supportedSeries)
		}
		return defaultSeries, msgDefaultModelSeries, nil
	}

	// Use latest LTS.
	latestLtsSeries := config.LatestLtsSeries()
	if !force && !isSeriesSupported(latestLtsSeries, supportedSeries) {
		return "", "", charm.NewUnsupportedSeriesError(latestLtsSeries, supportedSeries)
	}
	return latestLtsSeries, msgLatestLTSSeries, nil
}

type deployCharmArgs struct {
	id       charmstore.CharmID
	csMac    *macaroon.Macaroon
	series   string
	ctx      *cmd.Context
	client   *api.Client
	deployer *serviceDeployer
}

func (c *DeployCommand) deployCharm(args deployCharmArgs) (rErr error) {
	charmInfo, err := args.client.CharmInfo(args.id.URL.String())
	if err != nil {
		return err
	}

	numUnits := c.NumUnits
	if charmInfo.Meta.Subordinate {
		if !constraints.IsEmpty(&c.Constraints) {
			return errors.New("cannot use --constraints with subordinate service")
		}
		if numUnits == 1 && c.PlacementSpec == "" {
			numUnits = 0
		} else {
			return errors.New("cannot use --num-units or --to with subordinate service")
		}
	}
	serviceName := c.ServiceName
	if serviceName == "" {
		serviceName = charmInfo.Meta.Name
	}

	var configYAML []byte
	if c.Config.Path != "" {
		configYAML, err = c.Config.Read(args.ctx)
		if err != nil {
			return err
		}
	}

	state, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}

	deployInfo := DeploymentInfo{
		CharmID:     args.id,
		ServiceName: serviceName,
		ModelUUID:   args.client.ModelUUID(),
	}

	for _, step := range c.Steps {
		err = step.RunPre(state, bakeryClient, args.ctx, deployInfo)
		if err != nil {
			return err
		}
	}

	defer func() {
		for _, step := range c.Steps {
			err = step.RunPost(state, bakeryClient, args.ctx, deployInfo, rErr)
			if err != nil {
				rErr = err
			}
		}
	}()

	if len(charmInfo.Meta.Terms) > 0 {
		args.ctx.Infof("Deployment under prior agreement to terms: %s",
			strings.Join(charmInfo.Meta.Terms, " "))
	}

	ids, err := handleResources(c, c.Resources, serviceName, args.id, args.csMac, charmInfo.Meta.Resources)
	if err != nil {
		return errors.Trace(err)
	}

	params := serviceDeployParams{
		charmID:       args.id,
		serviceName:   serviceName,
		series:        args.series,
		numUnits:      numUnits,
		configYAML:    string(configYAML),
		constraints:   c.Constraints,
		placement:     c.Placement,
		storage:       c.Storage,
		spaceBindings: c.Bindings,
		resources:     ids,
	}
	return args.deployer.serviceDeploy(params)
}

type APICmd interface {
	NewAPIRoot() (api.Connection, error)
}

func handleResources(c APICmd, resources map[string]string, serviceName string, chID charmstore.CharmID, csMac *macaroon.Macaroon, metaResources map[string]charmresource.Meta) (map[string]string, error) {
	if len(resources) == 0 && len(metaResources) == 0 {
		return nil, nil
	}

	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ids, err := resourceadapters.DeployResources(serviceName, chID, csMac, resources, metaResources, api)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return ids, nil
}

const parseBindErrorPrefix = "--bind must be in the form '[<default-space>] [<endpoint-name>=<space> ...]'. "

// parseBind parses the --bind option. Valid forms are:
// * relation-name=space-name
// * extra-binding-name=space-name
// * space-name (equivalent to binding all endpoints to the same space, i.e. service-default)
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

type serviceDeployParams struct {
	charmID       charmstore.CharmID
	serviceName   string
	series        string
	numUnits      int
	configYAML    string
	constraints   constraints.Value
	placement     []*instance.Placement
	storage       map[string]storage.Constraints
	spaceBindings map[string]string
	resources     map[string]string
}

type serviceDeployer struct {
	ctx *cmd.Context
	api APICmd
}

func (d *serviceDeployer) newServiceAPIClient() (*apiservice.Client, error) {
	root, err := d.api.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiservice.NewClient(root), nil
}

func (d *serviceDeployer) newAnnotationsAPIClient() (*apiannotations.Client, error) {
	root, err := d.api.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiannotations.NewClient(root), nil
}

func (c *serviceDeployer) serviceDeploy(args serviceDeployParams) error {
	serviceClient, err := c.newServiceAPIClient()
	if err != nil {
		return err
	}
	defer serviceClient.Close()
	for i, p := range args.placement {
		if p.Scope == "model-uuid" {
			p.Scope = serviceClient.ModelUUID()
		}
		args.placement[i] = p
	}

	clientArgs := apiservice.DeployArgs{
		CharmID:          args.charmID,
		ServiceName:      args.serviceName,
		Series:           args.series,
		NumUnits:         args.numUnits,
		ConfigYAML:       args.configYAML,
		Cons:             args.constraints,
		Placement:        args.placement,
		Networks:         []string{},
		Storage:          args.storage,
		EndpointBindings: args.spaceBindings,
		Resources:        args.resources,
	}

	return serviceClient.Deploy(clientArgs)
}

func (c *DeployCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()

	err = c.deployCharmOrBundle(ctx, client)
	return block.ProcessBlockedError(err, block.BlockChange)
}

type metricCredentialsAPI interface {
	SetMetricCredentials(string, []byte) error
	Close() error
}

type metricsCredentialsAPIImpl struct {
	api   *apiservice.Client
	state api.Connection
}

// SetMetricCredentials sets the credentials on the service.
func (s *metricsCredentialsAPIImpl) SetMetricCredentials(serviceName string, data []byte) error {
	return s.api.SetMetricCredentials(serviceName, data)
}

// Close closes the api connection
func (s *metricsCredentialsAPIImpl) Close() error {
	err := s.state.Close()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

var getMetricCredentialsAPI = func(state api.Connection) (metricCredentialsAPI, error) {
	return &metricsCredentialsAPIImpl{api: apiservice.NewClient(state), state: state}, nil
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
