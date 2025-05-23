// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/retry"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/caasapplication"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/constants"
	"github.com/juju/juju/cmd/containeragent/utils"
	"github.com/juju/juju/internal/cmd"
	internallogger "github.com/juju/juju/internal/logger"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	pebbleidentity "github.com/juju/juju/internal/service/pebble/identity"
	pebbleplan "github.com/juju/juju/internal/service/pebble/plan"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/internal/worker/introspection"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/application_mock.go github.com/juju/juju/cmd/containeragent/initialize ApplicationAPI
type initCommand struct {
	cmd.CommandBase

	config           configFunc
	identity         identityFunc
	applicationAPI   ApplicationAPI
	fileReaderWriter utils.FileReaderWriter
	environment      utils.Environment
	clock            clock.Clock

	// charmModifiedVersion holds just that and is used for generating the
	// pebble service to run the container agent.
	charmModifiedVersion string

	// containerAgentPebbleDir holds the path to the pebble config dir used on
	// the container agent.
	containerAgentPebbleDir string

	// pebbleIdentitiesFile holds the path to the pebble identities for the
	// workload sidecar containers so that the charm can connect to them.
	pebbleIdentitiesFile string

	// pebbleCharmIdentity holds the user id for the charm identity.
	pebbleCharmIdentity int

	dataDir      string
	binDir       string
	profileDir   string
	isController bool
}

// ApplicationAPI provides methods for unit introduction.
type ApplicationAPI interface {
	UnitIntroduction(ctx context.Context, podName string, podUUID string) (*caasapplication.UnitConfig, error)
	Close() error
}

// New creates containeragent init command.
func New() cmd.Command {
	return &initCommand{
		config:           defaultConfig,
		identity:         identityFromK8sMetadata,
		fileReaderWriter: utils.NewFileReaderWriter(),
		environment:      utils.NewEnvironment(),
		clock:            clock.WallClock,
	}
}

// SetFlags implements Command.
func (c *initCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.containerAgentPebbleDir, "containeragent-pebble-dir", "", "directory for container agent pebble config")
	f.StringVar(&c.charmModifiedVersion, "charm-modified-version", "", "charm modified version for update hook")
	f.StringVar(&c.dataDir, "data-dir", "", "directory for juju data")
	f.StringVar(&c.binDir, "bin-dir", "", "copy juju binaries to this directory")
	f.StringVar(&c.profileDir, "profile-dir", "", "install introspection functions to this directory")
	f.StringVar(&c.pebbleIdentitiesFile, "pebble-identities-file", "", "pebble identities file for configuring workload pebbles to auth with the charm")
	f.IntVar(&c.pebbleCharmIdentity, "pebble-charm-identity", -1, "charm identity user-id to add to the --pebble-identities-file")
	f.BoolVar(&c.isController, "controller", false, "set when the charm is colocated with the controller")
}

// Info returns a description of the command.
func (c *initCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "init",
		Purpose: "Initialize containeragent local state.",
	})
}

func (c *initCommand) getApplicationAPI(ctx context.Context) (ApplicationAPI, error) {
	if c.applicationAPI == nil {
		connection, err := apicaller.OnlyConnect(ctx, c, api.Open, internallogger.GetLogger("juju.containeragent"))
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

func (c *initCommand) Run(ctx *cmd.Context) (err error) {
	ctx.Infof("starting containeragent init command")

	defer func() {
		if err == nil {
			err = c.writeContainerAgentPebbleConfig()
		}
		if err == nil {
			err = c.copyBinaries()
		}
		if err == nil {
			err = c.installIntrospectionFunctions()
		}
	}()

	// If the agent conf already exists, no need to do the unit introduction.
	// TODO(wallyworld) - we may need to revisit this when we support stateless workloads.
	templateConfigPath := path.Join(c.dataDir, k8sconstants.TemplateFileNameAgentConf)
	_, err = c.fileReaderWriter.Stat(templateConfigPath)
	if err == nil || !os.IsNotExist(err) {
		return errors.Trace(err)
	}

	applicationAPI, err := c.getApplicationAPI(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = applicationAPI.Close() }()

	identity, err := c.identity()
	if err != nil {
		return errors.Trace(err)
	}

	var unitConfig *caasapplication.UnitConfig
	err = retry.Call(retry.CallArgs{
		Func: func() error {
			unitConfig, err = applicationAPI.UnitIntroduction(ctx, identity.PodName, identity.PodUUID)
			return errors.Trace(err)
		},
		IsFatalError: func(err error) bool {
			return !errors.Is(err, errors.NotAssigned) && !errors.Is(err, errors.AlreadyExists)
		},
		Attempts: -1,
		Delay:    10 * time.Second,
		MaxDelay: 30 * time.Second,
		NotifyFunc: func(lastError error, attempt int) {
			ctx.Infof("failed to introduce pod %s: %v...", identity.PodName, lastError)
		},
		Clock: c.clock,
	})
	if err != nil {
		return errors.Trace(err)
	}

	if err = c.fileReaderWriter.MkdirAll(c.dataDir, 0775); err != nil {
		return errors.Trace(err)
	}

	if err = c.fileReaderWriter.WriteFile(templateConfigPath, unitConfig.AgentConf, 0664); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *initCommand) copyBinaries() error {
	if err := c.fileReaderWriter.MkdirAll(c.binDir, 0775); err != nil {
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
			dstStream, err := c.fileReaderWriter.Writer(dst, 0775)
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

	doCopy("/opt/pebble", path.Join(c.binDir, "pebble"))
	doCopy("/opt/containeragent", path.Join(c.binDir, "containeragent"))
	doCopy("/opt/jujuc", path.Join(c.binDir, "jujuc"))
	return eg.Wait()
}

// writeContainerAgentPebbleConfig is responsible for generating the container agent
// pebble service configuration.
func (c *initCommand) writeContainerAgentPebbleConfig() error {
	extraArgs := []string{}
	// If we actually have the charmModifiedVersion let's add it the args.
	if c.charmModifiedVersion != "" {
		extraArgs = append(extraArgs, "--charm-modified-version", c.charmModifiedVersion)
	}

	if c.isController {
		extraArgs = append(extraArgs, "--controller")
	}

	onCheckFailureAction := pebbleplan.ActionShutdown
	if c.isController {
		onCheckFailureAction = pebbleplan.ActionRestart
	}

	containerAgentLayer := pebbleplan.Layer{
		Summary: "Juju container agent service",
		Services: map[string]*pebbleplan.Service{
			"container-agent": {
				Summary:  "Juju container agent",
				Override: pebbleplan.ReplaceOverride,
				Command: fmt.Sprintf("%s unit --data-dir %s --append-env \"PATH=$PATH:%s\" --show-log %s",
					path.Join(c.binDir, "containeragent"),
					c.dataDir,
					c.binDir,
					strings.Join(extraArgs, " ")),
				KillDelay: pebbleplan.OptionalDuration{Value: 30 * time.Minute, IsSet: true},
				Startup:   pebbleplan.StartupEnabled,
				OnSuccess: pebbleplan.ActionIgnore,
				OnFailure: onCheckFailureAction,
				OnCheckFailure: map[string]pebbleplan.ServiceAction{
					"liveness":  pebbleplan.ActionIgnore,
					"readiness": pebbleplan.ActionIgnore,
				},
				Environment: map[string]string{
					constants.EnvHTTPProbePort: constants.DefaultHTTPProbePort,
				},
			},
		},
		Checks: map[string]*pebbleplan.Check{
			"readiness": {
				Override:  pebbleplan.ReplaceOverride,
				Level:     pebbleplan.ReadyLevel,
				Period:    pebbleplan.OptionalDuration{Value: 10 * time.Second, IsSet: true},
				Timeout:   pebbleplan.OptionalDuration{Value: 3 * time.Second, IsSet: true},
				Threshold: 3,
				HTTP: &pebbleplan.HTTPCheck{
					URL: fmt.Sprintf("http://localhost:%s/readiness", constants.DefaultHTTPProbePort),
				},
			},
			"liveness": {
				Override:  pebbleplan.ReplaceOverride,
				Level:     pebbleplan.AliveLevel,
				Period:    pebbleplan.OptionalDuration{Value: 10 * time.Second, IsSet: true},
				Timeout:   pebbleplan.OptionalDuration{Value: 3 * time.Second, IsSet: true},
				Threshold: 3,
				HTTP: &pebbleplan.HTTPCheck{
					URL: fmt.Sprintf("http://localhost:%s/liveness", constants.DefaultHTTPProbePort),
				},
			},
		},
	}

	layerDir := path.Join(c.containerAgentPebbleDir, "layers")
	if err := c.fileReaderWriter.MkdirAll(layerDir, 0775); err != nil {
		return fmt.Errorf("making pebble container agent layer dir at %q: %w", layerDir, err)
	}

	p := path.Join(layerDir, "001-container-agent.yaml")

	rawConfig, err := yaml.Marshal(&containerAgentLayer)
	if err != nil {
		return fmt.Errorf("making pebble container agent layer yaml: %w", err)
	}

	if err := c.fileReaderWriter.WriteFile(p, rawConfig, 0664); err != nil {
		return fmt.Errorf("writing container agent pebble configuration to %q: %w", p, err)
	}

	if c.pebbleIdentitiesFile != "" {
		idFile := pebbleidentity.IdentitiesFile{}
		if c.pebbleCharmIdentity >= 0 {
			uid := uint32(c.pebbleCharmIdentity)
			idFile.Identities = map[string]*pebbleidentity.Identity{
				"charm": {
					Access: pebbleidentity.AdminAccess,
					Local: &pebbleidentity.LocalIdentity{
						UserID: &uid,
					},
				},
			}
		}
		rawIDFile, err := yaml.Marshal(&idFile)
		if err != nil {
			return fmt.Errorf("cannot format pebble identities file: %w", err)
		}
		if err := c.fileReaderWriter.WriteFile(c.pebbleIdentitiesFile, rawIDFile, 0664); err != nil {
			return fmt.Errorf("writing pebble identities file to %q: %w", c.pebbleIdentitiesFile, err)
		}
	}

	return nil
}

func (c *initCommand) installIntrospectionFunctions() error {
	if c.profileDir == "" {
		return nil
	}
	return introspection.UpdateProfileFunctions(c.fileReaderWriter, c.profileDir)
}

func (c *initCommand) CurrentConfig() agent.Config {
	return c.config()
}

func (c *initCommand) ChangeConfig(agent.ConfigMutator) error {
	return errors.NotSupportedf("cannot change config")
}
