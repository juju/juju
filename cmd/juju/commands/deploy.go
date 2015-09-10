// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"
	"strings"

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

type DeployCommand struct {
	envcmd.EnvCommandBase
	service.UnitCommandBase
	CharmName    string
	ServiceName  string
	Config       cmd.FileVar
	Constraints  constraints.Value
	Networks     string
	BumpRevision bool   // Remove this once the 1.16 support is dropped.
	RepoPath     string // defaults to JUJU_REPOSITORY
	RegisterURL  string

	// TODO(axw) move this to UnitCommandBase once we support --storage
	// on add-unit too.
	//
	// Storage is a map of storage constraints, keyed on the storage name
	// defined in charm storage metadata.
	Storage map[string]storage.Constraints
}

const deployDoc = `
<charm name> can be a charm URL, or an unambiguously condensed form of it;
assuming a current series of "precise", the following forms will be accepted:

For cs:precise/mysql
  mysql
  precise/mysql

For cs:~user/precise/mysql
  cs:~user/mysql

The current series is determined first by the default-series environment
setting, followed by the preferred series for the charm in the charm store.

In these cases, a versioned charm URL will be expanded as expected (for example,
mysql-33 becomes cs:precise/mysql-33).

However, for local charms, when the default-series is not specified in the
environment, one must specify the series. For example:
  local:precise/mysql

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

   juju deploy mysql --networks=storage,mynet --constraints networks=^logging,db
   (deploy mysql on machines with "storage", "mynet" and "db" networks,
    but not on machines with "logging" network, also configure "storage" and
    "mynet" networks)

Like constraints, service-specific network requirements can be
specified with the --networks argument, which takes a comma-delimited
list of juju-specific network names. Networks can also be specified with
constraints, but they only define what machine to pick, not what networks
to configure on it. The --networks argument instructs juju to add all the
networks specified with it to all new machines deployed to host units of
the service. Not supported on all providers.

See Also:
   juju help constraints
   juju help set-constraints
   juju help get-constraints
`

func (c *DeployCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "deploy",
		Args:    "<charm name> [<service name>]",
		Purpose: "deploy a new service",
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
	f.StringVar(&c.Networks, "networks", "", "bind the service to specific networks")
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
		if _, err := charm.ParseReference(args[0]); err != nil {
			return fmt.Errorf("invalid charm name %q", args[0])
		}
		c.CharmName = args[0]
	case 0:
		return errors.New("no charm specified")
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

	csClient, err := newCharmStoreClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer csClient.jar.Save()

	repoPath := ctx.AbsPath(c.RepoPath)
	url, repo, err := resolveEntityURL(c.CharmName, csClient.params, repoPath, conf)
	if err != nil {
		return errors.Trace(err)
	}

	if url.Series == "bundle" {
		// Deploy a bundle entity.
		bundle, err := repo.GetBundle(url)
		if err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
		if err := deployBundle(bundle.Data(), client, csClient, repoPath, conf, ctx); err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
		ctx.Infof("deployment of bundle %q completed", url)
		return nil
	}

	url, err = addCharmViaAPI(client, url, repo, csClient)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	ctx.Infof("Added charm %q to the environment.", url)

	if c.BumpRevision {
		ctx.Infof("--upgrade (or -u) is deprecated and ignored; charms are always deployed with a unique revision.")
	}

	requestedNetworks, err := networkNamesToTags(parseNetworks(c.Networks))
	if err != nil {
		return err
	}
	// We need to ensure network names are valid below, but we don't need them here.
	_, err = networkNamesToTags(c.Constraints.IncludeNetworks())
	if err != nil {
		return err
	}
	_, err = networkNamesToTags(c.Constraints.ExcludeNetworks())
	if err != nil {
		return err
	}
	haveNetworks := len(requestedNetworks) > 0 || c.Constraints.HaveNetworks()

	charmInfo, err := client.CharmInfo(url.String())
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
			url.String(),
			serviceName,
			numUnits,
			string(configYAML),
			c.Constraints,
			c.PlacementSpec,
			c.Placement,
			requestedNetworks,
			c.Storage,
		)
		if params.IsCodeNotImplemented(err) {
			return notSupported
		}
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	err = client.ServiceDeployWithNetworks(
		url.String(),
		serviceName,
		numUnits,
		string(configYAML),
		c.Constraints,
		c.PlacementSpec,
		requestedNetworks,
	)
	if params.IsCodeNotImplemented(err) {
		if haveNetworks {
			return errors.New("cannot use --networks/--constraints networks=...: not supported by the API server")
		}
		err = client.ServiceDeploy(
			url.String(),
			serviceName,
			numUnits,
			string(configYAML),
			c.Constraints,
			c.PlacementSpec)
	}

	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	state, err := c.NewAPIRoot()
	if err != nil {
		return err
	}
	err = registerMeteredCharm(c.RegisterURL, state, csClient.jar, url.String(), serviceName, client.EnvironmentUUID())
	if params.IsCodeNotImplemented(err) {
		// The state server is too old to support metering.  Warn
		// the user, but don't return an error.
		logger.Warningf("current state server version does not support charm metering")
		return nil
	}

	return block.ProcessBlockedError(err, block.BlockChange)
}

// parseNetworks returns a list of network names by parsing the
// comma-delimited string value of --networks argument.
func parseNetworks(networksValue string) []string {
	parts := strings.Split(networksValue, ",")
	var networks []string
	for _, part := range parts {
		network := strings.TrimSpace(part)
		if network != "" {
			networks = append(networks, network)
		}
	}
	return networks
}

// networkNamesToTags returns the given network names converted to
// tags, or an error.
func networkNamesToTags(networks []string) ([]string, error) {
	var tags []string
	for _, network := range networks {
		if !names.IsValidNetwork(network) {
			return nil, fmt.Errorf("%q is not a valid network name", network)
		}
		tags = append(tags, names.NewNetworkTag(network).String())
	}
	return tags, nil
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
