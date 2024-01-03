// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/client/modelconfig"
	apicontroller "github.com/juju/juju/api/controller/controller"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/environs/tools"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
)

var usageUpgradeJujuSummary = `
Upgrades Juju on all machines in a model.`[1:]

var usageUpgradeJujuDetails = `
Juju provides agent software to every machine it creates. This command
upgrades that software across an entire model, which is, by default, the
current model.
A model's agent version can be shown with `[1:] + "`juju model-config agent-\nversion`" + `.
A version is denoted by: major.minor.patch

If '--agent-version' is not specified, then the upgrade candidate is
selected to be the exact version the controller itself is running.

If the controller is without internet access, the client must first supply
the software to the controller's cache via the ` + "`juju sync-agent-binary`" + ` command.
The command will abort if an upgrade is in progress. It will also abort if
a previous upgrade was not fully completed (e.g.: if one of the
controllers in a high availability model failed to upgrade).

When looking for an agent to upgrade to Juju will check the currently
configured agent stream for that model. It's possible to overwrite this for
the lifetime of this upgrade using --agent-stream

Backups are recommended prior to upgrading.

`

const usageUpgradeJujuExamples = `
    juju upgrade-model --dry-run
    juju upgrade-model --agent-version 2.0.1
    juju upgrade-model --agent-stream proposed
`

func newUpgradeJujuCommand() cmd.Command {
	command := &upgradeJujuCommand{}
	return modelcmd.Wrap(command)
}

// baseUpgradeCommand is used by both the
// upgradeJujuCommand and upgradeControllerCommand
// to hold flags common to both.
type baseUpgradeCommand struct {
	vers        string
	Version     version.Number
	BuildAgent  bool
	DryRun      bool
	AssumeYes   bool
	AgentStream string
	timeout     time.Duration
	// IgnoreAgentVersions is used to allow an admin to request an agent
	// version without waiting for all agents to be at the right version.
	IgnoreAgentVersions bool

	rawArgs        []string
	upgradeMessage string

	modelConfigAPI   ModelConfigAPI
	modelUpgraderAPI ModelUpgraderAPI
	controllerAPI    ControllerAPI
}

func (c *baseUpgradeCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.vers, "agent-version", "", "Upgrade to specific version")
	f.StringVar(&c.AgentStream, "agent-stream", "", "Check this agent stream for upgrades")
	f.BoolVar(&c.BuildAgent, "build-agent", false, "Build a local version of the agent binary; for development use only")
	f.BoolVar(&c.DryRun, "dry-run", false, "Don't change anything, just report what would be changed")
	f.BoolVar(&c.AssumeYes, "y", false, "Answer 'yes' to confirmation prompts")
	f.BoolVar(&c.AssumeYes, "yes", false, "")
	f.BoolVar(&c.IgnoreAgentVersions, "ignore-agent-versions", false,
		"Don't check if all agents have already reached the current version")
	f.DurationVar(&c.timeout, "timeout", 10*time.Minute, "Timeout before upgrade is aborted")
}

func (c *baseUpgradeCommand) Init(args []string) error {
	c.rawArgs = args
	if c.upgradeMessage == "" {
		c.upgradeMessage = "upgrade to this version by running\n    juju upgrade-model"
	}
	if c.vers != "" {
		vers, err := version.Parse(c.vers)
		if err != nil {
			return err
		}
		if c.BuildAgent && vers.Build != 0 {
			// TODO(fwereade): when we start taking versions from actual built
			// code, we should disable --agent-version when used with --build-agent.
			// For now, it's the only way to experiment with version upgrade
			// behaviour live, so the only restriction is that Build cannot
			// be used (because its value needs to be chosen internally so as
			// not to collide with existing tools).
			return errors.New("cannot specify build number when building an agent")
		}
		c.Version = vers
	}
	return cmd.CheckEmpty(args)
}

// upgradeJujuCommand upgrades the agents in a juju installation.
type upgradeJujuCommand struct {
	modelcmd.ModelCommandBase
	baseUpgradeCommand
}

func (c *upgradeJujuCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "upgrade-model",
		Purpose:  usageUpgradeJujuSummary,
		Doc:      usageUpgradeJujuDetails,
		Examples: usageUpgradeJujuExamples,
		SeeAlso: []string{
			"sync-agent-binary",
		},
	})
}

func (c *upgradeJujuCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.baseUpgradeCommand.SetFlags(f)
}

var (
	errUpToDate = stderrors.New("no upgrades available")
)

// ModelConfigAPI defines the model config API methods.
type ModelConfigAPI interface {
	ModelGet() (map[string]interface{}, error)
	Close() error
}

// ModelUpgraderAPI defines model upgrader API methods.
type ModelUpgraderAPI interface {
	UpgradeModel(
		modelUUID string, targetVersion version.Number, stream string, ignoreAgentVersions, druRun bool,
	) (version.Number, error)
	UploadTools(r io.ReadSeeker, vers version.Binary) (coretools.List, error)

	Close() error
}

// ControllerAPI defines the controller API methods.
type ControllerAPI interface {
	CloudSpec(modelTag names.ModelTag) (environscloudspec.CloudSpec, error)
	ControllerConfig() (controller.Config, error)
	ModelConfig() (map[string]interface{}, error)
	Close() error
}

func (c *upgradeJujuCommand) getModelUpgraderAPI() (ModelUpgraderAPI, error) {
	if c.modelUpgraderAPI != nil {
		return c.modelUpgraderAPI, nil
	}

	return c.NewModelUpgraderAPIClient()
}

func (c *upgradeJujuCommand) getModelConfigAPI() (ModelConfigAPI, error) {
	if c.modelConfigAPI != nil {
		return c.modelConfigAPI, nil
	}

	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(api), nil
}

func (c *upgradeJujuCommand) getControllerAPI() (ControllerAPI, error) {
	if c.controllerAPI != nil {
		return c.controllerAPI, nil
	}

	api, err := c.NewControllerAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apicontroller.NewClient(api), nil
}

// Run changes the version proposed for the juju envtools.
func (c *upgradeJujuCommand) Run(ctx *cmd.Context) (err error) {
	modelType, err := c.ModelType()
	if err != nil {
		return errors.Trace(err)
	}
	if modelType == model.CAAS {
		if c.BuildAgent {
			return errors.NotSupportedf("--build-agent for k8s model upgrades")
		}
	}
	return c.upgradeModel(ctx, c.timeout, modelType)
}

func uploadTools(
	modelUpgrader ModelUpgraderAPI, buildAgent bool, agentVersion version.Number, dryRun bool,
) (targetVersion version.Number, err error) {
	builtTools, err := sync.BuildAgentTarball(
		buildAgent, "upgrade",
		func(builtVersion version.Number) version.Number {
			builtVersion.Build++
			if agentVersion.Build >= builtVersion.Build {
				builtVersion.Build = agentVersion.Build + 1
			}
			targetVersion = builtVersion
			return builtVersion
		},
	)
	if err != nil {
		return targetVersion, errors.Trace(err)
	}
	defer os.RemoveAll(builtTools.Dir)

	if dryRun {
		logger.Debugf("dryrun, skipping upload agent binary")
		return targetVersion, nil
	}

	uploadToolsVersion := builtTools.Version
	uploadToolsVersion.Number = targetVersion

	toolsPath := path.Join(builtTools.Dir, builtTools.StorageName)
	logger.Infof("uploading agent binary %v (%dkB) to Juju controller", targetVersion, (builtTools.Size+512)/1024)
	f, err := os.Open(toolsPath)
	if err != nil {
		return targetVersion, errors.Trace(err)
	}
	defer f.Close()

	_, err = modelUpgrader.UploadTools(f, uploadToolsVersion)
	if err != nil {
		return targetVersion, errors.Trace(err)
	}
	return targetVersion, nil
}

func (c *upgradeJujuCommand) upgradeWithTargetVersion(
	ctx *cmd.Context, modelUpgrader ModelUpgraderAPI, isControllerModel, dryRun bool,
	modelType model.ModelType, targetVersion, agentVersion version.Number,
) (chosenVersion version.Number, err error) {
	// juju upgrade-controller --agent-version 3.x.x
	chosenVersion = targetVersion

	_, err = c.notifyControllerUpgrade(ctx, modelUpgrader, targetVersion, dryRun)
	if err == nil {
		// All good!
		// Upgraded to the provided target version.
		logger.Debugf("upgraded to the provided target version %q", targetVersion)
		return chosenVersion, nil
	}
	if !errors.Is(err, errors.NotFound) {
		return chosenVersion, err
	}

	// If target version is the current local binary version, then try to upload.
	canImplicitUpload := CheckCanImplicitUpload(
		modelType, isOfficialClient(), jujuversion.Current, agentVersion,
	)
	if !canImplicitUpload || !isControllerModel {
		// expecting to upload a local binary but we are not allowed to upload, so pretend there
		// is no more recent version available.
		logger.Debugf("no available binary found, and we are not allowed to upload, err %v", err)
		return chosenVersion, errUpToDate
	}

	if targetVersion.Compare(jujuversion.Current.ToPatch()) != 0 {
		logger.Warningf(
			"try again with --agent-version=%s if you want to upgrade using the local packaged jujud from the snap",
			jujuversion.Current.ToPatch(),
		)
		return chosenVersion, errUpToDate
	}

	// found a best target version but a local binary is required to be uploaded.
	if chosenVersion, err = uploadTools(modelUpgrader, false, agentVersion, dryRun); err != nil {
		return chosenVersion, block.ProcessBlockedError(err, block.BlockChange)
	}
	fmt.Fprintf(ctx.Stdout,
		"no prepackaged agent binaries available, using the local snap jujud %v%s\n",
		chosenVersion, "",
	)

	chosenVersion, err = c.notifyControllerUpgrade(ctx, modelUpgrader, chosenVersion, dryRun)
	if err != nil {
		return chosenVersion, err
	}
	return chosenVersion, nil
}

func (c *upgradeJujuCommand) upgradeModel(
	ctx *cmd.Context, fetchTimeout time.Duration,
	modelType model.ModelType,
) (err error) {
	targetVersion := c.Version
	defer func() {
		if err == nil {
			fmt.Fprintf(ctx.Stderr, "best version:\n    %v\n", targetVersion)
			if c.DryRun {
				if c.BuildAgent {
					fmt.Fprintf(ctx.Stderr, "%s --build-agent\n", c.upgradeMessage)
				} else {
					fmt.Fprintf(ctx.Stderr, "%s\n", c.upgradeMessage)
				}
			} else {
				fmt.Fprintf(ctx.Stdout, "started upgrade to %s\n", targetVersion)
			}
		}

		if err == errUpToDate {
			ctx.Infof(err.Error())
			err = nil
		}
		if err != nil {
			logger.Debugf("upgradeModel failed %v", err)
		}
	}()

	modelUpgrader, err := c.getModelUpgraderAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer modelUpgrader.Close()

	controllerClient, err := c.getControllerAPI()
	if err != nil {
		return err
	}
	defer controllerClient.Close()

	modelConfigClient, err := c.getModelConfigAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer modelConfigClient.Close()

	attrs, err := modelConfigClient.ModelGet()
	if err != nil {
		return errors.Trace(err)
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return errors.Trace(err)
	}

	agentVersion, ok := cfg.AgentVersion()
	if !ok {
		// Can't happen. In theory.
		return errors.New("incomplete model configuration")
	}

	if c.Version == agentVersion {
		return errUpToDate
	}

	controllerModelConfig, err := controllerClient.ModelConfig()
	if err != nil && !params.IsCodeUnauthorized(err) {
		return err
	}
	haveControllerModelPermission := err == nil
	isControllerModel := haveControllerModelPermission && cfg.UUID() == controllerModelConfig[config.UUIDKey]

	if c.BuildAgent {
		// For UploadTools, model must be the "controller" model,
		// that is, modelUUID == controllerUUID
		if !haveControllerModelPermission {
			return errors.New("--build-agent can only be used with the controller model but you don't have permission to access that model")
		}
		if !isControllerModel {
			return errors.Errorf("--build-agent can only be used with the controller model")
		}
		if targetVersion != version.Zero {
			return errors.Errorf("--build-agent cannot be used with --agent-version together")
		}
	}

	// Decide the target version to upgrade.
	if targetVersion != version.Zero {
		targetVersion, err = c.upgradeWithTargetVersion(
			ctx, modelUpgrader, isControllerModel, c.DryRun,
			modelType, targetVersion, agentVersion,
		)
		return err
	}
	if c.BuildAgent {
		if targetVersion, err = uploadTools(modelUpgrader, c.BuildAgent, agentVersion, c.DryRun); err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
		builtMsg := " (built from source)"
		fmt.Fprintf(ctx.Stdout,
			"no prepackaged agent binaries available, using local agent binary %v%s\n",
			targetVersion, builtMsg,
		)
		targetVersion, err = c.notifyControllerUpgrade(ctx, modelUpgrader, targetVersion, c.DryRun)
		return err
	}
	// juju upgrade-controller without --build-agent or --agent-version
	// or juju upgrade-model without --agent-version
	targetVersion, err = c.notifyControllerUpgrade(
		ctx, modelUpgrader,
		version.Zero, // no target version provided, we figure it out on the server side.
		c.DryRun,
	)
	if err == nil {
		// All good!
		// Upgraded to a next stable version or the newest stable version.
		logger.Debugf("upgraded to a next version or latest stable version")
		return nil
	}
	if errors.Is(err, errors.NotFound) {
		return errUpToDate
	}
	return err
}

// For test.
var CheckCanImplicitUpload = checkCanImplicitUpload

func checkCanImplicitUpload(
	modelType model.ModelType, isOfficialClient bool,
	clientVersion, agentVersion version.Number,
) bool {
	if modelType != model.IAAS {
		logger.Tracef("the model is not IAAS model")
		return false
	}

	if !isOfficialClient {
		logger.Tracef("the client is not an official client")
		// For non official (under $GOPATH) client, always use --build-agent explicitly.
		return false
	}
	newerClient := clientVersion.Compare(agentVersion.ToPatch()) >= 0
	if !newerClient {
		logger.Tracef(
			"the client version(%s) is not newer than agent version(%s)",
			clientVersion, agentVersion.ToPatch(),
		)
		return false
	}

	if agentVersion.Build > 0 || clientVersion.Build > 0 {
		return true
	}
	return false
}

func isOfficialClient() bool {
	// If there's an error getting jujud version, play it safe.
	// We pretend it's not official and don't do an implicit upload.
	jujudPath, err := tools.ExistingJujuLocation()
	if err != nil {
		return false
	}
	_, official, err := tools.JujudVersion(jujudPath)
	if err != nil {
		return false
	}
	// For non official (under $GOPATH) client, always use --build-agent explicitly.
	// For official (under /snap/juju/bin) client, upload only if the client is not a published version.
	return official
}

func (c *upgradeJujuCommand) notifyControllerUpgrade(
	ctx *cmd.Context, modelUpgrader ModelUpgraderAPI, targetVersion version.Number, dryRun bool,
) (chosenVersion version.Number, err error) {
	_, details, err := c.ModelCommandBase.ModelDetails()
	if err != nil {
		return chosenVersion, errors.Trace(err)
	}
	modelTag := names.NewModelTag(details.ModelUUID)

	if chosenVersion, err = modelUpgrader.UpgradeModel(
		modelTag.Id(), targetVersion, c.AgentStream, c.IgnoreAgentVersions, dryRun,
	); err != nil {
		if params.IsCodeUpgradeInProgress(err) {
			return chosenVersion, errors.Errorf("%s\n\n"+
				"Please wait for the upgrade to complete or if there was a problem with\n"+
				"the last upgrade that has been resolved, consider running the\n"+
				"upgrade-model command with the --reset-previous-upgrade option.", err,
			)
		}
		if errors.Is(err, errors.AlreadyExists) {
			err = errUpToDate
		}
		return chosenVersion, block.ProcessBlockedError(err, block.BlockChange)
	}
	return chosenVersion, nil
}
