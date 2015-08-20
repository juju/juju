// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/exec"

	jujucmd "github.com/juju/juju/cmd"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/storage/looputil"
	// Import the providers.
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

var jujudDoc = `
juju provides easy, intelligent service orchestration on top of environments
such as OpenStack, Amazon AWS, or bare metal. jujud is a component of juju.

https://juju.ubuntu.com/

The jujud command can also forward invocations over RPC for execution by the
juju unit agent. When used in this way, it expects to be called via a symlink
named for the desired remote command, and expects JUJU_AGENT_SOCKET and
JUJU_CONTEXT_ID be set in its environment.
`

const (
	// exit_err is the value that is returned when the user has run juju in an invalid way.
	exit_err = 2
	// exit_panic is the value that is returned when we exit due to an unhandled panic.
	exit_panic = 3
)

func getenv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%s not set", name)
	}
	return value, nil
}

func getwd() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// jujuCMain uses JUJU_CONTEXT_ID and JUJU_AGENT_SOCKET to ask a running unit agent
// to execute a Command on our behalf. Individual commands should be exposed
// by symlinking the command name to this executable.
func jujuCMain(commandName string, ctx *cmd.Context, args []string) (code int, err error) {
	code = 1
	contextId, err := getenv("JUJU_CONTEXT_ID")
	if err != nil {
		return
	}
	dir, err := getwd()
	if err != nil {
		return
	}
	req := jujuc.Request{
		ContextId:   contextId,
		Dir:         dir,
		CommandName: commandName,
		Args:        args[1:],
	}
	socketPath, err := getenv("JUJU_AGENT_SOCKET")
	if err != nil {
		return
	}
	client, err := sockets.Dial(socketPath)
	if err != nil {
		return
	}
	defer client.Close()
	var resp exec.ExecResponse
	err = client.Call("Jujuc.Main", req, &resp)
	if err != nil && err.Error() == jujuc.ErrNoStdin.Error() {
		req.Stdin, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			err = errors.Annotate(err, "cannot read stdin")
			return
		}
		req.StdinSet = true
		err = client.Call("Jujuc.Main", req, &resp)
	}
	if err != nil {
		return
	}
	os.Stdout.Write(resp.Stdout)
	os.Stderr.Write(resp.Stderr)
	return resp.Code, nil
}

// Main registers subcommands for the jujud executable, and hands over control
// to the cmd package.
func jujuDMain(args []string, ctx *cmd.Context) (code int, err error) {
	// Assuming an average of 200 bytes per log message, use up to
	// 200MB for the log buffer.
	logCh, err := logsender.InstallBufferedLogWriter(1048576)
	if err != nil {
		return 1, errors.Trace(err)
	}

	jujud := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "jujud",
		Doc:  jujudDoc,
	})
	jujud.Log.Factory = &writerFactory{}
	jujud.Register(NewBootstrapCommand())

	// TODO(katco-): AgentConf type is doing too much. The
	// MachineAgent type has called out the separate concerns; the
	// AgentConf should be split up to follow suit.
	agentConf := agentcmd.NewAgentConf("")
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(
		agentConf, logCh, looputil.NewLoopDeviceManager(),
	)
	jujud.Register(agentcmd.NewMachineAgentCmd(ctx, machineAgentFactory, agentConf, agentConf))

	jujud.Register(agentcmd.NewUnitAgent(ctx, logCh))

	code = cmd.Main(jujud, ctx, args[1:])
	return code, nil
}

// This function exists to preserve test functionality.
// On windows we need to catch the return code from main for
// service functionality purposes, but on unix we can just os.Exit
func MainWrapper(args []string) {
	os.Exit(Main(args))
}

// Main is not redundant with main(), because it provides an entry point
// for testing with arbitrary command line arguments.
func Main(args []string) int {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			buf = buf[:runtime.Stack(buf, false)]
			logger.Criticalf("Unhandled panic: \n%v\n%s", r, buf)
			os.Exit(exit_panic)
		}
	}()
	var code int = 1
	ctx, err := cmd.DefaultContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(exit_err)
	}
	commandName := filepath.Base(args[0])
	if commandName == names.Jujud {
		code, err = jujuDMain(args, ctx)
	} else if commandName == names.Jujuc {
		fmt.Fprint(os.Stderr, jujudDoc)
		code = exit_err
		err = fmt.Errorf("jujuc should not be called directly")
	} else if commandName == names.JujuRun {
		code = cmd.Main(&RunCommand{}, ctx, args[1:])
	} else {
		code, err = jujuCMain(commandName, ctx, args)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
	return code
}

type writerFactory struct{}

func (*writerFactory) NewWriter(target io.Writer) loggo.Writer {
	return &jujudWriter{target: target}
}

type jujudWriter struct {
	target           io.Writer
	unitFormatter    simpleFormatter
	defaultFormatter loggo.DefaultFormatter
}

var _ loggo.Writer = (*jujudWriter)(nil)

func (w *jujudWriter) Write(level loggo.Level, module, filename string, line int, timestamp time.Time, message string) {
	if strings.HasPrefix(module, "unit.") {
		fmt.Fprintln(w.target, w.unitFormatter.Format(level, module, timestamp, message))
	} else {
		fmt.Fprintln(w.target, w.defaultFormatter.Format(level, module, filename, line, timestamp, message))
	}
}

type simpleFormatter struct{}

func (*simpleFormatter) Format(level loggo.Level, module string, timestamp time.Time, message string) string {
	ts := timestamp.In(time.UTC).Format("2006-01-02 15:04:05")
	// Just show the last element of the module.
	lastDot := strings.LastIndex(module, ".")
	module = module[lastDot+1:]
	return fmt.Sprintf("%s %s %s %s", ts, level, module, message)
}
