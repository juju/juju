// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	utilsos "github.com/juju/os"
	"github.com/juju/os/series"
	proxyutils "github.com/juju/proxy"
	"github.com/juju/utils/featureflag"
	"github.com/juju/version"

	// Import the providers.
	cloudfile "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/caas"
	"github.com/juju/juju/cmd/juju/cachedimages"
	"github.com/juju/juju/cmd/juju/charmcmd"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/juju/crossmodel"
	"github.com/juju/juju/cmd/juju/firewall"
	"github.com/juju/juju/cmd/juju/gui"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/cmd/juju/metricsdebug"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/resource"
	rcmd "github.com/juju/juju/cmd/juju/romulus/commands"
	"github.com/juju/juju/cmd/juju/setmeterstatus"
	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/cmd/juju/subnet"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/utils/proxy"
	jujuversion "github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.cmd.juju.commands")

func init() {
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

// TODO(ericsnow) Move the following to cmd/juju/main.go:
//  jujuDoc
//  Main

var jujuDoc = `
Juju provides easy, intelligent application orchestration on top of cloud
infrastructure providers such as Amazon EC2, MaaS, OpenStack, Windows, Azure,
or your local machine.

See https://discourse.jujucharms.com/ for ideas, documentation and FAQ.
`

const juju1xCmdName = "juju-1"

var x = []byte("\x96\x8c\x8a\x91\x93\x9a\x9e\x8c\x97\x99\x8a\x9c\x94\x96\x91\x98\xdf\x9e\x92\x9e\x85\x96\x91\x98\xf5")

// Main registers subcommands for the juju executable, and hands over control
// to the cmd package. This function is not redundant with main, because it
// provides an entry point for testing with arbitrary command line arguments.
// This function returns the exit code, for main to pass to os.Exit.
func Main(args []string) int {
	return main{
		execCommand: exec.Command,
	}.Run(args)
}

// main is a type that captures dependencies for running the main function.
type main struct {
	// execCommand abstracts away exec.Command.
	execCommand func(command string, args ...string) *exec.Cmd
}

// Run is the main entry point for the juju client.
func (m main) Run(args []string) int {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		cmd.WriteError(os.Stderr, err)
		return 2
	}

	// note that this has to come before we init the juju home directory,
	// since it relies on detecting the lack of said directory.
	newInstall := m.maybeWarnJuju1x()

	if err = juju.InitJujuXDGDataHome(); err != nil {
		cmd.WriteError(ctx.Stderr, err)
		return 2
	}

	if err := installProxy(); err != nil {
		cmd.WriteError(ctx.Stderr, err)
		return 2
	}

	if newInstall {
		fmt.Fprintf(ctx.Stderr, "Since Juju %v is being run for the first time, downloading latest cloud information.\n", jujuversion.Current.Major)
		updateCmd := cloud.NewUpdatePublicCloudsCommand()
		if err := updateCmd.Run(ctx); err != nil {
			cmd.WriteError(ctx.Stderr, err)
		}
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

	jcmd := NewJujuCommand(ctx)
	return cmd.Main(jcmd, ctx, args[1:])
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

func (m main) maybeWarnJuju1x() (newInstall bool) {
	newInstall = !juju2xConfigDataExists()
	if !shouldWarnJuju1x() {
		return newInstall
	}
	ver, exists := m.juju1xVersion()
	if !exists {
		return newInstall
	}
	// TODO (anastasiamac 2016-10-21) Once manual page exists as per
	// https://github.com/juju/docs/issues/1487,
	// link it in the Note below to avoid propose here.
	welcomeMsgTemplate := `
Welcome to Juju {{.CurrentJujuVersion}}. 
    See https://jujucharms.com/docs/stable/introducing-2 for more details.

If you want to use Juju {{.OldJujuVersion}}, run 'juju' commands as '{{.OldJujuCommand}}'. For example, '{{.OldJujuCommand}} bootstrap'.
   See https://jujucharms.com/docs/stable/juju-coexist for installation details. 
`[1:]
	t := template.Must(template.New("plugin").Parse(welcomeMsgTemplate))
	var buf bytes.Buffer
	t.Execute(&buf, map[string]interface{}{
		"CurrentJujuVersion": jujuversion.Current,
		"OldJujuVersion":     ver,
		"OldJujuCommand":     juju1xCmdName,
	})
	fmt.Fprintln(os.Stderr, buf.String())
	return newInstall
}

func (m main) juju1xVersion() (ver string, exists bool) {
	out, err := m.execCommand(juju1xCmdName, "version").Output()
	if err == exec.ErrNotFound {
		return "", false
	}
	ver = "1.x"
	if err == nil {
		v := strings.TrimSpace(string(out))
		// parse so we can drop the series and arch
		bin, err := version.ParseBinary(v)
		if err == nil {
			ver = bin.Number.String()
		}
	}
	return ver, true
}

func shouldWarnJuju1x() bool {
	// this code only applies to Ubuntu, where we renamed Juju 1.x to juju-1.
	ostype, err := series.GetOSFromSeries(series.MustHostSeries())
	if err != nil || ostype != utilsos.Ubuntu {
		return false
	}
	return osenv.Juju1xEnvConfigExists() && !juju2xConfigDataExists()
}

func juju2xConfigDataExists() bool {
	_, err := os.Stat(osenv.JujuXDGDataHomeDir())
	return err == nil
}

// NewJujuCommand ...
func NewJujuCommand(ctx *cmd.Context) cmd.Command {
	var jcmd *cmd.SuperCommand
	jcmd = jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "juju",
		Doc:  jujuDoc,
		MissingCallback: RunPlugin(func(ctx *cmd.Context, subcommand string, args []string) error {
			if cmdName, _, ok := jcmd.FindClosestSubCommand(subcommand); ok {
				return &NotFoundCommand{
					ArgName: subcommand,
					CmdName: cmdName,
				}
			}
			return cmd.DefaultUnrecognizedCommand(subcommand)
		}),
		UserAliasesFilename: osenv.JujuXDGDataHomePath("aliases"),
		FlagKnownAs:         "option",
	})
	registerCommands(jcmd, ctx)
	return jcmd
}

const notFoundCommandMessage = `juju: %q is not a juju command. See "juju --help".

Did you mean:
	%s`

// NotFoundCommand gives valuable feedback to the operator about what commands
// could be available if a mistake around the subcommand name is given.
type NotFoundCommand struct {
	ArgName string
	CmdName string
}

func (c NotFoundCommand) Error() string {
	return fmt.Sprintf(notFoundCommandMessage, c.ArgName, c.CmdName)
}

type commandRegistry interface {
	Register(cmd.Command)
	RegisterSuperAlias(name, super, forName string, check cmd.DeprecationCheck)
	RegisterDeprecated(subcmd cmd.Command, check cmd.DeprecationCheck)
}

// TODO(ericsnow) Factor out the commands and aliases into a static
// registry that can be passed to the supercommand separately.

// registerCommands registers commands in the specified registry.
func registerCommands(r commandRegistry, ctx *cmd.Context) {
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
	if !featureflag.Enabled(feature.JujuV3) {
		r.Register(newDefaultRunCommand(nil))
	}
	r.Register(newDefaultExecCommand(nil))
	r.Register(newSCPCommand(nil))
	r.Register(newSSHCommand(nil, nil))
	r.Register(application.NewResolvedCommand())
	r.Register(newDebugLogCommand(nil))
	r.Register(newDebugHooksCommand(nil))

	// Configuration commands.
	r.Register(model.NewModelGetConstraintsCommand())
	r.Register(model.NewModelSetConstraintsCommand())
	r.Register(newSyncToolsCommand())
	r.Register(newUpgradeJujuCommand())
	r.Register(newUpgradeControllerCommand())
	r.Register(application.NewUpgradeCharmCommand())
	r.Register(application.NewSetSeriesCommand())
	r.Register(application.NewBindCommand())

	// Charm tool commands.
	r.Register(newHelpToolCommand())
	// TODO (anastasiamac 2017-08-1) This needs to be removed in Juju 3.x
	// lp#1707836
	r.Register(charmcmd.NewSuperCommand())

	// Manage backups.
	r.Register(backups.NewCreateCommand())
	r.Register(backups.NewDownloadCommand())
	r.Register(backups.NewShowCommand())
	r.Register(backups.NewListCommand())
	r.Register(backups.NewRemoveCommand())
	r.Register(backups.NewRestoreCommand())
	r.Register(backups.NewUploadCommand())

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

	// Manage cached images
	r.Register(cachedimages.NewRemoveCommand())
	r.Register(cachedimages.NewListCommand())

	// Manage machines
	r.Register(machine.NewAddCommand())
	r.Register(machine.NewRemoveCommand())
	r.Register(machine.NewListMachinesCommand())
	r.Register(machine.NewShowMachineCommand())
	r.Register(machine.NewUpgradeSeriesCommand())

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
	if featureflag.Enabled(feature.JujuV3) {
		r.Register(action.NewCallCommand())
		r.Register(action.NewListOperationsCommand())
		r.Register(action.NewShowOperationCommand())
	} else {
		r.Register(action.NewRunActionCommand())
		r.Register(action.NewShowActionOutputCommand())
		r.Register(action.NewStatusCommand())
	}

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
	r.Register(application.NewBundleDiffCommand())
	r.Register(application.NewShowApplicationCommand())

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
	r.Register(space.NewReloadCommand())
	if featureflag.Enabled(feature.PostNetCLIMVP) {
		r.Register(space.NewRemoveCommand())
		r.Register(space.NewUpdateCommand())
		r.Register(space.NewRenameCommand())
	}

	// Manage subnets
	r.Register(subnet.NewAddCommand())
	r.Register(subnet.NewListCommand())
	if featureflag.Enabled(feature.PostNetCLIMVP) {
		r.Register(subnet.NewCreateCommand())
		r.Register(subnet.NewRemoveCommand())
	}

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
	r.Register(caas.NewRemoveCAASCommand(&cloudToCommandAdapter{}))
	r.Register(application.NewScaleApplicationCommand())

	// Manage Application Credential Access
	r.Register(application.NewTrustCommand())

	// Juju GUI commands.
	r.Register(gui.NewGUICommand())
	r.Register(gui.NewUpgradeGUICommand())

	// Resource commands
	r.Register(resource.NewUploadCommand(resource.UploadDeps{
		NewClient: func(c *resource.UploadCommand) (resource.UploadClient, error) {
			apiRoot, err := c.NewAPIRoot()
			if err != nil {
				return nil, errors.Trace(err)
			}
			return resourceadapters.NewAPIClient(apiRoot)
		},
		OpenResource: func(s string) (resource.ReadSeekCloser, error) {
			return os.Open(s)
		},
	}))
	r.Register(resource.NewListCommand(resource.ListDeps{
		NewClient: func(c *resource.ListCommand) (resource.ListClient, error) {
			apiRoot, err := c.NewAPIRoot()
			if err != nil {
				return nil, errors.Trace(err)
			}
			return resourceadapters.NewAPIClient(apiRoot)
		},
	}))
	r.Register(resource.NewCharmResourcesCommand(nil))

	// Commands registered elsewhere.
	for _, newCommand := range registeredCommands {
		command := newCommand()
		r.Register(command)
	}
	for _, newCommand := range registeredEnvCommands {
		command := newCommand()
		r.Register(modelcmd.Wrap(command))
	}
	rcmd.RegisterAll(r)
}

type cloudToCommandAdapter struct{}

func (cloudToCommandAdapter) ParseCloudMetadataFile(path string) (map[string]cloudfile.Cloud, error) {
	return cloudfile.ParseCloudMetadataFile(path)
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
