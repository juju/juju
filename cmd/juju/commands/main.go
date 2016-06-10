// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	rcmd "github.com/juju/romulus/cmd/commands"
	"github.com/juju/utils/featureflag"
	utilsos "github.com/juju/utils/os"
	"github.com/juju/utils/series"
	"github.com/juju/version"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/cachedimages"
	"github.com/juju/juju/cmd/juju/charmcmd"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/juju/gui"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/cmd/juju/metricsdebug"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/service"
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
	jujuversion "github.com/juju/juju/version"
	// Import the providers.
	_ "github.com/juju/juju/provider/all"
)

var logger = loggo.GetLogger("juju.cmd.juju.commands")

func init() {
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

// TODO(ericsnow) Move the following to cmd/juju/main.go:
//  jujuDoc
//  Main

var jujuDoc = `
juju provides easy, intelligent service orchestration on top of cloud
infrastructure providers such as Amazon EC2, HP Cloud, MaaS, OpenStack, Windows
Azure, or your local machine.

https://juju.ubuntu.com/
`

const juju1xCmdName = "juju-1"

var usageHelp = `
Usage: juju [help] <command>

Summary:
Juju is model & service management software designed to leverage the power
of existing resource pools, particularly cloud-based ones. It has built-in
support for cloud providers such as Amazon EC2, Google GCE, Microsoft
Azure, OpenStack, and Rackspace. It also works very well with MAAS and
LXD. Juju allows for easy installation and management of workloads on a
chosen resource pool.

See https://jujucharms.com/docs/stable/help for documentation.

Common commands:

    add-cloud           Adds a user-defined cloud to Juju.
    add-credential      Adds or replaces credentials for a cloud.
    add-relation        Adds a relation between two services.
    add-unit            Adds extra units of a deployed service.
    add-user            Adds a Juju user to a controller.
    bootstrap           Initializes a cloud environment.
    add-model           Adds a hosted model.
    deploy              Deploys a new service.
    expose              Makes a service publicly available over the network.
    list-controllers    Lists all controllers.
    list-models         Lists models a user can access on a controller.
    status              Displays the current status of Juju, services, and units.
    switch              Selects or identifies the current controller and model.

Example help commands:

    `[1:] + "`juju help`" + `          This help page
    ` + "`juju help commands`" + ` Lists all commands
    ` + "`juju help deploy`" + `   Shows help for command 'deploy'
`

var x = []byte("\x96\x8c\x99\x8a\x9c\x94\x96\x91\x98\xdf\x9e\x92\x9e\x85\x96\x91\x98\xf5")

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
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	// note that this has to come before we init the juju home directory,
	// since it relies on detecting the lack of said directory.
	m.maybeWarnJuju1x()

	if err = juju.InitJujuXDGDataHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 2
	}

	for i := range x {
		x[i] ^= 255
	}
	if len(args) == 2 && args[1] == string(x[0:2]) {
		os.Stdout.Write(x[2:])
		return 0
	}

	jcmd := NewJujuCommand(ctx)
	return cmd.Main(jcmd, ctx, args[1:])
}

func (m main) maybeWarnJuju1x() {
	if !shouldWarnJuju1x() {
		return
	}
	ver, exists := m.juju1xVersion()
	if !exists {
		return
	}
	fmt.Fprintf(os.Stderr, `
    Welcome to Juju %s. If you meant to use Juju %s you can continue using it
    with the command %s e.g. '%s switch'.
    See https://jujucharms.com/docs/stable/introducing-2 for more details.
`[1:], jujuversion.Current, ver, juju1xCmdName, juju1xCmdName)
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
	ostype, err := series.GetOSFromSeries(series.HostSeries())
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
	jcmd := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:                "juju",
		Doc:                 jujuDoc,
		MissingCallback:     RunPlugin,
		UserAliasesFilename: osenv.JujuXDGDataHomePath("aliases"),
	})
	jcmd.AddHelpTopic("basics", "Basic Help Summary", usageHelp)
	registerCommands(jcmd, ctx)
	return jcmd
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
	r.Register(service.NewAddRelationCommand())

	// Destruction commands.
	r.Register(service.NewRemoveRelationCommand())
	r.Register(service.NewRemoveServiceCommand())
	r.Register(service.NewRemoveUnitCommand())

	// Reporting commands.
	r.Register(status.NewStatusCommand())
	r.Register(newSwitchCommand())
	r.Register(status.NewStatusHistoryCommand())

	// Error resolution and debugging commands.
	r.Register(newRunCommand())
	r.Register(newSCPCommand())
	r.Register(newSSHCommand())
	r.Register(newResolvedCommand())
	r.Register(newDebugLogCommand())
	r.Register(newDebugHooksCommand())

	// Configuration commands.
	r.Register(model.NewModelGetConstraintsCommand())
	r.Register(model.NewModelSetConstraintsCommand())
	r.Register(newSyncToolsCommand())
	r.Register(newUpgradeJujuCommand(nil))
	r.Register(service.NewUpgradeCharmCommand())

	// Charm publishing commands.
	r.Register(newPublishCommand())

	// Charm tool commands.
	r.Register(newHelpToolCommand())
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

	// Manage cached images
	r.Register(cachedimages.NewRemoveCommand())
	r.Register(cachedimages.NewListCommand())

	// Manage machines
	r.Register(machine.NewAddCommand())
	r.Register(machine.NewRemoveCommand())
	r.Register(machine.NewListMachinesCommand())
	r.Register(machine.NewShowMachineCommand())

	// Manage model
	r.Register(model.NewGetCommand())
	r.Register(model.NewSetCommand())
	r.Register(model.NewUnsetCommand())
	r.Register(model.NewRetryProvisioningCommand())
	r.Register(model.NewDestroyCommand())
	r.Register(model.NewUsersCommand())
	r.Register(model.NewGrantCommand())
	r.Register(model.NewRevokeCommand())
	r.Register(model.NewShowCommand())

	if featureflag.Enabled(feature.Migration) {
		r.Register(newMigrateCommand())
	}

	// Manage and control actions
	r.Register(action.NewStatusCommand())
	r.Register(action.NewRunCommand())
	r.Register(action.NewShowOutputCommand())
	r.Register(action.NewListCommand())

	// Manage controller availability
	r.Register(newEnableHACommand())

	// Manage and control services
	r.Register(service.NewAddUnitCommand())
	r.Register(service.NewGetCommand())
	r.Register(service.NewSetCommand())
	r.Register(service.NewDeployCommand())
	r.Register(service.NewExposeCommand())
	r.Register(service.NewUnexposeCommand())
	r.Register(service.NewServiceGetConstraintsCommand())
	r.Register(service.NewServiceSetConstraintsCommand())

	// Operation protection commands
	r.Register(block.NewSuperBlockCommand())
	r.Register(block.NewUnblockCommand())

	// Manage storage
	r.Register(storage.NewAddCommand())
	r.Register(storage.NewListCommand())
	r.Register(storage.NewPoolCreateCommand())
	r.Register(storage.NewPoolListCommand())
	r.Register(storage.NewShowCommand())

	// Manage spaces
	r.Register(space.NewAddCommand())
	r.Register(space.NewListCommand())
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
	r.Register(controller.NewListBlocksCommand())
	r.Register(controller.NewRegisterCommand())
	r.Register(controller.NewUnregisterCommand(jujuclient.NewFileClientStore()))
	r.Register(controller.NewRemoveBlocksCommand())
	r.Register(controller.NewShowControllerCommand())

	// Debug Metrics
	r.Register(metricsdebug.New())
	r.Register(metricsdebug.NewCollectMetricsCommand())
	r.Register(setmeterstatus.New())

	// Manage clouds and credentials
	r.Register(cloud.NewUpdateCloudsCommand())
	r.Register(cloud.NewListCloudsCommand())
	r.Register(cloud.NewShowCloudCommand())
	r.Register(cloud.NewAddCloudCommand())
	r.Register(cloud.NewListCredentialsCommand())
	r.Register(cloud.NewDetectCredentialsCommand())
	r.Register(cloud.NewSetDefaultRegionCommand())
	r.Register(cloud.NewSetDefaultCredentialCommand())
	r.Register(cloud.NewAddCredentialCommand())
	r.Register(cloud.NewRemoveCredentialCommand())

	// Juju GUI commands.
	r.Register(gui.NewGUICommand())
	r.Register(gui.NewUpgradeGUICommand())

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
