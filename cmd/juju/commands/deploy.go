// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	apiservice "github.com/juju/juju/api/service"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/service"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/storage"
)

func newDeployCommand() cmd.Command {
	return envcmd.Wrap(&DeployCommand{})
}

type DeployCommand struct {
	envcmd.EnvCommandBase
	service.UnitCommandBase
	CharmOrBundle string
	ServiceName   string
	Config        cmd.FileVar
	Constraints   constraints.Value
	Networks      string // TODO(dimitern): Drop this in a follow-up and fix docs.
	BumpRevision  bool   // Remove this once the 1.16 support is dropped.
	RepoPath      string // defaults to JUJU_REPOSITORY
	RegisterURL   string

	// TODO(axw) move this to UnitCommandBase once we support --storage
	// on add-unit too.
	//
	// Storage is a map of storage constraints, keyed on the storage name
	// defined in charm storage metadata.
	Storage map[string]storage.Constraints
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

The current series for charms is determined first by the default-series
environment setting, followed by the preferred series in the charm store.

In these cases, a versioned charm URL will be expanded as expected (for example,
mysql-33 becomes cs:precise/mysql-33).

However, for local charms, when the default-series is not specified in the
environment, one must specify the series. For example:
  local:precise/mysql

Local bundles can be specified either with a local:bundle/<name> URL, which is
interpreted relative to $JUJU_REPOSITORY, or with a direct path to a
bundle.yaml file. For example, to deploy the bundle in
$JUJU_REPOSITORY/bundle/openstack:

  juju deploy local:bundle/openstack

To deploy this using a direct path:

  juju deploy $JUJU_REPOSITORY/bundle/openstack/bundle.yaml

<service name>, if omitted, will be derived from <charm name>.

Constraints can be specified when using deploy by specifying the --constraints
flag.  When used with deploy, service-specific constraints are set so that later
machines provisioned with add-unit will use the same constraints (unless changed
by set-constraints).

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

func (c *DeployCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "deploy",
		Args:    "<charm or bundle> [<service name>]",
		Purpose: "deploy a new service or bundle",
		Doc:     deployDoc,
	}
}

func (c *DeployCommand) SetFlags(f *gnuflag.FlagSet) {
	c.UnitCommandBase.SetFlags(f)
	f.IntVar(&c.NumUnits, "n", 1, "number of service units to deploy for principal charms")
	f.BoolVar(&c.BumpRevision, "u", false, "increment local charm directory revision (DEPRECATED)")
	f.BoolVar(&c.BumpRevision, "upgrade", false, "")
	f.Var(&c.Config, "config", "path to yaml-formatted service config")
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints}, "constraints", "set service constraints")
	f.StringVar(&c.Networks, "networks", "", "deprecated and ignored: use space constraints instead.")
	f.StringVar(&c.RepoPath, "repository", os.Getenv(osenv.JujuRepositoryEnvKey), "local charm repository")
	f.Var(storageFlag{&c.Storage}, "storage", "charm storage constraints")
}

func (c *DeployCommand) Init(args []string) error {
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
	return c.UnitCommandBase.Init(args)
}

func (c *DeployCommand) newServiceAPIClient() (*apiservice.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiservice.NewClient(root), nil
}

func (c *DeployCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()

	conf, err := service.GetClientConfig(client)
	if err != nil {
		return err
	}

	if err := c.CheckProvider(conf); err != nil {
		return err
	}

	httpClient, err := c.HTTPClient()
	if err != nil {
		return errors.Trace(err)
	}
	csClient := newCharmStoreClient(httpClient)
	repoPath := ctx.AbsPath(c.RepoPath)

	// Handle local bundle paths.
	f, err := os.Open(c.CharmOrBundle)
	if err == nil {
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
		if info.IsDir() {
			return errors.New("deployment of bundle directories not yet supported")
		}
		bundleData, err := charm.ReadBundleData(f)
		if err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
		if err := deployBundle(bundleData, client, csClient, repoPath, conf, ctx); err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
		ctx.Infof("deployment of bundle %q completed", f.Name())
		return nil
	} else if !os.IsNotExist(err) {
		logger.Warningf("cannot open %q: %v; falling back to using charm repository", c.CharmOrBundle, err)
	}

	curl, repo, err := resolveCharmStoreEntityURL(c.CharmOrBundle, csClient.params, repoPath, conf)
	if err != nil {
		return errors.Trace(err)
	}

	// Handle bundle URLs.
	if curl.Series == "bundle" {
		// Deploy a bundle entity.
		bundle, err := repo.GetBundle(curl)
		if err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
		if err := deployBundle(bundle.Data(), client, csClient, repoPath, conf, ctx); err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
		ctx.Infof("deployment of bundle %q completed", curl)
		return nil
	}

	curl, err = addCharmViaAPI(client, curl, repo, csClient)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	ctx.Infof("Added charm %q to the environment.", curl)

	if c.BumpRevision {
		ctx.Infof("--upgrade (or -u) is deprecated and ignored; charms are always deployed with a unique revision.")
	}

	charmInfo, err := client.CharmInfo(curl.String())
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
		configYAML, err = c.Config.Read(ctx)
		if err != nil {
			return err
		}
	}

	// If storage or placement is specified, we attempt to use a new API on the service facade.
	if len(c.Storage) > 0 || len(c.Placement) > 0 {
		notSupported := errors.New("cannot deploy charms with storage or placement: not supported by the API server")
		serviceClient, err := c.newServiceAPIClient()
		if err != nil {
			return notSupported
		}
		defer serviceClient.Close()
		for i, p := range c.Placement {
			if p.Scope == "env-uuid" {
				p.Scope = serviceClient.EnvironmentUUID()
			}
			c.Placement[i] = p
		}
		err = serviceClient.ServiceDeploy(
			curl.String(),
			serviceName,
			numUnits,
			string(configYAML),
			c.Constraints,
			c.PlacementSpec,
			c.Placement,
			[]string{},
			c.Storage,
		)
		if params.IsCodeNotImplemented(err) {
			return notSupported
		}
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	if len(c.Networks) > 0 {
		ctx.Infof("use of --networks is deprecated and is ignored. Please use spaces to manage placement within networks")
	}

	err = client.ServiceDeploy(
		curl.String(),
		serviceName,
		numUnits,
		string(configYAML),
		c.Constraints,
		c.PlacementSpec)

	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	state, err := c.NewAPIRoot()
	if err != nil {
		return err
	}
	err = registerMeteredCharm(c.RegisterURL, state, httpClient, curl.String(), serviceName, client.EnvironmentUUID())
	if params.IsCodeNotImplemented(err) {
		// The state server is too old to support metering.  Warn
		// the user, but don't return an error.
		logger.Warningf("current state server version does not support charm metering")
		return nil
	}

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
	err := s.api.Close()
	if err != nil {
		return errors.Trace(err)
	}
	err = s.state.Close()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

var getMetricCredentialsAPI = func(state api.Connection) (metricCredentialsAPI, error) {
	return &metricsCredentialsAPIImpl{api: apiservice.NewClient(state), state: state}, nil
}
