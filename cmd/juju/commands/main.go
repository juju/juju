// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/utils/featureflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/cachedimages"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/cmd/juju/helptopics"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/cmd/juju/service"
	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/cmd/juju/subnet"
	"github.com/juju/juju/cmd/juju/system"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
	// Import the providers.
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/version"
)

func init() {
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

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
	if err = juju.InitJujuHome(); err != nil {
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
		Name:            "juju",
		Doc:             jujuDoc,
		MissingCallback: RunPlugin,
	})
	jcmd.AddHelpTopic("basics", "Basic commands", helptopics.Basics)
	jcmd.AddHelpTopic("local-provider", "How to configure a local (LXC) provider",
		helptopics.LocalProvider)
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
	jcmd.AddHelpTopic("glossary", "Glossary of terms", helptopics.Glossary)
	jcmd.AddHelpTopic("logging", "How Juju handles logging", helptopics.Logging)
	jcmd.AddHelpTopic("juju", "What is Juju?", helptopics.Juju)
	jcmd.AddHelpTopic("juju-systems", "About Juju Environment Systems (JES)", helptopics.JujuSystems)
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

// registerCommands registers commands in the specified registry.
// EnvironCommands must be wrapped with an envCmdWrapper.
func registerCommands(r commandRegistry, ctx *cmd.Context) {
	wrapEnvCommand := func(c envcmd.EnvironCommand) cmd.Command {
		return envCmdWrapper{envcmd.Wrap(c), ctx}
	}

	// Creation commands.
	r.Register(wrapEnvCommand(&BootstrapCommand{}))
	r.Register(wrapEnvCommand(&DeployCommand{}))
	r.Register(wrapEnvCommand(&AddRelationCommand{}))

	// Destruction commands.
	r.Register(wrapEnvCommand(&RemoveRelationCommand{}))
	r.Register(wrapEnvCommand(&RemoveServiceCommand{}))
	r.Register(wrapEnvCommand(&RemoveUnitCommand{}))
	r.Register(&DestroyEnvironmentCommand{})

	// Reporting commands.
	r.Register(wrapEnvCommand(&status.StatusCommand{}))
	r.Register(&SwitchCommand{})
	r.Register(wrapEnvCommand(&EndpointCommand{}))
	r.Register(wrapEnvCommand(&APIInfoCommand{}))
	r.Register(wrapEnvCommand(&status.StatusHistoryCommand{}))

	// Error resolution and debugging commands.
	r.Register(wrapEnvCommand(&RunCommand{}))
	r.Register(wrapEnvCommand(&SCPCommand{}))
	r.Register(wrapEnvCommand(&SSHCommand{}))
	r.Register(wrapEnvCommand(&ResolvedCommand{}))
	r.Register(wrapEnvCommand(&DebugLogCommand{}))
	r.Register(wrapEnvCommand(&DebugHooksCommand{}))

	// Configuration commands.
	r.Register(&InitCommand{})
	r.RegisterDeprecated(wrapEnvCommand(&common.GetConstraintsCommand{}),
		twoDotOhDeprecation("environment get-constraints or service get-constraints"))
	r.RegisterDeprecated(wrapEnvCommand(&common.SetConstraintsCommand{}),
		twoDotOhDeprecation("environment set-constraints or service set-constraints"))
	r.Register(wrapEnvCommand(&ExposeCommand{}))
	r.Register(wrapEnvCommand(&SyncToolsCommand{}))
	r.Register(wrapEnvCommand(&UnexposeCommand{}))
	r.Register(wrapEnvCommand(&UpgradeJujuCommand{}))
	r.Register(wrapEnvCommand(&UpgradeCharmCommand{}))

	// Charm publishing commands.
	r.Register(wrapEnvCommand(&PublishCommand{}))

	// Charm tool commands.
	r.Register(&HelpToolCommand{})

	// Manage backups.
	r.Register(backups.NewCommand())

	// Manage authorized ssh keys.
	r.Register(NewAuthorizedKeysCommand())

	// Manage users and access
	r.Register(user.NewSuperCommand())

	// Manage cached images
	r.Register(cachedimages.NewSuperCommand())

	// Manage machines
	r.Register(machine.NewSuperCommand())
	r.RegisterSuperAlias("add-machine", "machine", "add", twoDotOhDeprecation("machine add"))
	r.RegisterSuperAlias("remove-machine", "machine", "remove", twoDotOhDeprecation("machine remove"))
	r.RegisterSuperAlias("destroy-machine", "machine", "remove", twoDotOhDeprecation("machine remove"))
	r.RegisterSuperAlias("terminate-machine", "machine", "remove", twoDotOhDeprecation("machine remove"))

	// Mangage environment
	r.Register(environment.NewSuperCommand())
	r.RegisterSuperAlias("get-environment", "environment", "get", twoDotOhDeprecation("environment get"))
	r.RegisterSuperAlias("get-env", "environment", "get", twoDotOhDeprecation("environment get"))
	r.RegisterSuperAlias("set-environment", "environment", "set", twoDotOhDeprecation("environment set"))
	r.RegisterSuperAlias("set-env", "environment", "set", twoDotOhDeprecation("environment set"))
	r.RegisterSuperAlias("unset-environment", "environment", "unset", twoDotOhDeprecation("environment unset"))
	r.RegisterSuperAlias("unset-env", "environment", "unset", twoDotOhDeprecation("environment unset"))
	r.RegisterSuperAlias("retry-provisioning", "environment", "retry-provisioning", twoDotOhDeprecation("environment retry-provisioning"))

	// Manage and control actions
	r.Register(action.NewSuperCommand())

	// Manage state server availability
	r.Register(wrapEnvCommand(&EnsureAvailabilityCommand{}))

	// Manage and control services
	r.Register(service.NewSuperCommand())
	r.RegisterSuperAlias("add-unit", "service", "add-unit", twoDotOhDeprecation("service add-unit"))
	r.RegisterSuperAlias("get", "service", "get", twoDotOhDeprecation("service get"))
	r.RegisterSuperAlias("set", "service", "set", twoDotOhDeprecation("service set"))
	r.RegisterSuperAlias("unset", "service", "unset", twoDotOhDeprecation("service unset"))

	// Operation protection commands
	r.Register(block.NewSuperBlockCommand())
	r.Register(wrapEnvCommand(&block.UnblockCommand{}))

	// Manage storage
	r.Register(storage.NewSuperCommand())

	// Manage spaces
	r.Register(space.NewSuperCommand())

	// Manage subnets
	r.Register(subnet.NewSuperCommand())

	// Manage systems
	if featureflag.Enabled(feature.JES) {
		r.Register(system.NewSuperCommand())
		r.RegisterSuperAlias("systems", "system", "list", nil)

		// Add top level aliases of the same name as the subcommands.
		r.RegisterSuperAlias("environments", "system", "environments", nil)
		r.RegisterSuperAlias("login", "system", "login", nil)
		r.RegisterSuperAlias("create-environment", "system", "create-environment", nil)
		r.RegisterSuperAlias("create-env", "system", "create-env", nil)
	}
}

// envCmdWrapper is a struct that wraps an environment command and lets us handle
// errors returned from Init before they're returned to the main function.
type envCmdWrapper struct {
	cmd.Command
	ctx *cmd.Context
}

func (w envCmdWrapper) Init(args []string) error {
	err := w.Command.Init(args)
	if environs.IsNoEnv(err) {
		fmt.Fprintln(w.ctx.Stderr, "No juju environment configuration file exists.")
		fmt.Fprintln(w.ctx.Stderr, err)
		fmt.Fprintln(w.ctx.Stderr, "Please create a configuration by running:")
		fmt.Fprintln(w.ctx.Stderr, "    juju init")
		fmt.Fprintln(w.ctx.Stderr, "then edit the file to configure your juju environment.")
		fmt.Fprintln(w.ctx.Stderr, "You can then re-run the command.")
		return cmd.ErrSilent
	}
	return err
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
	if version.Current.Number.Compare(v.deprecate) > 0 {
		return true, v.replacement
	}
	return false, ""
}

// Obsolete implements cmd.DeprecationCheck.
// If the current version is after the obsolete version number,
// the command is obsolete and shouldn't be registered.
func (v *versionDeprecation) Obsolete() bool {
	return version.Current.Number.Compare(v.obsolete) > 0
}

func twoDotOhDeprecation(replacement string) cmd.DeprecationCheck {
	return &versionDeprecation{
		replacement: replacement,
		deprecate:   version.MustParse("2.0-00"),
		obsolete:    version.MustParse("3.0-00"),
	}
}
