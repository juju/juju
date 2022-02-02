// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bufio"
	stdcontext "context"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	apicontroller "github.com/juju/juju/api/controller"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/jujuclient"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/upgrades"
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
The upgrade candidate will be auto-selected if '--agent-version' is not
specified:
 - If the server major version matches the client major version, the
 version selected is minor+1. If such a minor version is not available then
 the next patch version is chosen.
 - If the server major version does not match the client major version,
 the version selected is that of the client version.
If the controller is without internet access, the client must first supply
the software to the controller's cache via the ` + "`juju sync-agent-binaries`" + ` command.
The command will abort if an upgrade is in progress. It will also abort if
a previous upgrade was not fully completed (e.g.: if one of the
controllers in a high availability model failed to upgrade).
When looking for an agent to upgrade to Juju will check the currently
configured agent stream for that model. It's possible to overwrite this for
the lifetime of this upgrade using --agent-stream
If a failed upgrade has been resolved, '--reset-previous-upgrade' can be
used to allow the upgrade to proceed.
Backups are recommended prior to upgrading.

Examples:
    juju upgrade-model --dry-run
    juju upgrade-model --agent-version 2.0.1
    juju upgrade-model --agent-stream proposed
    
See also: 
    sync-agent-binaries`

func newUpgradeJujuCommand() cmd.Command {
	command := &upgradeJujuCommand{}
	return modelcmd.Wrap(command)
}

func newUpgradeJujuCommandForTest(
	store jujuclient.ClientStore,
	jujuClientAPI ClientAPI,
	modelConfigAPI modelConfigAPI,
	modelManagerAPI modelManagerAPI,
	controllerAPI ControllerAPI,
	options ...modelcmd.WrapOption,
) cmd.Command {
	command := &upgradeJujuCommand{
		baseUpgradeCommand: baseUpgradeCommand{
			modelConfigAPI:  modelConfigAPI,
			modelManagerAPI: modelManagerAPI,
			controllerAPI:   controllerAPI,
		},
		jujuClientAPI: jujuClientAPI,
	}
	command.SetClientStore(store)
	return modelcmd.Wrap(command, options...)
}

// baseUpgradeCommand is used by both the
// upgradeJujuCommand and upgradeControllerCommand
// to hold flags common to both.
type baseUpgradeCommand struct {
	vers          string
	Version       version.Number
	BuildAgent    bool
	DryRun        bool
	ResetPrevious bool
	AssumeYes     bool
	AgentStream   string
	timeout       time.Duration
	// IgnoreAgentVersions is used to allow an admin to request an agent version without waiting for all agents to be at the right
	// version.
	IgnoreAgentVersions bool

	rawArgs        []string
	upgradeMessage string

	modelConfigAPI  modelConfigAPI
	modelManagerAPI modelManagerAPI
	controllerAPI   ControllerAPI
}

func (c *baseUpgradeCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.vers, "agent-version", "", "Upgrade to specific version")
	f.StringVar(&c.AgentStream, "agent-stream", "", "Check this agent stream for upgrades")
	f.BoolVar(&c.BuildAgent, "build-agent", false, "Build a local version of the agent binary; for development use only")
	f.BoolVar(&c.DryRun, "dry-run", false, "Don't change anything, just report what would be changed")
	f.BoolVar(&c.ResetPrevious, "reset-previous-upgrade", false, "Clear the previous (incomplete) upgrade status (use with care)")
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

func (c *baseUpgradeCommand) precheckVersion(ctx *cmd.Context, agentVersion version.Number) (bool, error) {
	if c.BuildAgent && c.Version == version.Zero {
		// Currently, uploading tools assumes the version to be
		// the same as jujuversion.Current if not specified with
		// --agent-version.
		c.Version = jujuversion.Current
	}
	warnCompat := false

	// TODO (agprado:01/30/2018):
	// This logic seems to be overly complicated and it checks the same condition multiple times.
	switch {
	case !canUpgradeRunningVersion(agentVersion):
		// This version of upgrade-model cannot upgrade the running
		// environment version (can't guarantee API compatibility).
		return false, errors.Errorf("cannot upgrade a %s model with a %s client",
			agentVersion, jujuversion.Current)
	case c.Version != version.Zero && compareNoBuild(agentVersion, c.Version) == 1:
		// The specified version would downgrade the environment.
		// Don't upgrade and return an error.
		return false, errors.Errorf(downgradeErrMsg, agentVersion, c.Version)
	case agentVersion.Major != jujuversion.Current.Major:
		// Running environment is the previous major version (a higher major
		// version wouldn't have passed the check in canUpgradeRunningVersion).
		if c.Version == version.Zero || c.Version.Major == agentVersion.Major {
			// Not requesting an upgrade across major release boundary.
			// Warn of incompatible CLI and filter on the prior major version
			// when searching for available tools.
			// TODO(cherylj) Add in a suggestion to upgrade to 2.0 if
			// no matching tools are found (bug 1532670)
			warnCompat = true
			break
		}
		// User requested an upgrade to the next major version.
		// Fallthrough to the next case to verify that the upgrade
		// conditions are met.
		fallthrough
	case c.Version.Major > agentVersion.Major:
		// User is requesting an upgrade to a new major number
		// Only upgrade to a different major number if:
		// 1 - Explicitly requested with --agent-version or using --build-agent, and
		// 2 - The model is running a valid version to upgrade from.
		allowed, minVer, err := upgrades.UpgradeAllowed(agentVersion, c.Version)
		if err != nil {
			return false, errors.Trace(err)
		}
		retErr := false
		if !allowed {
			ctx.Infof("upgrades to a new major version must first go through %s",
				minVer)
			retErr = true
		}
		if retErr {
			return false, errors.New("unable to upgrade to requested version")
		}
	}
	return warnCompat, nil
}

// upgradeJujuCommand upgrades the agents in a juju installation.
type upgradeJujuCommand struct {
	modelcmd.ModelCommandBase
	baseUpgradeCommand

	jujuClientAPI ClientAPI
}

func (c *upgradeJujuCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "upgrade-model",
		Purpose: usageUpgradeJujuSummary,
		Doc:     usageUpgradeJujuDetails,
		Aliases: []string{"upgrade-juju"},
	})
}

func (c *upgradeJujuCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.baseUpgradeCommand.SetFlags(f)
}

var (
	errUpToDate     = stderrors.New("no upgrades available")
	downgradeErrMsg = "cannot change version from %s to lower version %s"
)

// canUpgradeRunningVersion determines if the version of the running
// environment can be upgraded using this version of the
// upgrade-model command.  Only versions with a minor version
// of 0 are expected to be able to upgrade environments running
// the previous major version.
//
// This check is needed because we do not guarantee API
// compatibility across major versions.  For example, a 3.3.0
// version of the upgrade-model command may not know how to upgrade
// an environment running juju 4.0.0.
//
// The exception is that a N.*.* client must be able to upgrade
// an environment one major version prior (N-1.*.*) so that
// it can be used to upgrade the environment to N.0.*.  For
// example, the 3.*.* upgrade-model command must be able to upgrade
// environments running 2.* since it must be able to upgrade
// environments from 2.8.7 -> 3.*.*.
// We used to require that the minor version of a newer client had
// to be 0 but with snap auto update, the client can be any minor
// version so need to ensure that all N.*.* clients can upgrade
// N-1.*.* controllers.
func canUpgradeRunningVersion(runningAgentVer version.Number) bool {
	if runningAgentVer.Major == jujuversion.Current.Major {
		return true
	}
	if runningAgentVer.Major == (jujuversion.Current.Major - 1) {
		return true
	}
	return false
}

func formatVersions(agents coretools.Versions) string {
	formatted := set.NewStrings()
	for _, agent := range agents {
		formatted.Add(fmt.Sprintf("    %s", agent.AgentVersion().String()))
	}
	return strings.Join(formatted.SortedValues(), "\n")
}

type toolsAPI interface {
	FindTools(majorVersion, minorVersion int, osType, arch, agentStream string) (result params.FindToolsResult, err error)
	UploadTools(r io.ReadSeeker, vers version.Binary, additionalSeries ...string) (coretools.List, error)
}

type upgradeJujuAPI interface {
	AbortCurrentUpgrade() error
	SetModelAgentVersion(version version.Number, ignoreAgentVersion bool) error
	Close() error
}

type statusAPI interface {
	Status(patterns []string) (*params.FullStatus, error)
}

type ClientAPI interface {
	toolsAPI
	upgradeJujuAPI
	statusAPI
}

type modelConfigAPI interface {
	ModelGet() (map[string]interface{}, error)
	Close() error
}

type modelManagerAPI interface {
	ValidateModelUpgrade(modelTag names.ModelTag, force bool) error
	Close() error
}

type ControllerAPI interface {
	CloudSpec(modelTag names.ModelTag) (environscloudspec.CloudSpec, error)
	ControllerConfig() (controller.Config, error)
	ModelConfig() (map[string]interface{}, error)
	Close() error
}

func (c *upgradeJujuCommand) getJujuClientAPI() (ClientAPI, error) {
	if c.jujuClientAPI != nil {
		return c.jujuClientAPI, nil
	}

	return c.NewAPIClient()
}

func (c *upgradeJujuCommand) getModelManagerAPI() (modelManagerAPI, error) {
	if c.modelManagerAPI != nil {
		return c.modelManagerAPI, nil
	}

	return c.NewModelManagerAPIClient()
}

func (c *upgradeJujuCommand) getModelConfigAPI() (modelConfigAPI, error) {
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
	implicitAgentUploadAllowed := true
	if modelType == model.CAAS {
		if c.BuildAgent {
			return errors.NotSupportedf("--build-agent for k8s model upgrades")
		}
		implicitAgentUploadAllowed = false
	}
	return c.upgradeModel(ctx, implicitAgentUploadAllowed, c.timeout)
}

func (c *upgradeJujuCommand) upgradeModel(ctx *cmd.Context, implicitUploadAllowed bool, fetchTimeout time.Duration) (err error) {
	defer func() {
		if err != nil {
			logger.Debugf("upgradeModel failed %v", err)
		}
	}()
	// Validate a model can be upgraded, by running some pre-flight checks.
	if err := c.validateModelUpgrade(); err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	client, err := c.getJujuClientAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	modelConfigClient, err := c.getModelConfigAPI()
	if err != nil {
		return err
	}
	defer modelConfigClient.Close()
	controllerClient, err := c.getControllerAPI()
	if err != nil {
		return err
	}
	defer controllerClient.Close()
	defer func() {
		if err == errUpToDate {
			ctx.Infof(err.Error())
			err = nil
		}
	}()

	// Determine the version to upgrade to, uploading tools if necessary.
	attrs, err := modelConfigClient.ModelGet()
	if err != nil {
		return err
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return err
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
	}

	agentVersion, ok := cfg.AgentVersion()
	if !ok {
		// Can't happen. In theory.
		return errors.New("incomplete model configuration")
	}

	warnCompat, err := c.precheckVersion(ctx, agentVersion)
	if err != nil {
		return err
	}

	upgradeCtx, versionsErr := c.initVersions(client, cfg, agentVersion, warnCompat, fetchTimeout)
	if versionsErr != nil {
		return versionsErr
	}

	if err := c.precheckEnviron(upgradeCtx, controllerClient, agentVersion); err != nil {
		return err
	}

	tryImplicit := implicitUploadAllowed && len(upgradeCtx.packagedAgents) == 0

	// Look for any packaged binaries but only if we haven't been asked to build an agent.
	var packagedAgentErr error
	uploadLocalBinary := false
	if !c.BuildAgent {
		if tryImplicit {
			if tryImplicit, err = tryImplicitUpload(agentVersion); err != nil {
				return err
			}
		}
		if !tryImplicit && len(upgradeCtx.packagedAgents) == 0 {
			// No tools found and we shouldn't upload any, so if we are not asking for a
			// major upgrade, pretend there is no more recent version available.
			filterVersion := jujuversion.Current
			if warnCompat {
				filterVersion.Major--
			}
			if c.Version == version.Zero && agentVersion.Major == filterVersion.Major {
				return errUpToDate
			}
		}
		if packagedAgentErr = upgradeCtx.maybeChoosePackagedAgent(); packagedAgentErr != nil {
			ctx.Verbosef("%v", packagedAgentErr)
		}
		uploadLocalBinary = isControllerModel && packagedAgentErr != nil && tryImplicit
	}

	// If there's no packaged binaries, or we're running a custom build
	// or the user has asked for a new agent to be built, upload a local
	// jujud binary if possible.
	if !warnCompat && (uploadLocalBinary || c.BuildAgent) {
		controllerAgentCfg, err := config.New(config.NoDefaults, controllerModelConfig)
		if err != nil {
			return err
		}
		controllerAgentVersion, ok := controllerAgentCfg.AgentVersion()
		if !ok {
			// Can't happen. In theory.
			return errors.New("incomplete controller model configuration")
		}
		if err := upgradeCtx.uploadTools(client, c.BuildAgent, agentVersion, controllerAgentVersion, c.DryRun); err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
		builtMsg := ""
		if c.BuildAgent {
			builtMsg = " (built from source)"
		}
		fmt.Fprintf(ctx.Stdout, "no prepackaged agent binaries available, using local agent binary %v%s\n", upgradeCtx.chosen, builtMsg)
		packagedAgentErr = nil
	}
	if packagedAgentErr != nil {
		return packagedAgentErr
	}

	if err := upgradeCtx.validate(); err != nil {
		return err
	}
	ctx.Verbosef("available agent binaries:\n%s", formatVersions(upgradeCtx.packagedAgents))
	fmt.Fprintf(ctx.Stderr, "best version:\n    %v\n", upgradeCtx.chosen)
	if warnCompat {
		fmt.Fprintf(ctx.Stderr, "version %s incompatible with this client (%s)\n", upgradeCtx.chosen, jujuversion.Current)
	}
	if c.DryRun {
		if c.BuildAgent {
			fmt.Fprintf(ctx.Stderr, "%s --build-agent\n", c.upgradeMessage)
		} else {
			fmt.Fprintf(ctx.Stderr, "%s\n", c.upgradeMessage)
		}
	} else {
		return c.notifyControllerUpgrade(ctx, client, upgradeCtx)
	}
	return nil
}

func (c *baseUpgradeCommand) notifyControllerUpgrade(ctx *cmd.Context, client upgradeJujuAPI, upgradeCtx *upgradeContext) error {
	if c.ResetPrevious {
		if ok, err := c.confirmResetPreviousUpgrade(ctx); !ok || err != nil {
			const message = "previous upgrade not reset and no new upgrade triggered"
			if err != nil {
				return errors.Annotate(err, message)
			}
			return errors.New(message)
		}
		if err := client.AbortCurrentUpgrade(); err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
	}
	if err := client.SetModelAgentVersion(upgradeCtx.chosen, c.IgnoreAgentVersions); err != nil {
		if params.IsCodeUpgradeInProgress(err) {
			return errors.Errorf("%s\n\n"+
				"Please wait for the upgrade to complete or if there was a problem with\n"+
				"the last upgrade that has been resolved, consider running the\n"+
				"upgrade-model command with the --reset-previous-upgrade option.", err,
			)
		}
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	fmt.Fprintf(ctx.Stdout, "started upgrade to %s\n", upgradeCtx.chosen)
	return nil
}

// validateModelUpgrade checks to see if a model can be upgraded.
func (c *upgradeJujuCommand) validateModelUpgrade() error {
	_, details, err := c.ModelCommandBase.ModelDetails()
	if err != nil {
		return errors.Trace(err)
	}

	client, err := c.getModelManagerAPI()
	if err != nil {
		return errors.Trace(err)
	}

	// TODO (stickupkid): Define force for validation of model upgrade.
	// If the model to upgrade does not implement the minimum facade version
	// for validating, we return nil.
	if err = client.ValidateModelUpgrade(names.NewModelTag(details.ModelUUID), false); errors.IsNotImplemented(err) {
		return nil
	}
	return errors.Trace(err)
}

// environConfigGetter implements environs.EnvironConfigGetter for use
// to get an environ to be used by precheckEnviron.  It bridges the gap
// to allow controller versions of the methods to be used for getting
// the current environ.
type environConfigGetter struct {
	controllerAPI ControllerAPI

	modelTag names.ModelTag
}

// ModelConfig returns the complete config for the model.  It
// bridges the gap between EnvironConfigGetter.ModelConfig and
// controller.ModelConfig.
func (e environConfigGetter) ModelConfig() (*config.Config, error) {
	cfg, err := e.controllerAPI.ModelConfig()
	if err != nil {
		return nil, err
	}
	return config.New(config.NoDefaults, cfg)
}

// CloudSpec returns the cloud specification for the model associated
// with the upgrade command.
func (e environConfigGetter) CloudSpec() (environscloudspec.CloudSpec, error) {
	return e.controllerAPI.CloudSpec(e.modelTag)
}

var getEnviron = environs.GetEnviron

var getCAASBroker = func(getter environs.EnvironConfigGetter) (caas.Broker, error) {
	modelConfig, err := getter.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec, err := getter.CloudSpec()
	if err != nil {
		return nil, errors.Trace(err)
	}
	env, err := caas.New(stdcontext.TODO(), environs.OpenParams{
		Cloud:  cloudSpec,
		Config: modelConfig,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
}

// UpgradePrecheckEnviron combines two interfaces required by
// result of getEnviron. It is for testing purposes only.
type UpgradePrecheckEnviron interface {
	environs.Environ
	environs.JujuUpgradePrechecker
}

// precheckEnviron looks for available PrecheckUpgradeOperations from
// the current environs and runs them for the controller model.
func (c *upgradeJujuCommand) precheckEnviron(upgradeCtx *upgradeContext, api ControllerAPI, agentVersion version.Number) error {
	modelName, details, err := c.ModelCommandBase.ModelDetails()
	if err != nil {
		return err
	}

	// modelName in form user/model-name
	if modelName != jujuclient.JoinOwnerModelName(
		names.NewUserTag(environs.AdminUser), bootstrap.ControllerModelName) {
		return nil
	}
	cfgGetter := environConfigGetter{
		controllerAPI: api,
		modelTag:      names.NewModelTag(details.ModelUUID)}
	return doPrecheckEnviron(details.ModelType, cfgGetter, upgradeCtx, agentVersion)
}

// doPrecheckEnviron does the work on running precheck upgrade environ steps.
// This is split out from precheckEnviron to facilitate testing without the
// jujuconnsuite and without mocking a juju store.
func doPrecheckEnviron(modelType model.ModelType, cfgGetter environConfigGetter, upgradeCtx *upgradeContext, agentVersion version.Number) error {
	var (
		env interface{}
		err error
	)
	if modelType == model.CAAS {
		env, err = getCAASBroker(cfgGetter)
	} else {
		env, err = getEnviron(cfgGetter, environs.New)
	}
	if err != nil {
		return err
	}
	precheckEnv, ok := env.(environs.JujuUpgradePrechecker)
	if !ok {
		return nil
	}
	if err = precheckEnv.PreparePrechecker(); err != nil {
		return err
	}
	return checkEnvironForUpgrade(upgradeCtx.chosen, agentVersion, precheckEnv)
}

// checkEnvironForUpgrade returns an error if any of any Environs
// PrecheckUpgradeOperations fail.
func checkEnvironForUpgrade(targetVersion, agentVersion version.Number, precheckEnv environs.JujuUpgradePrechecker) error {
	for _, op := range precheckEnv.PrecheckUpgradeOperations() {
		if skipTarget(agentVersion, op.TargetVersion, targetVersion) {
			logger.Debugf("ignoring precheck upgrade operation for version %v",
				op.TargetVersion)
			continue
		}
		logger.Debugf("running precheck upgrade operation for version %v",
			op.TargetVersion)
		for _, step := range op.Steps {
			logger.Debugf("running precheck step %q", step.Description())
			if err := step.Run(); err != nil {
				return errors.Annotatef(err, "Unable to upgrade to %s:", targetVersion)
			}
		}
	}
	return nil
}

// skipTarget returns true if the from version is less than the target version
// AND the target version is greater than the to version.
// Borrowed from upgrades.opsIterator.
func skipTarget(from, target, to version.Number) bool {
	// Clear the version tag of the to release to ensure that all
	// upgrade steps for the release are run for alpha and beta
	// releases.
	// ...but only do this if the from version has actually changed,
	// lest we trigger upgrade mode unnecessarily for non-final
	// versions.
	if from.Compare(to) != 0 {
		to.Tag = ""
	}
	// Do not run steps for versions of Juju earlier or same as we are upgrading from.
	if target.Compare(from) <= 0 {
		return true
	}
	// Do not run steps for versions of Juju later than we are upgrading to.
	if target.Compare(to) > 0 {
		return true
	}
	return false
}

func tryImplicitUpload(agentVersion version.Number) (bool, error) {
	newerAgent := jujuversion.Current.Compare(agentVersion) > 0
	if newerAgent || agentVersion.Build > 0 || jujuversion.Current.Build > 0 {
		return true, nil
	}
	jujudPath, err := tools.ExistingJujuLocation()
	if err != nil {
		return false, errors.Trace(err)
	}
	_, official, err := tools.JujudVersion(jujudPath)
	// If there's an error getting jujud version, play it safe
	// and don't implicitly do an implicit upload.
	if err != nil {
		return false, nil
	}
	return !official, nil
}

const resetPreviousUpgradeMessage = `
WARNING! using --reset-previous-upgrade when an upgrade is in progress
will cause the upgrade to fail. Only use this option to clear an
incomplete upgrade where the root cause has been resolved.

Continue [y/N]? `

func (c *baseUpgradeCommand) confirmResetPreviousUpgrade(ctx *cmd.Context) (bool, error) {
	if c.AssumeYes {
		return true, nil
	}
	fmt.Fprint(ctx.Stdout, resetPreviousUpgradeMessage)
	scanner := bufio.NewScanner(ctx.Stdin)
	scanner.Scan()
	err := scanner.Err()
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.ToLower(scanner.Text())
	return answer == "y" || answer == "yes", nil
}

// initVersions collects state relevant to an upgrade decision. The returned
// agent and client versions, and the list of currently available tools, will
// always be accurate; the chosen version, and the flag indicating development
// mode, may remain blank until uploadTools or validate is called.
func (c *baseUpgradeCommand) initVersions(
	client toolsAPI, cfg *config.Config,
	agentVersion version.Number, filterOnPrior bool, timeout time.Duration,
) (*upgradeContext, error) {
	if c.Version == agentVersion {
		return nil, errUpToDate
	}
	filterVersion := jujuversion.Current
	if c.Version != version.Zero {
		filterVersion = c.Version
	} else if filterOnPrior {
		// Trying to find the latest of the prior major version.
		// TODO (cherylj) if no tools found, suggest upgrade to
		// the current client version.
		filterVersion.Major--
	}
	logger.Debugf("searching for %q agent binaries with major: %d", c.AgentStream, filterVersion.Major)
	streamVersions, err := fetchStreamsVersions(client, filterVersion.Major, c.AgentStream, timeout)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	logger.Debugf("fetched stream versions %s", streamVersions)
	agents, err := toolListToVersions(streamVersions)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &upgradeContext{
		agent:          agentVersion,
		client:         jujuversion.Current,
		chosen:         c.Version,
		packagedAgents: agents,
		config:         cfg,
	}, nil
}

// fetchStreamsVersions returns simplestreams agent metadata
// for the specified stream. timeout ensures we don't block forever.
func fetchStreamsVersions(
	client toolsAPI, majorVersion int, stream string, timeout time.Duration,
) (coretools.List, error) {
	// Use a go routine so we can timeout.
	result := make(chan coretools.List, 1)
	errChan := make(chan error, 1)
	go func() {
		findResult, err := client.FindTools(majorVersion, -1, "", "", stream)
		if err != nil {
			errChan <- err
		} else {
			result <- findResult.List
		}
	}()
	select {
	case <-time.After(timeout):
		return nil, errors.NewTimeout(nil, fmt.Sprintf("can not fetch available versions in %s", timeout.String()))
	case err := <-errChan:
		return nil, err
	case resultList := <-result:
		return resultList, nil
	}
}

// The default available agents come directly from streams metadata.
func toolListToVersions(streamsVersions coretools.List) (coretools.Versions, error) {
	agents := make(coretools.Versions, len(streamsVersions))
	for i, t := range streamsVersions {
		agents[i] = t
	}
	return agents, nil
}

// upgradeContext holds the version information for making upgrade decisions.
type upgradeContext struct {
	agent          version.Number
	client         version.Number
	chosen         version.Number
	packagedAgents coretools.Versions
	config         *config.Config
}

// uploadTools compiles jujud from $GOPATH and uploads it into the supplied
// storage. If no version has been explicitly chosen, the version number
// reported by the built tools will be based on the client version number.
// In any case, the version number reported will have a build component higher
// than that of any otherwise-matching available envtools.
// uploadTools resets the chosen version and replaces the available tools
// with the ones just uploaded.
func (context *upgradeContext) uploadTools(
	client ClientAPI, buildAgent bool, agentVersion version.Number, controllerAgentVersion version.Number, dryRun bool,
) (err error) {
	// TODO(fwereade): this is kinda crack: we should not assume that
	// jujuversion.Current matches whatever source happens to be built. The
	// ideal would be:
	//  1) compile jujud from $GOPATH into some build dir
	//  2) get actual version with `jujud version`
	//  3) check actual version for compatibility with CLI tools
	//  4) generate unique build version with reference to available tools
	//  5) force-version that unique version into the dir directly
	//  6) archive and upload the build dir
	// ...but there's no way we have time for that now. In the meantime,
	// considering the use cases, this should work well enough; but it
	// won't detect an incompatible major-version change, which is a shame.
	//
	// TODO(cherylj) If the determination of version changes, we will
	// need to also change the upgrade version checks in Run() that check
	// if a major upgrade is allowed.
	uploadBaseVersion := context.chosen
	if uploadBaseVersion == version.Zero {
		uploadBaseVersion = context.client
	}
	// If the Juju client matches the current running agent (excluding build number),
	// make sure the build number gets incremented.
	agentVersionCopy := agentVersion.ToPatch()
	uploadBaseVersionCopy := uploadBaseVersion.ToPatch()
	if agentVersionCopy.Compare(uploadBaseVersionCopy) == 0 {
		uploadBaseVersion = agentVersion
	}
	context.chosen = makeUploadVersion(uploadBaseVersion, context.packagedAgents)

	if dryRun {
		return nil
	}

	builtTools, err := sync.BuildAgentTarball(buildAgent, &context.chosen, "upgrade")
	if err != nil {
		return errors.Trace(err)
	}
	defer os.RemoveAll(builtTools.Dir)

	uploadToolsVersion := builtTools.Version
	if builtTools.Official {
		context.chosen = builtTools.Version.Number
	} else {
		uploadToolsVersion.Number = context.chosen
	}
	toolsPath := path.Join(builtTools.Dir, builtTools.StorageName)
	logger.Infof("uploading agent binary %v (%dkB) to Juju controller", uploadToolsVersion, (builtTools.Size+512)/1024)
	f, err := os.Open(toolsPath)
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()

	// Older 2.8 agents still look for tools based on series.
	// Newer 2.9+ controllers can deal with this but not older controllers.
	// Look at the model and get all series for all machines
	// and use those to create additional tools.
	// TODO(juju4) - remove this logic
	additionalSeries := set.NewStrings()
	if controllerAgentVersion.Major == 2 && controllerAgentVersion.Minor <= 8 {
		fullStatus, err := client.Status(nil)
		if err != nil {
			return errors.Trace(err)
		}
		for _, m := range fullStatus.Machines {
			additionalSeries.Add(m.Series)
		}
	}
	uploaded, err := client.UploadTools(f, uploadToolsVersion, additionalSeries.Values()...)
	if err != nil {
		return errors.Trace(err)
	}
	agents := make(coretools.Versions, len(uploaded))
	for i, t := range uploaded {
		agents[i] = t
	}
	context.packagedAgents = agents
	return nil
}

func (context *upgradeContext) maybeChoosePackagedAgent() (err error) {
	if context.chosen == version.Zero {
		// No explicitly specified version, so find the version to which we
		// need to upgrade. We find next available stable release to upgrade
		// to by incrementing the minor version, starting from the current
		// agent version and doing major.minor+1.patch=0.

		// Upgrading across a major release boundary requires that the version
		// be specified with --agent-version.
		nextVersion := context.agent
		nextVersion.Minor += 1
		nextVersion.Patch = 0
		// Set Tag to space so it will be considered lexicographically earlier
		// than any tagged version.
		nextVersion.Tag = " "

		newestNextStable, found := context.packagedAgents.NewestCompatible(nextVersion)
		if found {
			logger.Debugf("found a more recent stable version %s", newestNextStable)
			context.chosen = newestNextStable
		} else {
			newestCurrent, found := context.packagedAgents.NewestCompatible(context.agent)
			if found {
				logger.Debugf("found more recent current version %s", newestCurrent)
				context.chosen = newestCurrent
			} else {
				if context.agent.Major != context.client.Major {
					return errors.New("no compatible agent versions available")
				} else {
					return errors.New("no more recent supported versions available")
				}
			}
		}
	} else {
		// If not completely specified already, pick a single tools version.
		filter := coretools.Filter{Number: context.chosen}
		if context.packagedAgents, err = context.packagedAgents.Match(filter); err != nil {
			return errors.Wrap(err, errors.New("no matching agent versions available"))
		}
		context.chosen, context.packagedAgents = context.packagedAgents.Newest()
	}
	return nil
}

// validate ensures an upgrade can be done using the chosen agent version.
// If validate returns no error, the environment agent-version can be set to
// the value of the chosen agent field.
func (context *upgradeContext) validate() (err error) {
	if context.chosen == context.agent {
		return errUpToDate
	}

	// Disallow major.minor version downgrades.
	if context.chosen.Major < context.agent.Major ||
		context.chosen.Major == context.agent.Major && context.chosen.Minor < context.agent.Minor {
		// TODO(fwereade): I'm a bit concerned about old agent/CLI tools even
		// *connecting* to environments with higher agent-versions; but ofc they
		// have to connect in order to discover they shouldn't. However, once
		// any of our tools detect an incompatible version, they should act to
		// minimize damage: the CLI should abort politely, and the agents should
		// run an Upgrader but no other tasks.
		return errors.Errorf(downgradeErrMsg, context.agent, context.chosen)
	}

	return nil
}

// makeUploadVersion returns a copy of the supplied version with a build number
// higher than any of the supplied tools that share its major, minor and patch.
func makeUploadVersion(vers version.Number, existing coretools.Versions) version.Number {
	vers.Build++
	for _, t := range existing {
		if t.AgentVersion().Major != vers.Major || t.AgentVersion().Minor != vers.Minor || t.AgentVersion().Patch != vers.Patch {
			continue
		}
		if t.AgentVersion().Build >= vers.Build {
			vers.Build = t.AgentVersion().Build + 1
		}
	}
	return vers
}

func compareNoBuild(a, b version.Number) int {
	x := a.ToPatch()
	y := b.ToPatch()
	return x.Compare(y)
}
