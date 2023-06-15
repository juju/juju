// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/juju/charm/v11"
	"github.com/juju/charm/v11/hooks"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/retry"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/charms"
	apicharm "github.com/juju/juju/api/common/charm"
	charmscommon "github.com/juju/juju/api/common/charms"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/network/ssh"
	unitdebug "github.com/juju/juju/worker/uniter/runner/debug"
)

func NewDebugHooksCommand(hostChecker ssh.ReachableChecker, retryStrategy retry.CallArgs) cmd.Command {
	c := new(debugHooksCommand)
	c.hostChecker = hostChecker
	c.retryStrategy = retryStrategy
	return modelcmd.Wrap(c)
}

// debugHooksCommand is responsible for launching a ssh shell on a given unit or machine.
type debugHooksCommand struct {
	sshCommand
	hooks []string

	applicationAPI
	charmAPI
}

const debugHooksDoc = `
Interactively debug hooks or actions remotely on an application unit.

Valid unit identifiers are:
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

See the "juju help ssh" for information about SSH related options
accepted by the debug-hooks command.
`

func (c *debugHooksCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "debug-hooks",
		Args:    "<unit name> [hook or action names]",
		Purpose: "Launch a tmux session to debug hooks and/or actions.",
		Doc:     debugHooksDoc,
		Aliases: []string{"debug-hook"},
	})
}

func (c *debugHooksCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("no unit name specified")
	}
	if err := c.sshCommand.Init(args); err != nil {
		return err
	}

	c.provider.setTarget(args[0])
	if target := c.provider.getTarget(); !(names.IsValidUnit(target) || strings.HasSuffix(target, "/leader")) {
		return errors.Errorf("%q is not a valid unit name", c.provider.getTarget())
	}

	// If any of the hooks is "*", then debug all hooks.
	c.hooks = append([]string{}, args[1:]...)
	for _, h := range c.hooks {
		if h == "*" {
			c.hooks = nil
			break
		}
	}
	return nil
}

type applicationAPI interface {
	GetCharmURLOrigin(branchName, applicationName string) (*charm.URL, apicharm.Origin, error)
	Leader(string) (string, error)
	Close() error
}

type charmAPI interface {
	CharmInfo(charmURL string) (*charmscommon.CharmInfo, error)
	Close() error
}

func (c *debugHooksCommand) initAPIs() (err error) {
	defer func() {
		c.provider.setLeaderAPI(c.applicationAPI)
	}()

	if c.charmAPI != nil && c.applicationAPI != nil {
		return nil
	}

	root, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}

	if c.applicationAPI == nil {
		c.applicationAPI = application.NewClient(root)
	}
	if c.charmAPI == nil {
		c.charmAPI = charms.NewClient(root)
	}
	return nil
}

func (c *debugHooksCommand) closeAPIs() {
	if c.applicationAPI != nil {
		_ = c.applicationAPI.Close()
		c.applicationAPI = nil
	}
	if c.charmAPI != nil {
		_ = c.charmAPI.Close()
		c.charmAPI = nil
	}
}

func (c *debugHooksCommand) validateHooksOrActions() error {
	if len(c.hooks) == 0 {
		return nil
	}

	var (
		appName string
		err     error
	)

	// If the unit/leader syntax is used, we need to manually extract the
	// application name from the target parameter.
	target := c.provider.getTarget()
	if strings.HasSuffix(target, "/leader") {
		appName = strings.TrimSuffix(target, "/leader")
	} else {
		appName, err = names.UnitApplication(target)
	}

	if err != nil {
		return err
	}

	curl, _, err := c.applicationAPI.GetCharmURLOrigin("", appName)
	if err != nil {
		return err
	}

	charmInfo, err := c.charmAPI.CharmInfo(curl.String())
	if err != nil {
		return err
	}

	// Get a set of valid hooks.
	validHooks, err := c.getValidHooks(charmInfo.Charm())
	if err != nil {
		return err
	}

	// Get a set of valid actions.
	validActions, err := c.getValidActions(charmInfo.Charm())
	if err != nil {
		return err
	}

	// Is passed argument a valid hook or action name?
	// If not valid, err out.
	allValid := validHooks.Union(validActions)
	for _, hook := range c.hooks {
		if !allValid.Contains(hook) {
			return errors.Errorf("unit %q contains neither hook nor action %q, valid actions are %v and valid hooks are %v",
				c.provider.getTarget(),
				hook,
				validActions.SortedValues(),
				validHooks.SortedValues(),
			)
		}
	}
	return nil
}

func (c *debugHooksCommand) getValidActions(ch charm.Charm) (set.Strings, error) {
	validActions := set.NewStrings()
	for name := range ch.Actions().ActionSpecs {
		validActions.Add(name)
	}
	return validActions, nil
}

func (c *debugHooksCommand) getValidHooks(ch charm.Charm) (set.Strings, error) {
	validHooks := set.NewStrings()
	for _, hook := range hooks.RelationHooks() {
		hook := fmt.Sprintf("juju-info-%s", hook)
		validHooks.Add(hook)
	}
	return validHooks.Union(set.Strings(ch.Meta().Hooks())), nil
}

func (c *debugHooksCommand) decideEntryPoint(ctx *cmd.Context) string {
	if c.modelType == model.CAAS {
		c.provider.setArgs([]string{"which", "sudo"})
		if err := c.sshCommand.Run(ctx); err != nil {
			return "/bin/bash -c '%s'"
		}
	}
	return "sudo /bin/bash -c '%s'"
}

// commonRun is shared between debugHooks and debugCode
func (c *debugHooksCommand) commonRun(
	ctx *cmd.Context,
	target string,
	hooks []string,
	debugAt string,
) (err error) {
	err = c.validateHooksOrActions()
	if err != nil {
		return err
	}

	// If the unit/leader syntax is used, we first need to resolve it into
	// the unit name that corresponds to the current leader.
	resolvedTargetName, err := c.provider.maybeResolveLeaderUnit(target)
	if err != nil {
		return errors.Trace(err)
	}

	debugctx := unitdebug.NewHooksContext(resolvedTargetName)
	clientScript := unitdebug.ClientScript(debugctx, hooks, debugAt)
	b64Script := base64.StdEncoding.EncodeToString([]byte(clientScript))
	innercmd := fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; . $F`, b64Script)
	args := []string{fmt.Sprintf(c.decideEntryPoint(ctx), innercmd)}
	c.provider.setArgs(args)
	return c.sshCommand.Run(ctx)
}

// Run ensures Target is a unit, and resolves its address,
// and connects to it via SSH to execute the debug-hooks
// script.
func (c *debugHooksCommand) Run(ctx *cmd.Context) error {
	if err := c.initAPIs(); err != nil {
		return err
	}
	defer c.closeAPIs()
	return c.commonRun(ctx, c.provider.getTarget(), c.hooks, "")
}
