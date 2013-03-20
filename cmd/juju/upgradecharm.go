package main

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
)

// UpgradeCharm is responsible for upgrading a service's charm.
type UpgradeCharmCommand struct {
	EnvCommandBase
	ServiceName string
}

const upgradeCharmDoc = `
<service> needs to be an existing deployed service, whose charm you want to upgrade.
`

func (c *UpgradeCharmCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upgrade-charm",
		Args:    "<service>",
		Purpose: "upgrade a service's charm",
		Doc:     upgradeCharmDoc,
	}
}

func (c *UpgradeCharmCommand) Init(args []string) error {
	switch len(args) {
	case 1:
		if !state.IsServiceName(args[0]) {
			return fmt.Errorf("invalid service name %q", args[0])
		}
		c.ServiceName = args[0]
	case 0:
		return errors.New("no service specified")
	default:
		return cmd.CheckEmpty(args[1:])
	}
	// TODO(dimitern): add the other flags --switch, --force and --revision.
	return nil
}

// Run connects to the specified environment and starts the charm
// upgrade process.
func (c *UpgradeCharmCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	conf, err := conn.State.EnvironConfig()
	if err != nil {
		return err
	}
	curl, err := charm.InferURL(c.ServiceName, conf.DefaultSeries())
	if err != nil {
		return err
	}
	service, err := conn.State.Service(c.ServiceName)
	if err != nil {
		return err
	}
	// TODO(dimitern): this will change once we add the --switch flag.
	repo := charm.Store()
	sch, err := conn.PutCharm(curl, repo, false)
	if err != nil {
		return err
	}
	// TODO(dimitern): get this from the --force flag
	forced := false
	return service.SetCharm(sch, forced)
}
