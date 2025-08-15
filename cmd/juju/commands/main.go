// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"
	proxyutils "github.com/juju/proxy"

	cloudfile "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/juju/agree/agree"
	"github.com/juju/juju/cmd/juju/agree/listagreements"
	"github.com/juju/juju/cmd/juju/annotations"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/caas"
	"github.com/juju/juju/cmd/juju/charmhub"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/juju/crossmodel"
	"github.com/juju/juju/cmd/juju/dashboard"
	"github.com/juju/juju/cmd/juju/firewall"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/cmd/juju/metricsdebug"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/payload"
	"github.com/juju/juju/cmd/juju/resource"
	"github.com/juju/juju/cmd/juju/secretbackends"
	"github.com/juju/juju/cmd/juju/secrets"
	"github.com/juju/juju/cmd/juju/setmeterstatus"
	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/cmd/juju/ssh"
	"github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/cmd/juju/subnet"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/cmd/juju/waitfor"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/utils/proxy"
	jujuversion "github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.cmd.juju.commands")

func init() {
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey, osenv.JujuFeatures)
}

// TODO(ericsnow) Move the following to cmd/juju/main.go:
//  jujuDoc
//  Main

var jujuDoc = `
Juju provides easy, intelligent application orchestration on top of Kubernetes,
cloud infrastructure providers such as Amazon, Google, Microsoft, Openstack,
MAAS (and more!), or even on your local machine via LXD.

See https://juju.is for getting started tutorials and additional documentation.
`

var usageHelp = `
Juju provides easy, intelligent application orchestration on top of Kubernetes,
cloud infrastructure providers such as Amazon, Google, Microsoft, Openstack,
MAAS (and more!), or even your local machine via LXD.

See https://juju.is for getting started tutorials and additional documentation.

Starter commands:

    bootstrap           Initializes a cloud environment.
    add-model           Adds a workload model.
    deploy              Deploys a new application.
    status              Displays the current status of Juju, applications, and units.
    add-unit            Adds extra units of a deployed application.
    integrate           Adds an integration between two applications.
    expose              Makes an application publicly available over the network.
    models              Lists models a user can access on a controller.
    controllers         Lists all controllers.
    whoami              Display the current controller, model and logged in user name. 
    switch              Selects or identifies the current controller and model.
    add-k8s             Adds a k8s endpoint and credential to Juju.
    add-cloud           Adds a user-defined cloud to Juju.
    add-credential      Adds or replaces credentials for a cloud.

Interactive mode:

When run without arguments, Juju will enter an interactive shell which can be
used to run any Juju command directly.

Help commands:
    
    juju help           This help page.
    juju help <command> Show help for the specified command.

For the full list of supported commands run: 
    
    juju help commands`[1:]

const (
	cliHelpHint = `See "juju --help"`
)

var x = []byte("\x96\x8c\x8a\x91\x93\x9a\x9e\x8c\x97\x99\x8a\x9c\x94\x96\x91\x98\xdf\x9e\x92\x9e\x85\x96\x91\x98\xf5")

// Main registers subcommands for the juju executable, and hands over control
// to the cmd package. This function is not redundant with main, because it
// provides an entry point for testing with arbitrary command line arguments.
// This function returns the exit code, for main to pass to os.Exit.
func Main(args []string) int {
	return jujuMain{
		execCommand: exec.Command,
	}.Run(args)
}

// main is a type that captures dependencies for running the main function.
type jujuMain struct {
	// execCommand abstracts away exec.Command.
	execCommand func(command string, args ...string) *exec.Cmd
}

// Run is the main entry point for the juju client.
func (m jujuMain) Run(args []string) int {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		cmd.WriteError(os.Stderr, err)
		return 2
	}

	// note that this has to come before we init the juju home directory,
	// since it relies on detecting the lack of said directory.
	newInstall := !juju2xConfigDataExists()

	if err = juju.InitJujuXDGDataHome(); err != nil {
		cmd.WriteError(ctx.Stderr, err)
		if errors.Is(err, errors.NotFound) {
			ctx.Warningf("Installing Juju in a strictly confined Snap. To ensure correct operation, create the ~/.local/share/juju directory manually.")
		}
		return 2
	}

	if err := installProxy(); err != nil {
		cmd.WriteError(ctx.Stderr, err)
		return 2
	}

	var jujuMsg string
	if newInstall {
		if _, _, err := cloud.FetchAndMaybeUpdatePublicClouds(cloud.PublicCloudsAccess(), true); err != nil {
			cmd.WriteError(ctx.Stderr, err)
		}
		jujuMsg = fmt.Sprintf("Since Juju %v is being run for the first time, it has downloaded the latest public cloud information.\n", jujuversion.Current.Major)
	}

	for i := range x {
		x[i] ^= 255
	}
	if len(args) == 2 {
		if args[1] == string(x[0:2]) {
			os.Stdout.Write(x[9:])
			return 0
		}
		if args[1] == string(x[2:9]) {
			os.Stdout.Write(model.ExtractCert())
			return 0
		}
	}

	// See if we need to invoke the juju interactive shell.
	// It is invoked by:
	// $ juju
	// We also run the repl command so that it prints help and exits
	// if the user types:
	// $ juju --help | -h | help
	jujuArgs := set.NewStrings(args[1:]...)
	helpArgs := set.NewStrings("help", "-h", "--help")
	showHelp := jujuArgs.Intersection(helpArgs).Size() > 0 && jujuArgs.Size() == 1
	repl := jujuArgs.Size() == 0 && isTerminal(ctx.Stdout) && isTerminal(ctx.Stdin)
	if repl || showHelp {
		return cmd.Main(newReplCommand(showHelp), ctx, nil)
	}
	// We have registered a juju "version" command to replace the inbuilt one.
	// There's special processing to call the inbuilt version command if the
	// --version flag is set. But we want to invoke the juju version command.
	cmdArgs := make([]string, len(args)-1)
	flagIdx := -1
	cmdIdx := -1
	for i, arg := range args[1:] {
		if arg == "--version" {
			flagIdx = i
		}
		if cmdIdx == -1 && !strings.HasPrefix(arg, "--") && arg != "help" && arg != "version" {
			cmdIdx = i
		}
		cmdArgs[i] = arg
	}
	// If there is a command before the --version flag, don't change the flag
	// to the version command.
	if flagIdx != -1 && (cmdIdx > flagIdx || cmdIdx == -1) {
		cmdArgs[flagIdx] = "version"
	}
	jcmd := NewJujuCommand(ctx, jujuMsg)
	return cmd.Main(jcmd, ctx, cmdArgs)
}

func installProxy() error {
	// Set the default transport to use the in-process proxy
	// configuration.
	if err := proxy.DefaultConfig.Set(proxyutils.DetectProxies()); err != nil {
		return errors.Trace(err)
	}
	if err := proxy.DefaultConfig.InstallInDefaultTransport(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func juju2xConfigDataExists() bool {
	_, err := os.Stat(osenv.JujuXDGDataHomeDir())
	return err == nil
}

// NewJujuCommand creates the "juju" super command.
func NewJujuCommand(ctx *cmd.Context, jujuMsg string) cmd.Command {
	return NewJujuCommandWithStore(ctx, jujuclient.NewFileClientStore(), jujucmd.DefaultLog, jujuMsg, cliHelpHint, nil, false)
}

// NewJujuCommandWithStore creates the "juju" super command with the specified parameters.
func NewJujuCommandWithStore(
	ctx *cmd.Context, store jujuclient.ClientStore, log *cmd.Log, jujuMsg, helpHint string, whitelist []string, embedded bool,
) cmd.Command {
	var jcmd *cmd.SuperCommand
	var jujuRegistry *jujuCommandRegistry
	var missingCallback cmd.MissingCallback = func(ctx *cmd.Context, subcommand string, args []string) error {
		excluded := jujuRegistry.excluded.Contains(subcommand)
		if excluded {
			return errors.Errorf("juju %q is not supported when run via a controller API call", subcommand)
		}
		if cmdName, _, ok := jcmd.FindClosestSubCommand(subcommand); ok {
			return &NotFoundCommand{
				ArgName:  subcommand,
				CmdName:  cmdName,
				HelpHint: helpHint,
			}
		}
		return cmd.DefaultUnrecognizedCommand(subcommand)
	}
	if !embedded {
		missingCallback = RunPlugin(missingCallback)
	}
	jcmd = jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:                "juju",
		Doc:                 jujuDoc,
		Log:                 log,
		MissingCallback:     missingCallback,
		UserAliasesFilename: osenv.JujuXDGDataHomePath("aliases"),
		FlagKnownAs:         "option",
		NotifyRun: func(string) {
			if jujuMsg != "" {
				ctx.Infof("%s", jujuMsg)
			}
		},
	})
	jcmd.AddHelpTopic("basics", "Basic Help Summary", usageHelp)

	jujuRegistry = &jujuCommandRegistry{
		store:           store,
		commandRegistry: jcmd,
		whitelist:       set.NewStrings(whitelist...),
		excluded:        set.NewStrings(),
		embedded:        embedded,
	}
	registerCommands(jujuRegistry)
	return jcmd
}

const notFoundCommandMessage = `juju: %q is not a juju command. %s.

Did you mean:
	%s`

// NotFoundCommand gives valuable feedback to the operator about what commands
// could be available if a mistake around the subcommand name is given.
type NotFoundCommand struct {
	ArgName  string
	CmdName  string
	HelpHint string
}

func (c NotFoundCommand) Error() string {
	return fmt.Sprintf(notFoundCommandMessage, c.ArgName, c.HelpHint, c.CmdName)
}

type supportsEmbedded interface {
	SetEmbedded(bool)
}

type hasClientStore interface {
	SetClientStore(store jujuclient.ClientStore)
}

type jujuCommandRegistry struct {
	commandRegistry

	store     jujuclient.ClientStore
	whitelist set.Strings
	excluded  set.Strings
	embedded  bool
}

// Register adds a command to the registry so it can be used.
func (r jujuCommandRegistry) Register(c cmd.Command) {
	cmdName := c.Info().Name
	if r.whitelist.Size() > 0 && !r.whitelist.Contains(cmdName) {
		logger.Tracef("command %q not allowed", cmdName)
		r.excluded.Add(cmdName)
		return
	}
	if se, ok := c.(supportsEmbedded); ok {
		se.SetEmbedded(r.embedded)
	} else {
		logger.Tracef("command %q is not embeddable", cmdName)
	}
	if csc, ok := c.(hasClientStore); ok {
		csc.SetClientStore(r.store)
	}
	r.commandRegistry.Register(c)
}

type commandRegistry interface {
	Register(cmd.Command)
	RegisterSuperAlias(name, super, forName string, check cmd.DeprecationCheck)
	RegisterDeprecated(subcmd cmd.Command, check cmd.DeprecationCheck)
}

// TODO(ericsnow) Factor out the commands and aliases into a static
// registry that can be passed to the supercommand separately.

// registerCommands registers commands in the specified registry.
func registerCommands(r commandRegistry) {
	// NOTE:
	// When adding a new command here, consider if the command should also
	// be whitelisted for being enabled as an embedded command accessible to
	// the Dashboard.
	// Update allowedEmbeddedCommands in apiserver.go
	r.Register(newVersionCommand())

	// Annotation commands.
	r.Register(annotations.NewGetAnnotationsCommand())
	r.Register(annotations.NewSetAnnotationsCommand())

	// Creation commands.
	r.Register(newBootstrapCommand())
	r.Register(application.NewAddRelationCommand())

	// Cross model relations commands.
	r.Register(crossmodel.NewOfferCommand())
	r.Register(crossmodel.NewRemoveOfferCommand())
	r.Register(crossmodel.NewShowOfferedEndpointCommand())
	r.Register(crossmodel.NewListEndpointsCommand())
	r.Register(crossmodel.NewFindEndpointsCommand())
	r.Register(application.NewConsumeCommand())
	r.Register(application.NewSuspendRelationCommand())
	r.Register(application.NewResumeRelationCommand())

	// Firewall rule commands.
	r.Register(firewall.NewSetFirewallRuleCommand())
	r.Register(firewall.NewListFirewallRulesCommand())

	// Destruction commands.
	r.Register(application.NewRemoveRelationCommand())
	r.Register(application.NewRemoveApplicationCommand())
	r.Register(application.NewRemoveUnitCommand())
	r.Register(application.NewRemoveSaasCommand())

	// Reporting commands.
	r.Register(status.NewStatusCommand())
	r.Register(newSwitchCommand())
	r.Register(status.NewStatusHistoryCommand())

	// Error resolution and debugging commands.
	r.Register(action.NewExecCommand(nil))
	r.Register(ssh.NewSCPCommand(nil, ssh.DefaultSSHRetryStrategy, ssh.DefaultSSHPublicKeyRetryStrategy))
	r.Register(ssh.NewSSHCommand(nil, nil, ssh.DefaultSSHRetryStrategy, ssh.DefaultSSHPublicKeyRetryStrategy))
	r.Register(application.NewResolvedCommand())
	r.Register(newDebugLogCommand(nil))
	r.Register(ssh.NewDebugHooksCommand(nil, ssh.DefaultSSHRetryStrategy, ssh.DefaultSSHPublicKeyRetryStrategy))
	r.Register(ssh.NewDebugCodeCommand(nil, ssh.DefaultSSHRetryStrategy, ssh.DefaultSSHPublicKeyRetryStrategy))

	// Configuration commands.
	r.Register(model.NewModelGetConstraintsCommand())
	r.Register(model.NewModelSetConstraintsCommand())
	r.Register(newSyncAgentBinaryCommand())
	r.Register(newUpgradeModelCommand())
	r.Register(newUpgradeControllerCommand())
	r.Register(application.NewRefreshCommand())
	r.Register(application.NewSetApplicationBaseCommand())
	r.Register(application.NewBindCommand())

	// Charm tool commands.
	r.Register(newhelpHookCmdsCommand())
	r.Register(newHelpActionCmdsCommand())

	// Manage backups.
	r.Register(backups.NewCreateCommand())
	r.Register(backups.NewDownloadCommand())

	// Manage authorized ssh keys.
	r.Register(NewAddKeysCommand())
	r.Register(NewRemoveKeysCommand())
	r.Register(NewImportKeysCommand())
	r.Register(NewListKeysCommand())

	// Manage users and access
	r.Register(user.NewAddCommand())
	r.Register(user.NewChangePasswordCommand())
	r.Register(user.NewShowUserCommand())
	r.Register(user.NewListCommand())
	r.Register(user.NewEnableCommand())
	r.Register(user.NewDisableCommand())
	r.Register(user.NewLoginCommand())
	r.Register(user.NewLogoutCommand())
	r.Register(user.NewRemoveCommand())
	r.Register(user.NewWhoAmICommand())

	// Manage machines
	r.Register(machine.NewAddCommand())
	r.Register(machine.NewRemoveCommand())
	r.Register(machine.NewListMachinesCommand())
	r.Register(machine.NewShowMachineCommand())
	r.Register(machine.NewUpgradeMachineCommand())

	// Manage model
	r.Register(model.NewConfigCommand())
	r.Register(model.NewDefaultsCommand())
	r.Register(model.NewRetryProvisioningCommand())
	r.Register(model.NewDestroyCommand())
	r.Register(model.NewGrantCommand())
	r.Register(model.NewRevokeCommand())
	r.Register(model.NewShowCommand())
	r.Register(model.NewModelCredentialCommand())
	if featureflag.Enabled(feature.Branches) || featureflag.Enabled(feature.Generations) {
		r.Register(model.NewAddBranchCommand())
		r.Register(model.NewCommitCommand())
		r.Register(model.NewTrackBranchCommand())
		r.Register(model.NewBranchCommand())
		r.Register(model.NewDiffCommand())
		r.Register(model.NewAbortCommand())
		r.Register(model.NewCommitsCommand())
		r.Register(model.NewShowCommitCommand())
	}

	r.Register(newMigrateCommand())
	r.Register(model.NewExportBundleCommand())

	if featureflag.Enabled(feature.DeveloperMode) {
		r.Register(model.NewDumpCommand())
		r.Register(model.NewDumpDBCommand())
	}

	// Manage and control actions
	r.Register(action.NewListCommand())
	r.Register(action.NewShowCommand())
	r.Register(action.NewCancelCommand())
	r.Register(action.NewRunCommand())
	r.Register(action.NewListOperationsCommand())
	r.Register(action.NewShowOperationCommand())
	r.Register(action.NewShowTaskCommand())

	// Manage controller availability
	r.Register(newEnableHACommand())

	// Manage and control applications
	r.Register(application.NewAddUnitCommand())
	r.Register(application.NewConfigCommand())
	r.Register(application.NewDeployCommand())
	r.Register(application.NewExposeCommand())
	r.Register(application.NewUnexposeCommand())
	r.Register(application.NewApplicationGetConstraintsCommand())
	r.Register(application.NewApplicationSetConstraintsCommand())
	r.Register(application.NewDiffBundleCommand())
	r.Register(application.NewShowApplicationCommand())
	r.Register(application.NewShowUnitCommand())

	// Operation protection commands
	r.Register(block.NewDisableCommand())
	r.Register(block.NewListCommand())
	r.Register(block.NewEnableCommand())

	// Manage storage
	r.Register(storage.NewAddCommand())
	r.Register(storage.NewListCommand())
	r.Register(storage.NewPoolCreateCommand())
	r.Register(storage.NewPoolListCommand())
	r.Register(storage.NewPoolRemoveCommand())
	r.Register(storage.NewPoolUpdateCommand())
	r.Register(storage.NewShowCommand())
	r.Register(storage.NewRemoveStorageCommandWithAPI())
	r.Register(storage.NewDetachStorageCommandWithAPI())
	r.Register(storage.NewAttachStorageCommandWithAPI())
	r.Register(storage.NewImportFilesystemCommand(storage.NewStorageImporter, nil))

	// Manage spaces
	r.Register(space.NewAddCommand())
	r.Register(space.NewListCommand())
	r.Register(space.NewMoveCommand())
	r.Register(space.NewReloadCommand())
	r.Register(space.NewShowSpaceCommand())
	r.Register(space.NewRemoveCommand())
	r.Register(space.NewRenameCommand())

	// Manage subnets
	r.Register(subnet.NewListCommand())

	// Manage controllers
	r.Register(controller.NewAddModelCommand())
	r.Register(controller.NewDestroyCommand())
	r.Register(controller.NewListModelsCommand())
	r.Register(controller.NewKillCommand())
	r.Register(controller.NewListControllersCommand())
	r.Register(controller.NewRegisterCommand())
	r.Register(controller.NewUnregisterCommand(jujuclient.NewFileClientStore()))
	r.Register(controller.NewEnableDestroyControllerCommand())
	r.Register(controller.NewShowControllerCommand())
	r.Register(controller.NewConfigCommand())

	// Debug Metrics
	r.Register(metricsdebug.New())
	r.Register(metricsdebug.NewCollectMetricsCommand())
	r.Register(setmeterstatus.New())

	// Manage clouds and credentials
	r.Register(cloud.NewUpdateCloudCommand(&cloudToCommandAdapter{}))
	r.Register(cloud.NewUpdatePublicCloudsCommand())
	r.Register(cloud.NewListCloudsCommand())
	r.Register(cloud.NewListRegionsCommand())
	r.Register(cloud.NewShowCloudCommand())
	r.Register(cloud.NewAddCloudCommand(&cloudToCommandAdapter{}))
	r.Register(cloud.NewRemoveCloudCommand())
	r.Register(cloud.NewListCredentialsCommand())
	r.Register(cloud.NewDetectCredentialsCommand())
	r.Register(cloud.NewSetDefaultRegionCommand())
	r.Register(cloud.NewSetDefaultCredentialCommand())
	r.Register(cloud.NewAddCredentialCommand())
	r.Register(cloud.NewRemoveCredentialCommand())
	r.Register(cloud.NewUpdateCredentialCommand())
	r.Register(cloud.NewShowCredentialCommand())
	r.Register(model.NewGrantCloudCommand())
	r.Register(model.NewRevokeCloudCommand())

	// CAAS commands
	r.Register(caas.NewAddCAASCommand(&cloudToCommandAdapter{}))
	r.Register(caas.NewUpdateCAASCommand(&cloudToCommandAdapter{}))
	r.Register(caas.NewRemoveCAASCommand(&cloudToCommandAdapter{}))
	r.Register(application.NewScaleApplicationCommand())

	// Manage Application Credential Access
	r.Register(application.NewTrustCommand())

	// Juju Dashboard commands.
	r.Register(dashboard.NewDashboardCommand())

	// Resource commands.
	r.Register(resource.NewUploadCommand())
	r.Register(resource.NewListCommand())
	r.Register(resource.NewCharmResourcesCommand())

	// CharmHub related commands
	r.Register(charmhub.NewInfoCommand())
	r.Register(charmhub.NewFindCommand())
	r.Register(charmhub.NewDownloadCommand())

	// Secrets.
	r.Register(secrets.NewListSecretsCommand())
	r.Register(secrets.NewShowSecretsCommand())
	r.Register(secrets.NewAddSecretCommand())
	r.Register(secrets.NewUpdateSecretCommand())
	r.Register(secrets.NewRemoveSecretCommand())
	r.Register(secrets.NewGrantSecretCommand())
	r.Register(secrets.NewRevokeSecretCommand())

	// Secret backends.
	r.Register(secretbackends.NewListSecretBackendsCommand())
	r.Register(secretbackends.NewAddSecretBackendCommand())
	r.Register(secretbackends.NewUpdateSecretBackendCommand())
	r.Register(secretbackends.NewRemoveSecretBackendCommand())
	r.Register(secretbackends.NewShowSecretBackendCommand())

	// Payload commands.
	r.Register(payload.NewListCommand())
	r.Register(waitfor.NewWaitForCommand())

	// Agreement commands
	r.Register(agree.NewAgreeCommand())
	r.Register(listagreements.NewListAgreementsCommand())
}

type cloudToCommandAdapter struct{}

func (cloudToCommandAdapter) ReadCloudData(path string) ([]byte, error) {
	return os.ReadFile(path)
}
func (cloudToCommandAdapter) ParseOneCloud(data []byte) (cloudfile.Cloud, error) {
	return cloudfile.ParseOneCloud(data)
}
func (cloudToCommandAdapter) PublicCloudMetadata(searchPaths ...string) (map[string]cloudfile.Cloud, bool, error) {
	return cloudfile.PublicCloudMetadata(searchPaths...)
}
func (cloudToCommandAdapter) PersonalCloudMetadata() (map[string]cloudfile.Cloud, error) {
	return cloudfile.PersonalCloudMetadata()
}
func (cloudToCommandAdapter) WritePersonalCloudMetadata(cloudsMap map[string]cloudfile.Cloud) error {
	return cloudfile.WritePersonalCloudMetadata(cloudsMap)
}
