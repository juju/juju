// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	rcmd "github.com/juju/romulus/cmd/commands"
	"github.com/juju/utils/featureflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/cachedimages"
	"github.com/juju/juju/cmd/juju/charmcmd"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/juju/helptopics"
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
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
	// Import the providers.
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/version"
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

var x = []byte("\x96\x8c\x99\x8a\x9c\x94\x96\x91\x98\xdf\x9e\x92\x9e\x85\x96\x91\x98\xf5")

// Main registers subcommands for the juju executable, and hands over control
// to the cmd package. This function is not redundant with main, because it
// provides an entry point for testing with arbitrary command line arguments.
func Main(args []string) {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	if err = juju.InitJujuXDGDataHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	for i := range x {
		x[i] ^= 255
	}
	if len(args) == 2 && args[1] == string(x[0:2]) {
		os.Stdout.Write(x[2:])
		os.Exit(0)
	}
	jcmd := NewJujuCommand(ctx)
	os.Exit(cmd.Main(jcmd, ctx, args[1:]))
}

func NewJujuCommand(ctx *cmd.Context) cmd.Command {
	jcmd := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:                "juju",
		Doc:                 jujuDoc,
		MissingCallback:     RunPlugin,
		UserAliasesFilename: osenv.JujuXDGDataHomePath("aliases"),
	})
	jcmd.AddHelpTopic("basics", "Basic commands", helptopics.Basics)
	jcmd.AddHelpTopic("openstack-provider", "How to configure an OpenStack provider",
		helptopics.OpenstackProvider, "openstack")
	jcmd.AddHelpTopic("ec2-provider", "How to configure an Amazon EC2 provider",
		helptopics.EC2Provider, "ec2", "aws", "amazon")
	jcmd.AddHelpTopic("hpcloud-provider", "How to configure an HP Cloud provider",
		helptopics.HPCloud, "hpcloud", "hp-cloud")
	jcmd.AddHelpTopic("azure-provider", "How to configure a Windows Azure provider",
		helptopics.AzureProvider, "azure")
	jcmd.AddHelpTopic("maas-provider", "How to configure a MAAS provider",
		helptopics.MAASProvider, "maas")
	jcmd.AddHelpTopic("constraints", "How to use commands with constraints", helptopics.Constraints)
	jcmd.AddHelpTopic("placement", "How to use placement directives", helptopics.Placement)
	jcmd.AddHelpTopic("spaces", "How to configure more complex networks using spaces", helptopics.Spaces, "networking")
	jcmd.AddHelpTopic("glossary", "Glossary of terms", helptopics.Glossary)
	jcmd.AddHelpTopic("logging", "How Juju handles logging", helptopics.Logging)
	jcmd.AddHelpTopic("juju", "What is Juju?", helptopics.Juju)
	jcmd.AddHelpTopic("controllers", "About Juju Controllers", helptopics.JujuControllers)
	jcmd.AddHelpTopic("users", "About users in Juju", helptopics.Users)
	jcmd.AddHelpTopicCallback("plugins", "Show Juju plugins", PluginHelpTopic)

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
	r.Register(backups.NewSuperCommand())
	r.RegisterSuperAlias("create-backup", "backups", "create", nil)
	r.RegisterSuperAlias("restore-backup", "backups", "restore", nil)

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
	r.Register(user.NewSwitchUserCommand())

	// Manage cached images
	r.Register(cachedimages.NewSuperCommand())

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

	r.Register(model.NewShareCommand())
	r.Register(model.NewUnshareCommand())
	r.Register(model.NewUsersCommand())

	// Manage and control actions
	r.Register(action.NewSuperCommand())
	r.RegisterSuperAlias("run-action", "action", "do", nil)
	r.RegisterSuperAlias("list-actions", "action", "defined", nil)
	r.RegisterSuperAlias("show-action-output", "action", "fetch", nil)
	r.RegisterSuperAlias("show-action-status", "action", "status", nil)

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
	r.Register(storage.NewSuperCommand())
	r.RegisterSuperAlias("list-storage", "storage", "list", nil)
	r.RegisterSuperAlias("show-storage", "storage", "show", nil)
	r.RegisterSuperAlias("add-storage", "storage", "add", nil)

	// Manage spaces
	r.Register(space.NewSuperCommand())
	r.RegisterSuperAlias("add-space", "space", "create", nil)
	r.RegisterSuperAlias("list-spaces", "space", "list", nil)

	// Manage subnets
	r.Register(subnet.NewSuperCommand())
	r.RegisterSuperAlias("add-subnet", "subnet", "add", nil)

	// Manage controllers
	r.Register(controller.NewCreateModelCommand())
	r.Register(controller.NewDestroyCommand())
	r.Register(controller.NewListModelsCommand())
	r.Register(controller.NewKillCommand())
	r.Register(controller.NewListControllersCommand())
	r.Register(controller.NewListBlocksCommand())
	r.Register(controller.NewRegisterCommand())
	r.Register(controller.NewRemoveBlocksCommand())
	r.Register(controller.NewShowControllerCommand())

	// Debug Metrics
	r.Register(metricsdebug.New())
	r.Register(metricsdebug.NewCollectMetricsCommand())
	r.Register(setmeterstatus.New())

	// Manage clouds and credentials
	r.Register(cloud.NewListCloudsCommand())
	r.Register(cloud.NewShowCloudCommand())
	r.Register(cloud.NewAddCloudCommand())
	r.Register(cloud.NewListCredentialsCommand())
	r.Register(cloud.NewDetectCredentialsCommand())

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

func main() {
	Main(os.Args)
}

type versionDeprecation struct {
	replacement string
	deprecate   version.Number
	obsolete    version.Number
}

// Deprecated implements cmd.DeprecationCheck.
// If the current version is after the deprecate version number,
// the command is deprecated and the replacement should be used.
func (v *versionDeprecation) Deprecated() (bool, string) {
	if version.Current.Compare(v.deprecate) > 0 {
		return true, v.replacement
	}
	return false, ""
}

// Obsolete implements cmd.DeprecationCheck.
// If the current version is after the obsolete version number,
// the command is obsolete and shouldn't be registered.
func (v *versionDeprecation) Obsolete() bool {
	return version.Current.Compare(v.obsolete) > 0
}

func twoDotOhDeprecation(replacement string) cmd.DeprecationCheck {
	return &versionDeprecation{
		replacement: replacement,
		deprecate:   version.MustParse("2.0-00"),
		obsolete:    version.MustParse("3.0-00"),
	}
}
