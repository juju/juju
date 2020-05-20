// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bufio"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"github.com/juju/os/series"
	"github.com/juju/version"

	apicontroller "github.com/juju/juju/api/controller"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/jujuclient"
	coretools "github.com/juju/juju/tools"
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
	command := &upgradeJujuCommand{
		baseUpgradeCommand: baseUpgradeCommand{minMajorUpgradeVersion: minMajorUpgradeVersion}}
	return modelcmd.Wrap(command)
}

func newUpgradeJujuCommandForTest(
	store jujuclient.ClientStore,
	minUpgradeVers map[int]version.Number,
	jujuClientAPI jujuClientAPI,
	modelConfigAPI modelConfigAPI,
	controllerAPI controllerAPI,
	options ...modelcmd.WrapOption) cmd.Command {
	if minUpgradeVers == nil {
		minUpgradeVers = minMajorUpgradeVersion
	}
	command := &upgradeJujuCommand{
		baseUpgradeCommand: baseUpgradeCommand{
			minMajorUpgradeVersion: minUpgradeVers,
			modelConfigAPI:         modelConfigAPI,
			controllerAPI:          controllerAPI,
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

	// IgnoreAgentVersions is used to allow an admin to request an agent version without waiting for all agents to be at the right
	// version.
	IgnoreAgentVersions bool

	rawArgs        []string
	upgradeMessage string

	// minMajorUpgradeVersion maps known major numbers to
	// the minimum version that can be upgraded to that
	// major version.  For example, users must be running
	// 1.25.4 or later in order to upgrade to 2.0.
	minMajorUpgradeVersion map[int]version.Number

	modelConfigAPI modelConfigAPI
	controllerAPI  controllerAPI
}

func (u *baseUpgradeCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&u.vers, "agent-version", "", "Upgrade to specific version")
	f.StringVar(&u.AgentStream, "agent-stream", "", "Check this agent stream for upgrades")
	f.BoolVar(&u.BuildAgent, "build-agent", false, "Build a local version of the agent binary; for development use only")
	f.BoolVar(&u.DryRun, "dry-run", false, "Don't change anything, just report what would be changed")
	f.BoolVar(&u.ResetPrevious, "reset-previous-upgrade", false, "Clear the previous (incomplete) upgrade status (use with care)")
	f.BoolVar(&u.AssumeYes, "y", false, "Answer 'yes' to confirmation prompts")
	f.BoolVar(&u.AssumeYes, "yes", false, "")
	f.BoolVar(&u.IgnoreAgentVersions, "ignore-agent-versions", false,
		"Don't check if all agents have already reached the current version")
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
		// 2 - The environment is running a valid version to upgrade from, and
		// 3 - The upgrade is to a minor version of 0.
		if c.minMajorUpgradeVersion == nil {
			break
		}
		minVer, ok := c.minMajorUpgradeVersion[c.Version.Major]
		if !ok {
			return false, errors.Errorf("unknown version %q", c.Version)
		}
		retErr := false
		if c.Version.Minor != 0 {
			ctx.Infof("upgrades to %s must first go through juju %d.0",
				c.Version, c.Version.Major)
			retErr = true
		}
		if comp := agentVersion.Compare(minVer); comp < 0 {
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

	jujuClientAPI jujuClientAPI
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
	errUpToDate            = stderrors.New("no upgrades available")
	downgradeErrMsg        = "cannot change version from %s to lower version %s"
	minMajorUpgradeVersion = map[int]version.Number{
		2: version.MustParse("1.25.4"),
	}
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
// The exception is that a N.0.* client must be able to upgrade
// an environment one major version prior (N-1.*.*) so that
// it can be used to upgrade the environment to N.0.*.  For
// example, the 2.0.1 upgrade-model command must be able to upgrade
// environments running 1.* since it must be able to upgrade
// environments from 1.25.4 -> 2.0.*.
func canUpgradeRunningVersion(runningAgentVer version.Number) bool {
	if runningAgentVer.Major == jujuversion.Current.Major {
		return true
	}
	if jujuversion.Current.Minor == 0 && runningAgentVer.Major == (jujuversion.Current.Major-1) {
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
	FindTools(majorVersion, minorVersion int, series, arch, agentStream string) (result params.FindToolsResult, err error)
	UploadTools(r io.ReadSeeker, vers version.Binary, additionalSeries ...string) (coretools.List, error)
}

type upgradeJujuAPI interface {
	AbortCurrentUpgrade() error
	SetModelAgentVersion(version version.Number, ignoreAgentVersion bool) error
	Close() error
}

type jujuClientAPI interface {
	toolsAPI
	upgradeJujuAPI
}

type modelConfigAPI interface {
	ModelGet() (map[string]interface{}, error)
	Close() error
}

type controllerAPI interface {
	CloudSpec(modelTag names.ModelTag) (environs.CloudSpec, error)
	ControllerConfig() (controller.Config, error)
	ModelConfig() (map[string]interface{}, error)
	Close() error
}

func (c *upgradeJujuCommand) getJujuClientAPI() (jujuClientAPI, error) {
	if c.jujuClientAPI != nil {
		return c.jujuClientAPI, nil
	}

	return c.NewAPIClient()
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

func (c *upgradeJujuCommand) getControllerAPI() (controllerAPI, error) {
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
	// The default available agents come directly from streams metadata.
	availableAgents := func(controllerCfg controller.Config, majorVersion int, streamsVersions coretools.List) (coretools.Versions, error) {
		agents := make(coretools.Versions, len(streamsVersions))
		for i, t := range streamsVersions {
			agents[i] = t
		}
		return agents, nil
	}
	implicitAgentUploadAllowed := true
	fetchToolsTimeout := 10 * time.Minute
	if modelType == model.CAAS {
		if c.BuildAgent {
			return errors.NotSupportedf("--build-agent for k8s model upgrades")
		}
		implicitAgentUploadAllowed = false
		fetchToolsTimeout = caasStreamsTimeout
		availableAgents = c.initCAASVersions
	}
	return c.upgradeModel(ctx, implicitAgentUploadAllowed, fetchToolsTimeout, availableAgents)
}

func (c *upgradeJujuCommand) upgradeModel(ctx *cmd.Context, implicitUploadAllowed bool, fetchTimeout time.Duration, availableAgents availableAgentsFunc) (err error) {

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
	if err != nil {
		return err
	}
	isControllerModel := cfg.UUID() == controllerModelConfig[config.UUIDKey]
	if c.BuildAgent && !isControllerModel {
		// For UploadTools, model must be the "controller" model,
		// that is, modelUUID == controllerUUID
		return errors.Errorf("--build-agent can only be used with the controller model")
	}
	controllerCfg, err := controllerClient.ControllerConfig()
	if err != nil {
		return err
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

	upgradeCtx, versionsErr := c.initVersions(client, controllerCfg, cfg, agentVersion, warnCompat, fetchTimeout, availableAgents)
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
		if err := upgradeCtx.uploadTools(client, c.BuildAgent, agentVersion, c.DryRun); err != nil {
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
		} else {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
	}
	fmt.Fprintf(ctx.Stdout, "started upgrade to %s\n", upgradeCtx.chosen)
	return nil
}

// environConfigGetter implements environs.EnvironConfigGetter for use
// to get an environ to be used by precheckEnviron.  It bridges the gap
// to allow controller versions of the methods to be used for getting
// the current environ.
type environConfigGetter struct {
	controllerAPI

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
func (e environConfigGetter) CloudSpec() (environs.CloudSpec, error) {
	return e.controllerAPI.CloudSpec(e.modelTag)
}

var getEnviron = environs.GetEnviron

// UpgradePrecheckEnviron combines two interfaces required by
// result of getEnviron. It is for testing purposes only.
type UpgradePrecheckEnviron interface {
	environs.Environ
	environs.JujuUpgradePrechecker
}

// precheckEnviron looks for available PrecheckUpgradeOperations from
// the current environs and runs them for the controller model.
func (c *upgradeJujuCommand) precheckEnviron(upgradeCtx *upgradeContext, api controllerAPI, agentVersion version.Number) error {
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
	return doPrecheckEnviron(cfgGetter, upgradeCtx, agentVersion)
}

// doPrecheckEnviron does the work on running precheck upgrade environ steps.
// This is split out from precheckEnviron to facilitate testing without the
// jujuconnsuite and without mocking a juju store.
func doPrecheckEnviron(cfgGetter environConfigGetter, upgradeCtx *upgradeContext, agentVersion version.Number) error {
	env, err := getEnviron(cfgGetter, environs.New)
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

// availableAgentsFunc defines a function that returns the
// available agent versions for the given simple streams agent metadata.
type availableAgentsFunc func(controllerCfg controller.Config, majorVersion int, streamVersions coretools.List) (coretools.Versions, error)

// initVersions collects state relevant to an upgrade decision. The returned
// agent and client versions, and the list of currently available tools, will
// always be accurate; the chosen version, and the flag indicating development
// mode, may remain blank until uploadTools or validate is called.
func (c *baseUpgradeCommand) initVersions(
	client toolsAPI, controllerCfg controller.Config, cfg *config.Config,
	agentVersion version.Number, filterOnPrior bool, timeout time.Duration,
	availableAgents availableAgentsFunc,
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
	if err != nil && !params.IsCodeNotFound(err) {
		return nil, errors.Trace(err)
	}
	agents, err := availableAgents(controllerCfg, filterVersion.Major, streamVersions)
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
func (context *upgradeContext) uploadTools(client toolsAPI, buildAgent bool, agentVersion version.Number, dryRun bool) (err error) {
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
	seriesOs, err := series.GetOSFromSeries(builtTools.Version.Series)
	if err != nil {
		return errors.Trace(err)
	}
	additionalSeries := series.OSSupportedSeries(seriesOs)
	uploaded, err := client.UploadTools(f, uploadToolsVersion, additionalSeries...)
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
