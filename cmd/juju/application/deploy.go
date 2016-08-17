// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	csclientparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	apiannotations "github.com/juju/juju/api/annotations"
	"github.com/juju/juju/api/application"
	apicharms "github.com/juju/juju/api/charms"
	"github.com/juju/juju/api/modelconfig"
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

	// Series is the series of the charm to deploy.
	Series string

	// Force is used to allow a charm to be deployed onto a machine
	// running an unsupported series.
	Force bool

	ApplicationName string
	Config          cmd.FileVar
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
  ~user/mysql

For cs:bundle/mediawiki-single
  mediawiki-single
  bundle/mediawiki-single

The current series for charms is determined first by the 'default-series' model
setting, followed by the preferred series for the charm in the charm store.

In these cases, a versioned charm URL will be expanded as expected (for
example, mysql-33 becomes cs:precise/mysql-33).

Charms may also be deployed from a user specified path. In this case, the path
to the charm is specified along with an optional series.

  juju deploy /path/to/charm --series trusty

If '--series' is not specified, the charm's default series is used. The default
series for a charm is the first one specified in the charm metadata. If the
specified series is not supported by the charm, this results in an error,
unless '--force' is used.

  juju deploy /path/to/charm --series wily --force

Local bundles are specified with a direct path to a bundle.yaml file.
For example:

  juju deploy /path/to/bundle/openstack/bundle.yaml

If an 'application name' is not provided, the application name used is the
'charm or bundle' name.

Constraints can be specified by specifying the '--constraints' option. If the
application is later scaled out with ` + "`juju add-unit`" + `, provisioned machines
will use the same constraints (unless changed by ` + "`juju set-constraints`" + `).

Resources may be uploaded by specifying the '--resource' option followed by a
name=filepath pair. This option may be repeated more than once to upload more
than one resource.

  juju deploy foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml

Where 'bar' and 'baz' are resources named in the metadata for the 'foo' charm.

When using a placement directive to deploy to an existing machine or container
('--to' option), the ` + "`juju status`" + ` command should be used for guidance. A few
placement directives are provider-dependent (e.g.: 'zone').

In more complex scenarios, Juju's network spaces are used to partition the
cloud networking layer into sets of subnets. Instances hosting units inside the
same space can communicate with each other without any firewalls. Traffic
crossing space boundaries could be subject to firewall and access restrictions.
Using spaces as deployment targets, rather than their individual subnets,
allows Juju to perform automatic distribution of units across availability zones
to support high availability for applications. Spaces help isolate applications
and their units, both for security purposes and to manage both traffic
segregation and congestion.

When deploying an application or adding machines, the 'spaces' constraint can
be used to define a comma-delimited list of required and forbidden spaces (the
latter prefixed with "^", similar to the 'tags' constraint).


Examples:
    juju deploy mysql --to 23       (deploy to machine 23)
    juju deploy mysql --to 24/lxd/3 (deploy to lxd container 3 on machine 24)
    juju deploy mysql --to lxd:25   (deploy to a new lxd container on machine 25)
    juju deploy mysql --to lxd      (deploy to a new lxd container on a new machine)

    juju deploy mysql --to zone=us-east-1a
    (provider-dependent; deploy to a specific AZ)

    juju deploy mysql --to host.maas
    (deploy to a specific MAAS node)

    juju deploy mysql -n 5 --constraints mem=8G
    (deploy 5 units to machines with at least 8 GB of memory)

    juju deploy haproxy -n 2 --constraints spaces=dmz,^cms,^database
    (deploy 2 units to machines that are part of the 'dmz' space but not of the
    'cmd' or the 'database' spaces)

See also:
    spaces
    constraints
    add-unit
    set-config
    get-config
    set-constraints
    get-constraints
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
	CharmID         charmstore.CharmID
	ApplicationName string
	ModelUUID       string
}

func (c *DeployCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "deploy",
		Args:    "<charm or bundle> [<application name>]",
		Purpose: "Deploy a new application or bundle.",
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
	f.IntVar(&c.NumUnits, "n", 1, "Number of application units to deploy for principal charms")
	f.StringVar((*string)(&c.Channel), "channel", "", "Channel to use when getting the charm or bundle from the charm store")
	f.Var(&c.Config, "config", "Path to yaml-formatted application config")
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints}, "constraints", "Set application constraints")
	f.StringVar(&c.Series, "series", "", "The series on which to deploy")
	f.BoolVar(&c.Force, "force", false, "Allow a charm to be deployed to a machine running an unsupported series")
	f.Var(storageFlag{&c.Storage, &c.BundleStorage}, "storage", "Charm storage constraints")
	f.Var(stringMap{&c.Resources}, "resource", "Resource to be uploaded to the controller")
	f.StringVar(&c.BindToSpaces, "bind", "", "Configure application endpoint bindings to spaces")

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
		if !names.IsValidApplication(args[1]) {
			return fmt.Errorf("invalid application name %q", args[1])
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
	err := c.parseBind()
	if err != nil {
		return err
	}
	return c.UnitCommandBase.Init(args)
}

type ModelConfigGetter interface {
	ModelGet() (map[string]interface{}, error)
}

var getModelConfig = func(client ModelConfigGetter) (*config.Config, error) {
	// Separated into a variable for easy overrides
	attrs, err := client.ModelGet()
	if err != nil {
		return nil, err
	}

	return config.New(config.NoDefaults, attrs)
}

func (c *DeployCommand) deployBundle(
	ctx *cmd.Context,
	ident string,
	filePath string,
	data *charm.BundleData,
	channel csclientparams.Channel,
	apiClient *api.Client,
	appDeployer *applicationDeployer,
	resolver *charmURLResolver,
	bundleStorage map[string]map[string]storage.Constraints,
) error {
	// TODO(ericsnow) Do something with the CS macaroons that were returned?
	if _, err := deployBundle(
		filePath,
		data,
		channel,
		apiClient,
		appDeployer,
		resolver,
		ctx,
		bundleStorage,
	); err != nil {
		return errors.Trace(err)
	}
	ctx.Infof("deployment of bundle %q completed", ident)
	return nil
}

type deployCharmArgs struct {
	id       charmstore.CharmID
	csMac    *macaroon.Macaroon
	series   string
	ctx      *cmd.Context
	client   *api.Client
	deployer *applicationDeployer
}

func (c *DeployCommand) deployCharm(args deployCharmArgs) (rErr error) {
	conn, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	charmsClient := apicharms.NewClient(conn)
	charmInfo, err := charmsClient.CharmInfo(args.id.URL.String())
	if err != nil {
		return err
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
	serviceName := c.ApplicationName
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

	uuid, ok := args.client.ModelUUID()
	if !ok {
		return errors.New("API connection is controller-only (should never happen)")
	}

	deployInfo := DeploymentInfo{
		CharmID:         args.id,
		ApplicationName: serviceName,
		ModelUUID:       uuid,
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

	if args.id.URL != nil && args.id.URL.Schema != "local" && len(charmInfo.Meta.Terms) > 0 {
		args.ctx.Infof("Deployment under prior agreement to terms: %s",
			strings.Join(charmInfo.Meta.Terms, " "))
	}

	ids, err := handleResources(c, c.Resources, serviceName, args.id, args.csMac, charmInfo.Meta.Resources)
	if err != nil {
		return errors.Trace(err)
	}

	params := applicationDeployParams{
		charmID:         args.id,
		applicationName: serviceName,
		series:          args.series,
		numUnits:        numUnits,
		configYAML:      string(configYAML),
		constraints:     c.Constraints,
		placement:       c.Placement,
		storage:         c.Storage,
		spaceBindings:   c.Bindings,
		resources:       ids,
	}
	return args.deployer.applicationDeploy(params)
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

type applicationDeployParams struct {
	charmID         charmstore.CharmID
	applicationName string
	series          string
	numUnits        int
	configYAML      string
	constraints     constraints.Value
	placement       []*instance.Placement
	storage         map[string]storage.Constraints
	spaceBindings   map[string]string
	resources       map[string]string
}

type applicationDeployer struct {
	ctx *cmd.Context
	api APICmd
}

func (d *applicationDeployer) newApplicationAPIClient() (*application.Client, error) {
	root, err := d.api.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (d *applicationDeployer) newModelConfigAPIClient() (*modelconfig.Client, error) {
	root, err := d.api.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(root), nil
}

func (d *applicationDeployer) newAnnotationsAPIClient() (*apiannotations.Client, error) {
	root, err := d.api.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiannotations.NewClient(root), nil
}

func (d *applicationDeployer) newCharmsAPIClient() (*apicharms.Client, error) {
	root, err := d.api.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apicharms.NewClient(root), nil
}

func (c *applicationDeployer) applicationDeploy(args applicationDeployParams) error {
	serviceClient, err := c.newApplicationAPIClient()
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

	clientArgs := application.DeployArgs{
		CharmID:          args.charmID,
		ApplicationName:  args.applicationName,
		Series:           args.series,
		NumUnits:         args.numUnits,
		ConfigYAML:       args.configYAML,
		Cons:             args.constraints,
		Placement:        args.placement,
		Storage:          args.storage,
		EndpointBindings: args.spaceBindings,
		Resources:        args.resources,
	}

	return serviceClient.Deploy(clientArgs)
}

func (c *DeployCommand) Run(ctx *cmd.Context) error {
	deploy, err := findDeployerFIFO(
		c.maybeReadLocalBundle,
		c.maybeReadLocalCharm,
		c.maybeReadCharmstoreBundle,
		c.charmStoreCharm, // This always returns a deployer
	)
	if err != nil {
		return errors.Trace(err)
	}

	apiClient, err := c.NewAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer apiClient.Close()

	return block.ProcessBlockedError(deploy(ctx, apiClient, &applicationDeployer{ctx, c}), block.BlockChange)
}

func (c *DeployCommand) newResolver() (*config.Config, *csclient.Client, *charmURLResolver, error) {
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}

	modelConfigClient := modelconfig.NewClient(api)
	defer modelConfigClient.Close()

	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	csClient := newCharmStoreClient(bakeryClient).WithChannel(c.Channel)

	conf, err := getModelConfig(modelConfigClient)
	if err != nil {
		return nil, nil, nil, err
	}

	return conf, csClient, newCharmURLResolver(conf, csClient), nil
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

type deployFn func(*cmd.Context, *api.Client, *applicationDeployer) error

func (c *DeployCommand) validateBundleFlags() error {
	if flags := getFlags(c.flagSet, charmOnlyFlags); len(flags) > 0 {
		return errors.Errorf("Flags provided but not supported when deploying a bundle: %s.", strings.Join(flags, ", "))
	}
	return nil
}

func (c *DeployCommand) validateCharmFlags() error {
	if flags := getFlags(c.flagSet, bundleOnlyFlags); len(flags) > 0 {
		return errors.Errorf("Flags provided but not supported when deploying a charm: %s.", strings.Join(flags, ", "))
	}
	return nil
}

func (c *DeployCommand) maybeReadLocalBundle() (deployFn, error) {
	bundleFile := c.CharmOrBundle
	var (
		bundleFilePath                string
		resolveRelativeBundleFilePath bool
	)

	bundleData, err := charmrepo.ReadBundleFile(bundleFile)
	if err != nil {
		// We may have been given a local bundle archive or exploded directory.
		bundle, url, pathErr := charmrepo.NewBundleAtPath(bundleFile)
		if pathErr != nil {
			// If the bundle files existed but we couldn't read them,
			// then return that error rather than trying to interpret
			// as a charm.
			if info, statErr := os.Stat(c.CharmOrBundle); statErr == nil {
				if info.IsDir() {
					if _, ok := pathErr.(*charmrepo.NotFoundError); !ok {
						return nil, pathErr
					}
				}
			}

			return nil, nil
		}

		bundleData = bundle.Data()
		bundleFile = url.String()
		if info, err := os.Stat(bundleFile); err == nil && info.IsDir() {
			bundleFilePath = bundleFile
		}
	} else {
		resolveRelativeBundleFilePath = true
	}

	if err := c.validateBundleFlags(); err != nil {
		return nil, errors.Trace(err)
	}

	return func(ctx *cmd.Context, apiClient *api.Client, deployer *applicationDeployer) error {
		// For local bundles, we extract the local path of the bundle
		// directory.
		if resolveRelativeBundleFilePath {
			bundleFilePath = filepath.Dir(ctx.AbsPath(bundleFile))
		}

		_, _, resolver, err := c.newResolver()
		if err != nil {
			return errors.Trace(err)
		}

		return errors.Trace(c.deployBundle(
			ctx,
			bundleFile,
			bundleFilePath,
			bundleData,
			c.Channel,
			apiClient,
			deployer,
			resolver,
			c.BundleStorage,
		))
	}, nil
}

func (c *DeployCommand) maybeReadLocalCharm() (deployFn, error) {
	// Charm may have been supplied via a path reference.
	ch, curl, err := charmrepo.NewCharmAtPathForceSeries(c.CharmOrBundle, c.Series, c.Force)
	// We check for several types of known error which indicate
	// that the supplied reference was indeed a path but there was
	// an issue reading the charm located there.
	if charm.IsMissingSeriesError(err) {
		return nil, err
	} else if charm.IsUnsupportedSeriesError(err) {
		return nil, errors.Errorf("%v. Use --force to deploy the charm anyway.", err)
	} else if errors.Cause(err) == zip.ErrFormat {
		return nil, errors.Errorf("invalid charm or bundle provided at %q", c.CharmOrBundle)
	} else if _, ok := err.(*charmrepo.NotFoundError); ok {
		return nil, errors.Errorf("no charm or bundle found at %q", c.CharmOrBundle)
	} else if err != nil && err != os.ErrNotExist {
		// If we get a "not exists" error then we attempt to interpret
		// the supplied charm reference as a URL elsewhere, otherwise
		// we return the error.
		return nil, err
	} else if err != nil {
		return nil, nil
	}

	return func(ctx *cmd.Context, apiClient *api.Client, deployer *applicationDeployer) error {
		if curl, err = apiClient.AddLocalCharm(curl, ch); err != nil {
			return errors.Trace(err)
		}

		id := charmstore.CharmID{
			URL: curl,
			// Local charms don't need a channel.
		}
		var csMac *macaroon.Macaroon // local charms don't need one.
		return errors.Trace(c.deployCharm(deployCharmArgs{
			id:       id,
			csMac:    csMac,
			series:   curl.Series,
			ctx:      ctx,
			client:   apiClient,
			deployer: deployer,
		}))
	}, nil
}

func (c *DeployCommand) maybeReadCharmstoreBundle() (deployFn, error) {
	userRequestedURL, err := charm.ParseURL(c.CharmOrBundle)
	if err != nil {
		return nil, errors.Trace(err)
	}

	_, _, resolver, err := c.newResolver()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Charm or bundle has been supplied as a URL so we resolve and
	// deploy using the store.
	storeCharmOrBundleURL, channel, _, store, err := resolver.resolve(userRequestedURL)
	if charm.IsUnsupportedSeriesError(err) {
		return nil, errors.Errorf("%v. Use --force to deploy the charm anyway.", err)
	} else if err != nil {
		return nil, errors.Trace(err)
	} else if storeCharmOrBundleURL.Series != "bundle" {
		return nil, nil
	}

	if err := c.validateBundleFlags(); err != nil {
		return nil, errors.Trace(err)
	}

	return func(ctx *cmd.Context, apiClient *api.Client, deployer *applicationDeployer) error {
		bundle, err := store.GetBundle(storeCharmOrBundleURL)
		if err != nil {
			return errors.Trace(err)
		}
		data := bundle.Data()
		ident := storeCharmOrBundleURL.String()

		return errors.Trace(c.deployBundle(
			ctx,
			ident,
			"", // filepath
			data,
			channel,
			apiClient,
			deployer,
			resolver,
			c.BundleStorage,
		))
	}, nil
}

func (c *DeployCommand) charmStoreCharm() (deployFn, error) {
	userRequestedURL, err := charm.ParseURL(c.CharmOrBundle)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// resolver.resolve potentially updates the series of anything
	// passed in. Store this for use in seriesSelector.
	userRequestedSeries := userRequestedURL.Series

	modelCfg, csClient, resolver, err := c.newResolver()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Charm or bundle has been supplied as a URL so we resolve and deploy using the store.
	storeCharmOrBundleURL, channel, supportedSeries, _, err := resolver.resolve(userRequestedURL)
	if charm.IsUnsupportedSeriesError(err) {
		return nil, errors.Errorf("%v. Use --force to deploy the charm anyway.", err)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	if err := c.validateCharmFlags(); err != nil {
		return nil, errors.Trace(err)
	}

	return func(ctx *cmd.Context, apiClient *api.Client, deployer *applicationDeployer) error {
		selector := seriesSelector{
			charmURLSeries:  userRequestedSeries,
			seriesFlag:      c.Series,
			supportedSeries: supportedSeries,
			force:           c.Force,
			conf:            modelCfg,
			fromBundle:      false,
		}

		// Get the series to use.
		series, message, err := selector.charmSeries()
		if charm.IsUnsupportedSeriesError(err) {
			return errors.Errorf("%v. Use --force to deploy the charm anyway.", err)
		}

		// Store the charm in the controller
		curl, csMac, err := addCharmFromURL(apiClient, storeCharmOrBundleURL, channel, csClient)
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
			Channel: channel,
		}
		return c.deployCharm(deployCharmArgs{
			id:       id,
			csMac:    csMac,
			series:   series,
			ctx:      ctx,
			client:   apiClient,
			deployer: deployer,
		})
	}, nil
}

type metricCredentialsAPI interface {
	SetMetricCredentials(string, []byte) error
	Close() error
}

type metricsCredentialsAPIImpl struct {
	api   *application.Client
	state api.Connection
}

// SetMetricCredentials sets the credentials on the application.
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
	return &metricsCredentialsAPIImpl{api: application.NewClient(state), state: state}, nil
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
