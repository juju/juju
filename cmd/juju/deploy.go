// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
)

type DeployCommand struct {
	envcmd.EnvCommandBase
	UnitCommandBase
	CharmName    string
	ServiceName  string
	Config       cmd.FileVar
	Constraints  constraints.Value
	Networks     string
	BumpRevision bool   // Remove this once the 1.16 support is dropped.
	RepoPath     string // defaults to JUJU_REPOSITORY
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

   juju deploy mysql -n 5 --constraints mem=8G (deploy 5 instances of mysql with at least 8 GB of RAM each)

   juju deploy mysql --networks=storage,mynet

Like constraints, service-specific network requirements can be
specified with the --networks argument, which takes a comma-delimited
list of provider-specific network names/labels.
These instruct juju to ensure to add all the networks specified with
--networks to all new machines deployed to host units of the service.
Not supported on all providers.

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
	f.StringVar(&c.Networks, "networks", "", "enable networks for service")
	f.StringVar(&c.RepoPath, "repository", os.Getenv(osenv.JujuRepositoryEnvKey), "local charm repository")
}

func (c *DeployCommand) Init(args []string) error {
	switch len(args) {
	case 2:
		if !names.IsService(args[1]) {
			return fmt.Errorf("invalid service name %q", args[1])
		}
		c.ServiceName = args[1]
		fallthrough
	case 1:
		if _, err := charm.InferURL(args[0], "fake"); err != nil {
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

func (c *DeployCommand) Run(ctx *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()

	attrs, err := client.EnvironmentGet()
	if err != nil {
		return err
	}
	conf, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return err
	}

	curl, err := resolveCharmURL(c.CharmName, client, conf)
	if err != nil {
		return err
	}

	repo, err := charm.InferRepository(curl.Reference, ctx.AbsPath(c.RepoPath))
	if err != nil {
		return err
	}

	repo = config.SpecializeCharmRepo(repo, conf)

	curl, err = addCharmViaAPI(client, ctx, curl, repo)
	if err != nil {
		return err
	}

	if c.BumpRevision {
		ctx.Infof("--upgrade (or -u) is deprecated and ignored; charms are always deployed with a unique revision.")
	}
	var includeNetworks []string
	if c.Networks != "" {
		includeNetworks = parseNetworks(c.Networks)

		env, err := environs.New(conf)
		if err != nil {
			return err
		}
		if !env.SupportNetworks() {
			return errors.New("cannot use --networks: not supported by the environment")
		}
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
		if numUnits == 1 && c.ToMachineSpec == "" {
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
	err = client.ServiceDeployWithNetworks(
		curl.String(),
		serviceName,
		numUnits,
		string(configYAML),
		c.Constraints,
		c.ToMachineSpec,
		includeNetworks,
		nil,
	)
	if params.IsCodeNotImplemented(err) {
		if len(includeNetworks) > 0 {
			return errors.New("cannot use --networks: not supported by the API server")
		}
		err = client.ServiceDeploy(
			curl.String(),
			serviceName,
			numUnits,
			string(configYAML),
			c.Constraints,
			c.ToMachineSpec)
	}
	return err
}

// addCharmViaAPI calls the appropriate client API calls to add the
// given charm URL to state. Also displays the charm URL of the added
// charm on stdout.
func addCharmViaAPI(client *api.Client, ctx *cmd.Context, curl *charm.URL, repo charm.Repository) (*charm.URL, error) {
	if curl.Revision < 0 {
		latest, err := charm.Latest(repo, curl)
		if err != nil {
			return nil, err
		}
		curl = curl.WithRevision(latest)
	}
	switch curl.Schema {
	case "local":
		ch, err := repo.Get(curl)
		if err != nil {
			return nil, err
		}
		stateCurl, err := client.AddLocalCharm(curl, ch)
		if err != nil {
			return nil, err
		}
		curl = stateCurl
	case "cs":
		err := client.AddCharm(curl)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported charm URL schema: %q", curl.Schema)
	}
	ctx.Infof("Added charm %q to the environment.", curl)
	return curl, nil
}

// parseNetworks returns a list of network tags by parsing the
// comma-delimited string value of --networks argument.
func parseNetworks(networksValue string) (networks []string) {
	parts := strings.Split(networksValue, ",")
	for _, part := range parts {
		network := strings.TrimSpace(part)
		if network != "" {
			networks = append(networks, names.NetworkTag(network))
		}
	}
	return networks
}
