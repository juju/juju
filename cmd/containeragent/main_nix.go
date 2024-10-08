// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	proxyutils "github.com/juju/proxy"

	"github.com/juju/juju/agent/introspect"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/containeragent/config"
	initcommand "github.com/juju/juju/cmd/containeragent/initialize"
	unitcommand "github.com/juju/juju/cmd/containeragent/unit"
	"github.com/juju/juju/cmd/internal/dumplogs"
	"github.com/juju/juju/cmd/internal/run"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/internal/featureflag"
	internallogger "github.com/juju/juju/internal/logger"
	proxy "github.com/juju/juju/internal/proxy/config"
	_ "github.com/juju/juju/internal/secrets/provider/all" // Import the secret providers.
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/osenv"
)

var logger = internallogger.GetLogger("juju.cmd.containeragent")

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func init() {
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey, osenv.JujuFeatures)
}

var containerAgentDoc = `
juju provides easy, intelligent service orchestration on top of models
such as OpenStack, Amazon AWS, or bare metal. containeragent is a component
of juju for managing container workloads.

https://juju.is/

The containeragent command can also forward invocations over RPC for execution by the
juju unit agent. When used in this way, it expects to be called via a symlink
named for the desired remote command, and expects JUJU_AGENT_SOCKET_ADDRESS and
JUJU_CONTEXT_ID be set in its model.
`

const (
	// exit_err is the value that is returned when the user has run juju in an invalid way.
	exit_err = 2
	// exit_panic is the value that is returned when we exit due to an unhandled panic.
	exit_panic = 3
)

type containerAgentLogWriter struct {
	target io.Writer
}

func (w *containerAgentLogWriter) Write(entry loggo.Entry) {
	if strings.HasPrefix(entry.Module, "unit.") {
		fmt.Fprintln(w.target, w.unitFormat(entry))
	} else {
		fmt.Fprintln(w.target, loggo.DefaultFormatter(entry))
	}
}

func (w *containerAgentLogWriter) unitFormat(entry loggo.Entry) string {
	ts := entry.Timestamp.In(time.UTC).Format("2006-01-02 15:04:05")
	// Just show the last element of the module.
	lastDot := strings.LastIndex(entry.Module, ".")
	module := entry.Module[lastDot+1:]
	return fmt.Sprintf("%s %s %s %s", ts, entry.Level, module, entry.Message)
}

func containerAgentCommand(ctx *cmd.Context) (cmd.Command, error) {
	// Assuming an average of 200 bytes per log message, use up to
	// 200MB for the log buffer.
	// TODO(ycliuhw): move the buffered logger into core package.
	bufferedLogger, err := logsender.InstallBufferedLogWriter(loggo.DefaultContext(), 1048576)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Set the default transport to use the in-process proxy
	// configuration.
	if err := proxy.DefaultConfig.Set(proxyutils.DetectProxies()); err != nil {
		return nil, errors.Trace(err)
	}
	if err := proxy.DefaultConfig.InstallInDefaultTransport(); err != nil {
		return nil, errors.Trace(err)
	}

	containerAgent := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "containerAgent",
		Doc:  containerAgentDoc,
		Log:  jujucmd.DefaultLog,
	})
	containerAgent.Log.NewWriter = func(target io.Writer) loggo.Writer {
		return &containerAgentLogWriter{target: target}
	}

	containerAgent.Register(initcommand.New())
	unitCommand, err := unitcommand.New(ctx, bufferedLogger)
	if err != nil {
		return nil, errors.Trace(err)
	}
	containerAgent.Register(unitCommand)
	return containerAgent, nil
}

func mainWrapper(f commandFactory, args []string) (code int) {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		cmd.WriteError(os.Stderr, err)
		os.Exit(exit_err)
	}
	switch filepath.Base(args[0]) {
	case names.ContainerAgent:
		code = f.containerAgentCmd(ctx, args)
	case names.JujuExec:
		code = f.jujuExec(ctx, args)
	case names.JujuIntrospect:
		code = f.jujuIntrospect(ctx, args)
	case names.JujuDumpLogs:
		code = f.jujuDumpLogs(ctx, args)
	default:
		// This should never happen unless jujuc was missing and hooktools were misconfigured.
		err = errors.New("containeragent always expects to use jujuc for hook tools")
		cmd.WriteError(ctx.Stderr, err)
		os.Exit(exit_err)
	}
	return code
}

// CAAS implementation currently only works for Linux.
func main() {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			buf = buf[:runtime.Stack(buf, false)]
			logger.Criticalf("Unhandled panic: \n%v\n%s", r, buf)
			os.Exit(exit_panic)
		}
	}()

	f := commandFactory{
		containerAgentCmd: func(ctx *cmd.Context, args []string) int {
			cmdToRun, err := containerAgentCommand(ctx)
			if err != nil {
				cmd.WriteError(ctx.Stderr, err)
				os.Exit(1)
			}
			return cmd.Main(cmdToRun, ctx, args[1:])
		},
		jujuExec: func(ctx *cmd.Context, args []string) int {
			lock, err := machinelock.New(machinelock.Config{
				AgentName: "juju-exec",
				Clock:     clock.WallClock,
				Logger:    internallogger.GetLogger("juju.machinelock"),
				// TODO(ycliuhw): consider to rename machinelock package to something more generic for k8s pod lock.
				LogFilename: filepath.Join(config.LogDir, "juju", machinelock.Filename),
			})
			if err != nil {
				err = errors.Annotatef(err, "acquiring machine lock for juju-exec")
				cmd.WriteError(ctx.Stderr, err)
				os.Exit(1)
			}
			return cmd.Main(&run.RunCommand{MachineLock: lock}, ctx, args[1:])
		},
		jujuIntrospect: func(ctx *cmd.Context, args []string) int {
			return cmd.Main(introspect.New(), ctx, args[1:])
		},
		jujuDumpLogs: func(ctx *cmd.Context, args []string) int {
			return cmd.Main(dumplogs.NewCommand(), ctx, args[1:])
		},
	}
	os.Exit(mainWrapper(f, os.Args))
}

type command func(*cmd.Context, []string) int

type commandFactory struct {
	containerAgentCmd command
	jujuExec          command
	jujuIntrospect    command
	jujuDumpLogs      command
}
