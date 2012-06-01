package main

import (
	"errors"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"os"
)

type DeployCommand struct {
	EnvName     string
	CharmName   string
	ServiceName string
	ConfPath    string
	NumUnits    int
	Upgrade     bool
	RepoPath    string // defaults to JUJU_REPOSITORY
}

func (c *DeployCommand) Info() *cmd.Info {
	return &cmd.Info{
		"deploy", "<charm-name> [<service-name>]", "deploy a new service", "",
	}
}

func (c *DeployCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	f.IntVar(&c.NumUnits, "n", 1, "number of service units to deploy")
	f.IntVar(&c.NumUnits, "num-units", 1, "")
	f.BoolVar(&c.Upgrade, "u", false, "increment local charm revision")
	f.BoolVar(&c.Upgrade, "upgrade", false, "")
	f.StringVar(&c.ConfPath, "config", "", "path to yaml-formatted service config")
	f.StringVar(&c.RepoPath, "repository", os.Getenv("JUJU_REPOSITORY"), "local charm repository")
	// TODO --constraints
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	switch len(args) {
	case 2:
		c.ServiceName = args[1]
		fallthrough
	case 1:
		c.CharmName = args[0]
	case 0:
		return errors.New("no charm specified")
	default:
		return cmd.CheckEmpty(args[2:])
	}
	if c.NumUnits < 1 {
		return errors.New("must deploy at least one unit")
	}
	return nil
}

func (c *DeployCommand) Run(ctx *cmd.Context) error {
	panic("not implemented")
}
