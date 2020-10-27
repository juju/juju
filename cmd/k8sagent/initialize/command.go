// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize

import (
	"context"
	"io"
	"path"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"golang.org/x/sync/errgroup"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/caasapplication"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/k8sagent/utils"
	"github.com/juju/juju/worker/apicaller"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/application_mock.go github.com/juju/juju/cmd/k8sagent/initialize ApplicationAPI
type initCommand struct {
	cmd.CommandBase

	config           configFunc
	identity         identityFunc
	applicationAPI   ApplicationAPI
	fileReaderWriter utils.FileReaderWriter

	dataDir string
	binDir  string
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

// SetFlags implements Command.
func (c *initCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.dataDir, "data-dir", "", "directory for juju data")
	f.StringVar(&c.binDir, "bin-dir", "", "copy juju binaries to this directory")
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

func (c *initCommand) Init(args []string) error {
	if c.dataDir == "" {
		return errors.NotValidf("--data-dir")
	}
	if c.binDir == "" {
		return errors.NotValidf("--bin-dir")
	}
	return c.CommandBase.Init(args)
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

	if err = c.fileReaderWriter.MkdirAll(c.dataDir, 0755); err != nil {
		return errors.Trace(err)
	}

	templateConfigPath := path.Join(c.dataDir, k8sconstants.TemplateFileNameAgentConf)
	if err = c.fileReaderWriter.WriteFile(templateConfigPath, unitConfig.AgentConf, 0644); err != nil {
		return errors.Trace(err)
	}

	if err = c.fileReaderWriter.MkdirAll(c.binDir, 0755); err != nil {
		return errors.Trace(err)
	}

	eg, _ := errgroup.WithContext(context.Background())
	doCopy := func(src string, dst string) {
		eg.Go(func() error {
			srcStream, err := c.fileReaderWriter.Reader(src)
			if err != nil {
				return errors.Annotatef(err, "opening %q for reading", src)
			}
			defer srcStream.Close()
			dstStream, err := c.fileReaderWriter.Writer(dst, 0755)
			if err != nil {
				return errors.Annotatef(err, "opening %q for writing", dst)
			}
			defer dstStream.Close()
			_, err = io.Copy(dstStream, srcStream)
			if err == io.EOF {
				return nil
			} else if err != nil {
				return errors.Annotatef(err, "copying %q to %q", src, dst)
			}
			ctx.Infof("copied %q to %q", src, dst)
			return nil
		})
	}
	doCopy("/opt/pebble", path.Join(c.binDir, "pebble"))
	doCopy("/opt/k8sagent", path.Join(c.binDir, "k8sagent"))
	doCopy("/opt/jujuc", path.Join(c.binDir, "jujuc"))
	return eg.Wait()
}

func (c *initCommand) CurrentConfig() agent.Config {
	return c.config()
}

func (c *initCommand) ChangeConfig(agent.ConfigMutator) error {
	return errors.NotSupportedf("cannot change config")
}
