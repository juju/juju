// Copyright 2012-2020 Canonical Ltd.
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
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/loggo"
	proxyutils "github.com/juju/proxy"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/jujud/dumplogs"
	"github.com/juju/juju/cmd/jujud/introspect"
	"github.com/juju/juju/cmd/jujud/run"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	initcommand "github.com/juju/juju/cmd/k8sagent/init"
	unitcommand "github.com/juju/juju/cmd/k8sagent/unit"
	components "github.com/juju/juju/component/all"
	"github.com/juju/juju/juju/names"
	"github.com/juju/juju/utils/proxy"
	"github.com/juju/juju/worker/logsender"
)

var logger = loggo.GetLogger("juju.cmd.k8sagent")

func init() {
	if err := components.RegisterForServer(); err != nil {
		logger.Criticalf("unable to register server components: %v", err)
		os.Exit(1)
	}
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

var k8sAgentDoc = `
juju provides easy, intelligent service orchestration on top of models
such as OpenStack, Amazon AWS, or bare metal. k8sagent is a component
of juju for managing k8s workloads.

https://jujucharms.com/

The k8sagent command can also forward invocations over RPC for execution by the
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

type k8sAgentLogWriter struct {
	target io.Writer
}

func (w *k8sAgentLogWriter) Write(entry loggo.Entry) {
	if strings.HasPrefix(entry.Module, "unit.") {
		fmt.Fprintln(w.target, w.unitFormat(entry))
	} else {
		fmt.Fprintln(w.target, loggo.DefaultFormatter(entry))
	}
}

func (w *k8sAgentLogWriter) unitFormat(entry loggo.Entry) string {
	ts := entry.Timestamp.In(time.UTC).Format("2006-01-02 15:04:05")
	// Just show the last element of the module.
	lastDot := strings.LastIndex(entry.Module, ".")
	module := entry.Module[lastDot+1:]
	return fmt.Sprintf("%s %s %s %s", ts, entry.Level, module, entry.Message)
}

func k8sAgentCommand(args []string, ctx *cmd.Context) (code int, err error) {
	// Assuming an average of 200 bytes per log message, use up to
	// 200MB for the log buffer.
	defer logger.Debugf("k8sagent complete, code %d, err %v", code, err)
	bufferedLogger, err := logsender.InstallBufferedLogWriter(loggo.DefaultContext(), 1048576)
	if err != nil {
		return 1, errors.Trace(err)
	}

	// Set the default transport to use the in-process proxy
	// configuration.
	if err := proxy.DefaultConfig.Set(proxyutils.DetectProxies()); err != nil {
		return 1, errors.Trace(err)
	}
	if err := proxy.DefaultConfig.InstallInDefaultTransport(); err != nil {
		return 1, errors.Trace(err)
	}

	k8sAgent := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "k8sAgent",
		Doc:  k8sAgentDoc,
	})

	k8sAgent.Log.NewWriter = func(target io.Writer) loggo.Writer {
		return &k8sAgentLogWriter{target: target}
	}

	k8sAgent.Register(initcommand.New())
	k8sAgent.Register(unitcommand.New(ctx, bufferedLogger))
	code = cmd.Main(k8sAgent, ctx, args[1:])
	return code, nil
}

func mainWrapper(args []string) int {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			buf = buf[:runtime.Stack(buf, false)]
			logger.Criticalf("Unhandled panic: \n%v\n%s", r, buf)
			os.Exit(exit_panic)
		}
	}()

	ctx, err := cmd.DefaultContext()
	if err != nil {
		cmd.WriteError(os.Stderr, err)
		os.Exit(exit_err)
	}

	var code int
	commandName := filepath.Base(args[0])
	switch commandName {
	case names.K8sAgent:
		code, err = k8sAgentCommand(args, ctx)
	case names.JujuRun:
		lock, err := machinelock.New(machinelock.Config{
			AgentName:   "juju-run",
			Clock:       clock.WallClock,
			Logger:      loggo.GetLogger("juju.machinelock"),
			LogFilename: filepath.Join(cmdutil.LogDir, machinelock.Filename),
		})
		if err != nil {
			code = exit_err
		} else {
			run := &run.RunCommand{MachineLock: lock}
			code = cmd.Main(run, ctx, args[1:])
		}
	case names.JujuDumpLogs:
		code = cmd.Main(dumplogs.NewCommand(), ctx, args[1:])
	case names.JujuIntrospect:
		code = cmd.Main(&introspect.IntrospectCommand{}, ctx, args[1:])
	default:
		code = 1
		// This should never happen unless jujuc was missing and hooktools were misconfigured.
		err = errors.New("k8sagent always expects to use jujuc for hook tools")
	}
	if err != nil {
		cmd.WriteError(ctx.Stderr, err)
	}
	return code
}
