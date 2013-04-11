package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"os"
)

type DeployCommand struct {
	EnvCommandBase
	CharmName    string
	ServiceName  string
	Config       cmd.FileVar
	Constraints  constraints.Value
	NumUnits     int // defaults to 1
	BumpRevision bool
	RepoPath     string // defaults to JUJU_REPOSITORY
	MachineId    string
}

const deployDoc = `
<charm name> can be a charm URL, or an unambiguously condensed form of it;
assuming a current default series of "precise", the following forms will be
accepted.

For cs:precise/mysql
  mysql
  precise/mysql

For cs:~user/precise/mysql
  cs:~user/mysql

For local:precise/mysql
  local:mysql

In all cases, a versioned charm URL will be expanded as expected (for example,
mysql-33 becomes cs:precise/mysql-33).

<service name>, if omitted, will be derived from <charm name>.
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
	c.EnvCommandBase.SetFlags(f)
	f.IntVar(&c.NumUnits, "n", 1, "number of service units to deploy for principal charms")
	f.IntVar(&c.NumUnits, "num-units", 1, "")
	f.StringVar(&c.MachineId, "force-machine", "", "Machine to deploy initial unit, bypasses constraints")
	f.BoolVar(&c.BumpRevision, "u", false, "increment local charm directory revision")
	f.BoolVar(&c.BumpRevision, "upgrade", false, "")
	f.Var(&c.Config, "config", "path to yaml-formatted service config")
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "set service constraints")
	f.StringVar(&c.RepoPath, "repository", os.Getenv("JUJU_REPOSITORY"), "local charm repository")
}

func (c *DeployCommand) Init(args []string) error {
	// TODO --constraints
	switch len(args) {
	case 2:
		if !state.IsServiceName(args[1]) {
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
	if c.NumUnits < 1 {
		// TODO improve/remove: this is misleading when deploying subordinates.
		return errors.New("must deploy at least one unit")
	}

	if c.MachineId != "" {
		if !state.IsMachineId(c.MachineId) {
			return fmt.Errorf("Invalid machine id %q", c.MachineId)
		}
	}
	return nil
}

func (c *DeployCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	conf, err := conn.State.EnvironConfig()
	if err != nil {
		return err
	}
	curl, err := charm.InferURL(c.CharmName, conf.DefaultSeries())
	if err != nil {
		return err
	}
	repo, err := charm.InferRepository(curl, ctx.AbsPath(c.RepoPath))
	if err != nil {
		return err
	}
	var configYAML []byte
	if c.Config.Path != "" {
		configYAML, err = ioutil.ReadFile(c.Config.Path)
		if err != nil {
			return err
		}
	}
	charm, err := conn.PutCharm(curl, repo, c.BumpRevision)
	if err != nil {
		return err
	}
	if charm.Meta().Subordinate {
		empty := constraints.Value{}
		if c.Constraints != empty {
			return state.ErrSubordinateConstraints
		}
	}
	serviceName := c.ServiceName
	if serviceName == "" {
		serviceName = curl.Name
	}
	args := juju.DeployServiceParams{
		Charm:       charm,
		ServiceName: serviceName,
		NumUnits:    c.NumUnits,
		// BUG(lp:1162122): --config has no tests.
		ConfigYAML:  string(configYAML),
		Constraints: c.Constraints,
		ForceMachineId:   c.MachineId,
	}
	_, err = conn.DeployService(args)
	return err
}
