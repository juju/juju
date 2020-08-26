// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/caasapplication"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	jujucmd "github.com/juju/juju/cmd"
	corepaths "github.com/juju/juju/core/paths"
)

type InitCommand struct {
	cmd.CommandBase
	Connect  ConnectFunc
	Config   ConfigFunc
	Identity IdentityFunc
}

func New() cmd.Command {
	return &InitCommand{
		Connect:  DefaultConnect,
		Config:   DefaultConfig,
		Identity: DefaultIdentity,
	}
}

// Info returns a description of the command.
func (c *InitCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "init",
		Purpose: "initialize k8sagent state",
	})
}

func (c *InitCommand) Run(ctx *cmd.Context) error {
	ctx.Infof("starting k8sagent init command")

	identity := c.Identity()
	connection, err := c.Connect(c)
	if err != nil {
		return errors.Trace(err)
	}

	client := caasapplication.NewClient(connection)
	unitConfig, err := client.UnitIntroduction(identity.PodName, identity.PodUUID)
	if err != nil {
		return errors.Trace(err)
	}

	dataDir, _ := corepaths.DataDir("kubernetes")
	err = os.MkdirAll(dataDir, 0755)
	if err != nil {
		return errors.Trace(err)
	}

	templateConfigPath := path.Join(dataDir, k8sconstants.TemplateFileNameAgentConf)
	err = ioutil.WriteFile(templateConfigPath, unitConfig.AgentConf, 0644)
	if err != nil {
		return errors.Trace(err)
	}

	pebbleBytes, err := ioutil.ReadFile("/opt/pebble")
	if err != nil {
		return errors.Trace(err)
	}
	err = ioutil.WriteFile("/shared/usr/bin/pebble", pebbleBytes, 0755)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *InitCommand) CurrentConfig() agent.Config {
	return c.Config()
}

func (c *InitCommand) ChangeConfig(agent.ConfigMutator) error {
	return errors.NotSupportedf("cannot change config")
}
