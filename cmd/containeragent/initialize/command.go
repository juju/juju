// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize

import (
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/caasapplication"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/constants"
	"github.com/juju/juju/cmd/containeragent/utils"
	"github.com/juju/juju/service/pebble/plan"
	"github.com/juju/juju/worker/apicaller"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/application_mock.go github.com/juju/juju/cmd/containeragent/initialize ApplicationAPI
type initCommand struct {
	cmd.CommandBase

	config           configFunc
	identity         identityFunc
	applicationAPI   ApplicationAPI
	fileReaderWriter utils.FileReaderWriter
	environment      utils.Environment

	// charmModifiedVersion holds just that and is used for generating the
	// pebble service to run the container agent.
	charmModifiedVersion string

	// containerAgentPebbleDir holds the path to the pebble config dir used on
	// the container agent.
	containerAgentPebbleDir string

	dataDir string
	binDir  string
}

// ApplicationAPI provides methods for unit introduction.
type ApplicationAPI interface {
	UnitIntroduction(podName string, podUUID string) (*caasapplication.UnitConfig, error)
	Close() error
}

// New creates containeragent init command.
func New() cmd.Command {
	return &initCommand{
		config:           defaultConfig,
		identity:         defaultIdentity,
		fileReaderWriter: utils.NewFileReaderWriter(),
		environment:      utils.NewEnvironment(),
	}
}

// SetFlags implements Command.
func (c *initCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.containerAgentPebbleDir, "containeragent-pebble-dir", "", "directory for container agent pebble config")
	f.StringVar(&c.charmModifiedVersion, "charm-modified-version", "", "charm modified version for update hook")
	f.StringVar(&c.dataDir, "data-dir", "", "directory for juju data")
	f.StringVar(&c.binDir, "bin-dir", "", "copy juju binaries to this directory")
}

// Info returns a description of the command.
func (c *initCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "init",
		Purpose: "Initialize containeragent local state.",
	})
}

func (c *initCommand) getApplicationAPI() (ApplicationAPI, error) {
	if c.applicationAPI == nil {
		connection, err := apicaller.OnlyConnect(c, api.Open, loggo.GetLogger("juju.containeragent"))
		if err != nil {
			return nil, errors.Trace(err)
		}
		c.applicationAPI = caasapplication.NewClient(connection)
	}
	return c.applicationAPI, nil
}

func (c *initCommand) Init(args []string) error {
	if c.containerAgentPebbleDir == "" {
		return errors.NotValidf("--containeragent-pebble-dir")
	}
	if c.dataDir == "" {
		return errors.NotValidf("--data-dir")
	}
	if c.binDir == "" {
		return errors.NotValidf("--bin-dir")
	}
	return c.CommandBase.Init(args)
}

func (c *initCommand) Run(ctx *cmd.Context) error {
	ctx.Infof("starting containeragent init command")

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
			return nil
		})
	}

	if err := c.ContainerAgentPebbleConfig(); err != nil {
		return err
	}

	doCopy("/opt/pebble", path.Join(c.binDir, "pebble"))
	doCopy("/opt/containeragent", path.Join(c.binDir, "containeragent"))
	doCopy("/opt/jujuc", path.Join(c.binDir, "jujuc"))
	return eg.Wait()
}

// ContainerAgentPebbleConfig is responsible for generating the container agent
// pebble service configuration.
func (c *initCommand) ContainerAgentPebbleConfig() error {
	extraArgs := ""
	// If we actually have the charmModifiedVersion let's add it the args.
	if c.charmModifiedVersion != "" {
		extraArgs = "--charm-modified-version " + c.charmModifiedVersion
	}

	containerAgentLayer := plan.Layer{
		Summary: "Juju container agent service",
		Services: map[string]*plan.Service{
			"container-agent": {
				Summary:  "Juju container agent",
				Override: plan.ReplaceOverride,
				Command: fmt.Sprintf("%s unit --data-dir %s --append-env \"PATH=$PATH:%s\" --show-log %s",
					path.Join(c.binDir, "containeragent"),
					c.dataDir,
					c.binDir,
					extraArgs),
				Startup: plan.StartupEnabled,
				OnCheckFailure: map[string]plan.ServiceAction{
					"liveness":  plan.ActionShutdown,
					"readiness": plan.ActionIgnore,
				},
				Environment: map[string]string{
					constants.EnvHTTPProbePort: constants.DefaultHTTPProbePort,
				},
			},
		},
		Checks: map[string]*plan.Check{
			"readiness": {
				Override:  plan.ReplaceOverride,
				Level:     plan.ReadyLevel,
				Period:    plan.OptionalDuration{Value: 10 * time.Second, IsSet: true},
				Timeout:   plan.OptionalDuration{Value: 3 * time.Second, IsSet: true},
				Threshold: 3,
				HTTP: &plan.HTTPCheck{
					URL: fmt.Sprintf("http://localhost:%s/readiness", constants.DefaultHTTPProbePort),
				},
			},
			"liveness": {
				Override:  plan.ReplaceOverride,
				Level:     plan.AliveLevel,
				Period:    plan.OptionalDuration{Value: 10 * time.Second, IsSet: true},
				Timeout:   plan.OptionalDuration{Value: 3 * time.Second, IsSet: true},
				Threshold: 3,
				HTTP: &plan.HTTPCheck{
					URL: fmt.Sprintf("http://localhost:%s/liveness", constants.DefaultHTTPProbePort),
				},
			},
		},
	}

	layerDir := path.Join(c.containerAgentPebbleDir, "layers")
	if err := c.fileReaderWriter.MkdirAll(layerDir, 0555); err != nil {
		return fmt.Errorf("making pebble container agent layer dir at %q: %w", layerDir, err)
	}

	p := path.Join(layerDir, "001-container-agent.yaml")

	rawConfig, err := yaml.Marshal(&containerAgentLayer)
	if err != nil {
		return fmt.Errorf("making pebble container agent layer yaml: %w", err)
	}

	if err := c.fileReaderWriter.WriteFile(p, rawConfig, 0444); err != nil {
		return fmt.Errorf("writing container agent pebble configuration to %q: %w", p, err)
	}
	return nil
}

func (c *initCommand) CurrentConfig() agent.Config {
	return c.config()
}

func (c *initCommand) ChangeConfig(agent.ConfigMutator) error {
	return errors.NotSupportedf("cannot change config")
}
