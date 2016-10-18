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
	"github.com/juju/utils/featureflag"
	utilsos "github.com/juju/utils/os"
	"github.com/juju/utils/series"
	"github.com/juju/version"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/cachedimages"
	"github.com/juju/juju/cmd/juju/charmcmd"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/juju/crossmodel"
	"github.com/juju/juju/cmd/juju/gui"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/cmd/juju/metricsdebug"
	"github.com/juju/juju/cmd/juju/model"
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
juju provides easy, intelligent application orchestration on top of cloud
infrastructure providers such as Amazon EC2, MaaS, OpenStack, Windows, Azure,
or your local machine.

https://jujucharms.com/
`

const juju1xCmdName = "juju-1"

var usageHelp = `
Usage: juju [help] <command>

Summary:
Juju is model & application management software designed to leverage the power
of existing resource pools, particularly cloud-based ones. It has built-in
support for cloud providers such as Amazon EC2, Google GCE, Microsoft
Azure, OpenStack, and Rackspace. It also works very well with MAAS and
LXD. Juju allows for easy installation and management of workloads on a
chosen resource pool.

See https://jujucharms.com/docs/stable/help for documentation.

Common commands:

	add-cloud           Adds a user-defined cloud to Juju.
	add-credential      Adds or replaces credentials for a cloud.
	add-model           Adds a hosted model.
	add-relation        Adds a relation between two applications.
	add-unit            Adds extra units of a deployed application.
	add-user            Adds a Juju user to a controller.
	bootstrap           Initializes a cloud environment.
	controllers         Lists all controllers.
	deploy              Deploys a new application.
	expose              Makes an application publicly available over the network.
	models              Lists models a user can access on a controller.
	status              Displays the current status of Juju, applications, and units.
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

	firstRun := juju2xConfigDataExists() == false

	// note that this has to come before we init the juju home directory,
	// since it relies on detecting the lack of said directory.
	if firstRun && shouldWarnJuju1x() {
		m.warnJuju1x()
	}

	if err = juju.InitJujuXDGDataHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 2
	}

	if firstRun {
		fmt.Fprintf(
			ctx.Stderr,
			"Since Juju %v is being run for the first time, downloading latest cloud information.\n",
			jujuversion.Current.Major,
		)
		updateCmd := cloud.NewUpdateCloudsCommand()
		if err := updateCmd.Run(ctx); err != nil {
			fmt.Fprintf(ctx.Stderr, "error: %v\n", err)
		}
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

func (m main) warnJuju1x() {
	ver, exists := m.juju1xVersion()
	if !exists {
		return
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

	seriesName, err := series.HostSeries()
	if err != nil {
		// This is a non-critical error. The inability to determine
		// the series of the machine running the Juju command does not
		// preclude people from actually using Juju.
		logger.Warningf("%v", errors.Annotatef(err, "cannot determine whether to warn about Juju 1.x"))
		return false
	}
	ostype, err := series.GetOSFromSeries(seriesName)
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
	r.Register(application.NewAddRelationCommand())

	if featureflag.Enabled(feature.CrossModelRelations) {
		r.Register(crossmodel.NewOfferCommand())
		r.Register(crossmodel.NewShowOfferedEndpointCommand())
		r.Register(crossmodel.NewListEndpointsCommand())
		r.Register(crossmodel.NewFindEndpointsCommand())
	}

	// Destruction commands.
	r.Register(application.NewRemoveRelationCommand())
	r.Register(application.NewRemoveServiceCommand())
	r.Register(application.NewRemoveUnitCommand())

	// Reporting commands.
	r.Register(status.NewStatusCommand())
	r.Register(newSwitchCommand())
	r.Register(status.NewStatusHistoryCommand())

	// Error resolution and debugging commands.
	r.Register(newRunCommand())
	r.Register(newSCPCommand(nil))
	r.Register(newSSHCommand(nil))
	r.Register(newResolvedCommand())
	r.Register(newDebugLogCommand())
	r.Register(newDebugHooksCommand(nil))

	// Configuration commands.
	r.Register(model.NewModelGetConstraintsCommand())
	r.Register(model.NewModelSetConstraintsCommand())
	r.Register(newSyncToolsCommand())
	r.Register(newUpgradeJujuCommand(nil))
	r.Register(application.NewUpgradeCharmCommand())

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

	// Manage model
	r.Register(model.NewConfigCommand())
	r.Register(model.NewDefaultsCommand())
	r.Register(model.NewRetryProvisioningCommand())
	r.Register(model.NewDestroyCommand())
	r.Register(model.NewGrantCommand())
	r.Register(model.NewRevokeCommand())
	r.Register(model.NewShowCommand())

	if featureflag.Enabled(feature.Migration) {
		r.Register(newMigrateCommand())
	}
	if featureflag.Enabled(feature.DeveloperMode) {
		r.Register(model.NewDumpCommand())
		r.Register(model.NewDumpDBCommand())
	}

	// Manage and control actions
	r.Register(action.NewStatusCommand())
	r.Register(action.NewRunCommand())
	r.Register(action.NewShowOutputCommand())
	r.Register(action.NewListCommand())

	// Manage controller availability
	r.Register(newEnableHACommand())

	// Manage and control services
	r.Register(application.NewAddUnitCommand())
	r.Register(application.NewConfigCommand())
	r.Register(application.NewDefaultDeployCommand())
	r.Register(application.NewExposeCommand())
	r.Register(application.NewUnexposeCommand())
	r.Register(application.NewServiceGetConstraintsCommand())
	r.Register(application.NewServiceSetConstraintsCommand())

	// Operation protection commands
	r.Register(block.NewDisableCommand())
	r.Register(block.NewListCommand())
	r.Register(block.NewEnableCommand())

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
	r.Register(controller.NewRegisterCommand())
	r.Register(controller.NewUnregisterCommand(jujuclient.NewFileClientStore()))
	r.Register(controller.NewEnableDestroyControllerCommand())
	r.Register(controller.NewShowControllerCommand())
	r.Register(controller.NewGetConfigCommand())

	// Debug Metrics
	r.Register(metricsdebug.New())
	r.Register(metricsdebug.NewCollectMetricsCommand())
	r.Register(setmeterstatus.New())

	// Manage clouds and credentials
	r.Register(cloud.NewUpdateCloudsCommand())
	r.Register(cloud.NewListCloudsCommand())
	r.Register(cloud.NewListRegionsCommand())
	r.Register(cloud.NewShowCloudCommand())
	r.Register(cloud.NewAddCloudCommand())
	r.Register(cloud.NewRemoveCloudCommand())
	r.Register(cloud.NewListCredentialsCommand())
	r.Register(cloud.NewDetectCredentialsCommand())
	r.Register(cloud.NewSetDefaultRegionCommand())
	r.Register(cloud.NewSetDefaultCredentialCommand())
	r.Register(cloud.NewAddCredentialCommand())
	r.Register(cloud.NewRemoveCredentialCommand())
	r.Register(cloud.NewUpdateCredentialCommand())

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
