// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/api/client/modelupgrader"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

var usageUpgradeControllerSummary = `
Upgrades Juju on a controller.`[1:]

var usageUpgradeControllerDetails = `
This command upgrades the Juju agent for a controller.

A controller's agent version can be shown with `[1:] + "`juju model-config -m controller agent-version`" + `.
A version is denoted by: major.minor.patch

You can upgrade the controller to a new patch version by specifying
the '--agent-version' flag. If not specified, the upgrade candidate
will default to the most recent patch version matching the current 
major and minor version. Upgrading to a new major or minor version is
not supported.

The command will abort if an upgrade is in progress. It will also abort if
a previous upgrade was not fully completed (e.g.: if one of the
controllers in a high availability model failed to upgrade).

`

const usageUpgradeControllerExamples = `
    juju upgrade-controller --dry-run
    juju upgrade-controller --agent-version 2.0.1
`

const upgradeControllerMessage = "upgrade to this version by running\n    juju upgrade-controller"

func newUpgradeControllerCommand(options ...modelcmd.WrapControllerOption) cmd.Command {
	command := &upgradeControllerCommand{}
	return modelcmd.WrapController(command, options...)
}

// upgradeControllerCommand upgrades the agents in a juju installation.
type upgradeControllerCommand struct {
	modelcmd.ControllerCommandBase

	vers        string
	Version     semversion.Number
	BuildAgent  bool
	DryRun      bool
	AssumeYes   bool
	AgentStream string
	timeout     time.Duration
	// IgnoreAgentVersions is used to allow an admin to request an agent
	// version without waiting for all agents to be at the right version.
	IgnoreAgentVersions bool

	modelConfigAPI   ModelConfigAPI
	modelUpgraderAPI ModelUpgraderAPI

	controllerModelDetails *jujuclient.ModelDetails
}

func (c *upgradeControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "upgrade-controller",
		Purpose:  usageUpgradeControllerSummary,
		Doc:      usageUpgradeControllerDetails,
		Examples: usageUpgradeControllerExamples,
		SeeAlso: []string{
			"upgrade-model",
		},
	})
}

func (c *upgradeControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)

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

func (c *upgradeControllerCommand) Init(args []string) error {
	if c.vers != "" {
		vers, err := semversion.Parse(c.vers)
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

func (c *upgradeControllerCommand) getModelUpgraderAPI(ctx context.Context) (ModelUpgraderAPI, error) {
	if c.modelUpgraderAPI != nil {
		return c.modelUpgraderAPI, nil
	}
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelupgrader.NewClient(root), nil
}

func (c *upgradeControllerCommand) getModelConfigAPI(ctx context.Context) (ModelConfigAPI, error) {
	if c.modelConfigAPI != nil {
		return c.modelConfigAPI, nil
	}
	api, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(api), nil
}

// TODO(jujud-controller-snap): remove if not needed in final upgrade command.
// type ClientAPI interface {
// 	Status(args *apiclient.StatusArgs) (*params.FullStatus, error)
// }
// func (c *upgradeControllerCommand) getAPIClient() (ClientAPI, error) {
// 	if c.clientAPI != nil {
// 		return c.clientAPI, nil
// 	}
// 	api, err := c.NewModelAPIRoot(bootstrap.ControllerModelName)
// 	if err != nil {
// 		return nil, errors.Trace(err)
// 	}
// 	return apiclient.NewClient(api, logger), nil
// }

// Run changes the version proposed for the juju envtools.
func (c *upgradeControllerCommand) Run(ctx *cmd.Context) (err error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}

	store := c.ClientStore()
	accDetails, err := store.AccountDetails(controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	if !permission.Access(accDetails.LastKnownAccess).EqualOrGreaterControllerAccessThan(permission.SuperuserAccess) {
		return errors.Errorf("upgrade not possible missing"+
			" permissions, current level %q, need: %q", accDetails.LastKnownAccess, permission.SuperuserAccess)
	}
	controllerModel := jujuclient.JoinOwnerModelName(
		names.NewUserTag(environs.AdminUser), bootstrap.ControllerModelName)
	c.controllerModelDetails, err = store.ModelByName(controllerName, controllerModel)
	if err != nil {
		return errors.Annotatef(err, "cannot get controller model")
	}
	//c.fullControllerModelName = modelcmd.JoinModelName(controllerName, controllerModel)

	if c.controllerModelDetails.ModelType == model.CAAS {
		if c.BuildAgent {
			return errors.NotSupportedf("--build-agent for k8s model upgrades")
		}
	}
	return c.upgradeController(ctx, c.timeout, c.controllerModelDetails.ModelType)
}

func (c *upgradeControllerCommand) uploadTools(
	ctx context.Context, modelUpgrader ModelUpgraderAPI, buildAgent bool, agentVersion semversion.Number, dryRun bool,
) (targetVersion semversion.Number, err error) {
	builtTools, err := sync.BuildAgentTarball(
		buildAgent, "upgrade",
		func(builtVersion semversion.Number) semversion.Number {
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
		logger.Debugf(context.TODO(), "dryrun, skipping upload agent binary")
		return targetVersion, nil
	}

	uploadToolsVersion := builtTools.Version
	uploadToolsVersion.Number = targetVersion

	toolsPath := path.Join(builtTools.Dir, builtTools.StorageName)
	logger.Infof(context.TODO(), "uploading agent binary %v (%dkB) to Juju controller", targetVersion, (builtTools.Size+512)/1024)
	f, err := os.Open(toolsPath)
	if err != nil {
		return targetVersion, errors.Trace(err)
	}
	defer f.Close()

	_, err = modelUpgrader.UploadTools(ctx, f, uploadToolsVersion)
	if err != nil {
		return targetVersion, errors.Trace(err)
	}
	return targetVersion, nil
}

func (c *upgradeControllerCommand) upgradeWithTargetVersion(
	ctx *cmd.Context, modelUpgrader ModelUpgraderAPI, dryRun bool,
	modelType model.ModelType, targetVersion, agentVersion semversion.Number,
) (chosenVersion semversion.Number, err error) {
	chosenVersion, err = c.notifyControllerUpgrade(ctx, modelUpgrader, targetVersion, dryRun)
	if err == nil {
		// All good!
		// Upgraded to the provided target version.
		logger.Debugf(context.TODO(), "upgraded to the provided target version %q", targetVersion)
		return chosenVersion, nil
	}
	if !errors.Is(err, errors.NotFound) {
		return chosenVersion, err
	}

	// If target version is the current local binary version, then try to upload.
	canImplicitUpload := CheckCanImplicitUpload(
		modelType, isOfficialClient(), jujuversion.Current, agentVersion,
	)
	if !canImplicitUpload {
		// expecting to upload a local binary but we are not allowed to upload, so pretend there
		// is no more recent version available.
		logger.Debugf(context.TODO(), "no available binary found, and we are not allowed to upload, err %v", err)
		return chosenVersion, errUpToDate
	}

	if targetVersion.Compare(jujuversion.Current.ToPatch()) != 0 {
		logger.Warningf(context.TODO(),
			"try again with --agent-version=%s if you want to upgrade using the local packaged jujud from the snap",
			jujuversion.Current.ToPatch(),
		)
		return chosenVersion, errUpToDate
	}

	// found a best target version but a local binary is required to be uploaded.
	if chosenVersion, err = c.uploadTools(ctx, modelUpgrader, false, agentVersion, dryRun); err != nil {
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

func (c *upgradeControllerCommand) upgradeController(
	ctx *cmd.Context, fetchTimeout time.Duration,
	modelType model.ModelType,
) (err error) {
	targetVersion := c.Version
	defer func() {
		if err == nil {
			fmt.Fprintf(ctx.Stderr, "best version:\n    %v\n", targetVersion)
			if c.DryRun {
				if c.BuildAgent {
					fmt.Fprintf(ctx.Stderr, "%s --build-agent\n", upgradeControllerMessage)
				} else {
					fmt.Fprintf(ctx.Stderr, "%s\n", upgradeControllerMessage)
				}
			} else {
				fmt.Fprintf(ctx.Stdout, "started upgrade to %s\n", targetVersion)
			}
		}

		if errors.Is(err, errUpToDate) {
			ctx.Infof("%s", err.Error())
			err = nil
		}
		if err != nil {
			logger.Debugf(context.TODO(), "upgradeController failed %v", err)
		}
	}()

	modelUpgrader, err := c.getModelUpgraderAPI(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer modelUpgrader.Close()

	modelConfigClient, err := c.getModelConfigAPI(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer modelConfigClient.Close()

	attrs, err := modelConfigClient.ModelGet(ctx)
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

	if c.BuildAgent {
		if targetVersion != semversion.Zero {
			return errors.Errorf("--build-agent cannot be used with --agent-version together")
		}
	}

	// Decide the target version to upgrade.
	if targetVersion != semversion.Zero {
		targetVersion, err = c.upgradeWithTargetVersion(
			ctx, modelUpgrader, c.DryRun,
			modelType, targetVersion, agentVersion,
		)
		return err
	}
	if c.BuildAgent {
		if targetVersion, err = c.uploadTools(ctx, modelUpgrader, c.BuildAgent, agentVersion, c.DryRun); err != nil {
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
		semversion.Zero, // no target version provided, we figure it out on the server side.
		c.DryRun,
	)
	if err == nil {
		// All good!
		// Upgraded to a next stable version or the newest stable version.
		logger.Debugf(context.TODO(), "upgraded to a next version or latest stable version")
		return nil
	}
	if errors.Is(err, errors.NotFound) {
		return errUpToDate
	}
	return err
}

func (c *upgradeControllerCommand) notifyControllerUpgrade(
	ctx *cmd.Context, modelUpgrader ModelUpgraderAPI, targetVersion semversion.Number, dryRun bool,
) (chosenVersion semversion.Number, err error) {
	modelTag := names.NewModelTag(c.controllerModelDetails.ModelUUID)
	if chosenVersion, err = modelUpgrader.UpgradeModel(
		ctx,
		modelTag.Id(), targetVersion, c.AgentStream, c.IgnoreAgentVersions, dryRun,
	); err != nil {
		if params.IsCodeUpgradeInProgress(err) {
			return chosenVersion, errors.Errorf("%s\n\n"+
				"Please wait for the upgrade to complete.", err,
			)
		}
		if errors.Is(err, errors.AlreadyExists) {
			err = errUpToDate
		}
		return chosenVersion, block.ProcessBlockedError(err, block.BlockChange)
	}
	return chosenVersion, nil
}

// For test.
var CheckCanImplicitUpload = checkCanImplicitUpload

func checkCanImplicitUpload(
	modelType model.ModelType, isOfficialClient bool,
	clientVersion, agentVersion semversion.Number,
) bool {
	if modelType != model.IAAS {
		logger.Tracef(context.TODO(), "the model is not IAAS model")
		return false
	}

	if !isOfficialClient {
		logger.Tracef(context.TODO(), "the client is not an official client")
		// For non official (under $GOPATH) client, always use --build-agent explicitly.
		return false
	}
	newerClient := clientVersion.Compare(agentVersion.ToPatch()) >= 0
	if !newerClient {
		logger.Tracef(context.TODO(),
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
