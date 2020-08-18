// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/chzyer/readline"
	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

type replCommand struct {
	cmd.CommandBase

	store    jujuclient.ClientStore
	showHelp bool

	execJujuCommand func(cmd.Command, *cmd.Context, []string) int
}

func newReplCommand(showHelp bool) cmd.Command {
	return &replCommand{
		showHelp:        showHelp,
		store:           jujuclient.NewFileClientStore(),
		execJujuCommand: cmd.Main,
	}
}

const replDoc = `
When run without arguments, enter an interactive shell which can be
used to run any Juju command directly. When in the shell:
  type "help" to see a list of available commands.
  type "q" or ^D or ^C to quit.

Otherwise, the supported command usage is described below.
`

var firstPrompt = `
Welcome to the Juju interactive shell.
Type "help" to see a list of available commands.
Type "q" or ^D or ^C to quit.
`[1:]

var (
	quitCommands         = set.NewStrings("q", "quit", "exit")
	noControllerCommands = set.NewStrings("bootstrap", "register")
)

const (
	promptSuffix         = "$ "
	replHelpHint         = `Type "help" to see a list of commands`
	noControllersMessage = `Please either create a new controller using "bootstrap" or connect to
another controller that you have been given access to using "register".`
)

// Info implements Command.
func (c *replCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "juju",
		Purpose: "Enter an interactive shell for running Juju commands",
	})
}

// filterInput is used to exclude characters
// from being accepted from stdin.
func filterInput(r rune) (rune, bool) {
	switch r {
	// block CtrlZ feature
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}

// Run implements Command.
func (c *replCommand) Run(ctx *cmd.Context) error {
	if err := c.Init(nil); err != nil {
		return errors.Trace(err)
	}
	if c.showHelp {
		jujuCmd := NewJujuCommand(ctx, "")
		f := gnuflag.NewFlagSet(c.Info().Name, gnuflag.ContinueOnError)
		f.SetOutput(ioutil.Discard)
		jujuCmd.SetFlags(f)
		if err := jujuCmd.Init([]string{"help"}); err != nil {
			return errors.Trace(err)
		}
		fmt.Fprintln(ctx.Stdout, replDoc)
		return jujuCmd.Run(ctx)
	}

	history, err := ioutil.TempFile("", "juju-repl")
	if err != nil {
		return errors.Trace(err)
	}
	defer history.Close()

	l, err := readline.NewEx(&readline.Config{
		Stdin:               readline.NewCancelableStdin(ctx.Stdin),
		Stdout:              ctx.Stdout,
		Stderr:              ctx.Stderr,
		HistoryFile:         history.Name(),
		InterruptPrompt:     "^C",
		HistorySearchFold:   true,
		FuncFilterInputRune: filterInput,
		// TODO(wallyworld) - add auto complete support
		//AutoComplete:    jujuCompleter,
	})
	if err != nil {
		return errors.Trace(err)
	}
	defer l.Close()

	// Record the default loggo writer so we can
	// reset it before each command invocation.
	defaultWriter, err := loggo.RemoveWriter(loggo.DefaultWriterName)
	if err != nil {
		return errors.Trace(err)
	}
	first := true
	for {
		// loggo maintains global state so reset before each command.
		loggo.ResetLogging()
		_ = loggo.RegisterWriter(loggo.DefaultWriterName, defaultWriter)

		jujuCmd := NewJujuCommandWithStore(ctx, c.store, jujucmd.DefaultLog, "", replHelpHint, nil, false)
		if c.showHelp {
			f := gnuflag.NewFlagSet(c.Info().Name, gnuflag.ContinueOnError)
			f.SetOutput(ioutil.Discard)
			jujuCmd.SetFlags(f)
			if err := jujuCmd.Init([]string{"help"}); err != nil {
				return errors.Trace(err)
			}
			fmt.Fprintln(ctx.Stderr, replDoc)
			return jujuCmd.Run(ctx)
		}
		// Get the prompt based on the current controller/model/user.
		noCurrentController := false
		prompt, err := c.getPrompt()
		if err != nil {
			// There's no controller, so ask the user to bootstrap first.
			if errors.Cause(err) != modelcmd.ErrNoControllersDefined {
				return errors.Trace(err)
			}
			noCurrentController = true
			prompt = "no controllers registered" + promptSuffix
		}

		if first {
			fmt.Fprintln(ctx.Stdout, firstPrompt)
			first = false
		}
		l.SetPrompt(prompt)
		line, err := l.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if quitCommands.Contains(strings.ToLower(line)) {
			break
		}
		args := strings.Fields(line)
		if noCurrentController && !noControllerCommands.Contains(args[0]) {
			fmt.Fprintln(ctx.Stderr, noControllersMessage)
			continue
		}
		c.execJujuCommand(jujuCmd, ctx, args)
	}
	return nil
}

func (c *replCommand) getPrompt() (prompt string, err error) {
	defer func() {
		if err == nil {
			prompt += promptSuffix
		}
	}()

	store := modelcmd.QualifyingClientStore{c.store}

	controllerName, err := modelcmd.DetermineCurrentController(store)
	if err != nil && !errors.IsNotFound(err) {
		return "", errors.Trace(err)
	}
	if err != nil {
		all, err := c.store.AllControllers()
		if err != nil {
			return "", errors.Trace(err)
		}
		if len(all) == 0 {
			return "", modelcmd.ErrNoControllersDefined
		}
		// There are controllers but none selected as current.
		return "", nil
	}
	modelName, err := store.CurrentModel(controllerName)
	if errors.IsNotFound(err) {
		modelName = ""
	} else if err != nil {
		return "", errors.Trace(err)
	}

	userName := ""
	account, err := store.AccountDetails(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		return "", errors.Trace(err)
	}
	if err == nil {
		userName = account.User
	}
	if userName != "" {
		controllerName = userName + "@" + controllerName
		if jujuclient.IsQualifiedModelName(modelName) {
			baseModelName, userTag, _ := jujuclient.SplitModelName(modelName)
			if userName == userTag.Name() {
				modelName = baseModelName
			}
		}
	}
	prompt = controllerName
	if modelName != "" {
		prompt = controllerName + ":" + modelName
	}
	return prompt, nil
}
