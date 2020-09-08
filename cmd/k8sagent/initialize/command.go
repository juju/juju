// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize

import (
	"path"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/caasapplication"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/k8sagent/utils"
	corepaths "github.com/juju/juju/core/paths"
	"github.com/juju/juju/worker/apicaller"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/application_mock.go github.com/juju/juju/cmd/k8sagent/initialize ApplicationAPI
type initCommand struct {
	cmd.CommandBase

	config           configFunc
	identity         identityFunc
	applicationAPI   ApplicationAPI
	fileReaderWriter utils.FileReaderWriter
}

// ApplicationAPI provides methods for unit introduction.
type ApplicationAPI interface {
	UnitIntroduction(podName string, podUUID string) (*caasapplication.UnitConfig, error)
	Close() error
}

// New creates k8sagent init command.
func New() cmd.Command {
	return &initCommand{
		config:           defaultConfig,
		identity:         defaultIdentity,
		fileReaderWriter: utils.NewFileReaderWriter(),
	}
}

// Info returns a description of the command.
func (c *initCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "init",
		Purpose: "initialize k8sagent state",
	})
}

func (c *initCommand) getApplicationAPI() (ApplicationAPI, error) {
	if c.applicationAPI == nil {
		connection, err := apicaller.OnlyConnect(c, api.Open, loggo.GetLogger("juju.k8sagent"))
		if err != nil {
			return nil, errors.Trace(err)
		}
		c.applicationAPI = caasapplication.NewClient(connection)
	}
	return c.applicationAPI, nil
}

func (c *initCommand) Run(ctx *cmd.Context) error {
	ctx.Infof("starting k8sagent init command")

	applicationAPI, err := c.getApplicationAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = applicationAPI.Close() }()

	identity := c.identity()
	unitConfig, err := applicationAPI.UnitIntroduction(identity.PodName, identity.PodUUID)
	if err != nil {
		return errors.Trace(err)
	}

	dataDir, _ := corepaths.DataDir("kubernetes")
	if err = c.fileReaderWriter.MkdirAll(dataDir, 0755); err != nil {
		return errors.Trace(err)
	}

	templateConfigPath := path.Join(dataDir, k8sconstants.TemplateFileNameAgentConf)
	if err = c.fileReaderWriter.WriteFile(templateConfigPath, unitConfig.AgentConf, 0644); err != nil {
		return errors.Trace(err)
	}

	pebbleBytes, err := c.fileReaderWriter.ReadFile("/opt/pebble")
	if err != nil {
		return errors.Trace(err)
	}
	err = c.fileReaderWriter.WriteFile("/shared/usr/bin/pebble", pebbleBytes, 0755)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *initCommand) CurrentConfig() agent.Config {
	return c.config()
}

func (c *initCommand) ChangeConfig(agent.ConfigMutator) error {
	return errors.NotSupportedf("cannot change config")
}
