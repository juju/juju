// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	proxyutils "github.com/juju/proxy"
	"github.com/juju/utils/exec"
	"gopkg.in/juju/names.v3"

	jujucmd "github.com/juju/juju/cmd"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/cmd/jujud/agent/caasoperator"
	"github.com/juju/juju/cmd/jujud/dumplogs"
	"github.com/juju/juju/cmd/jujud/introspect"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	components "github.com/juju/juju/component/all"
	"github.com/juju/juju/core/machinelock"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/sockets"

	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"

	// Import the providers.
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/utils/proxy"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

var logger = loggo.GetLogger("juju.cmd.jujud")

func init() {
	if err := components.RegisterForServer(); err != nil {
		logger.Criticalf("unable to register server components: %v", err)
		os.Exit(1)
	}
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

var jujudDoc = `
juju provides easy, intelligent service orchestration on top of models
such as OpenStack, Amazon AWS, or bare metal. jujud is a component of juju.

https://jujucharms.com/

The jujud command can also forward invocations over RPC for execution by the
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

func getenv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", errors.Errorf("%s not set", name)
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

func getSocket() (sockets.Socket, error) {
	var err error
	socket := sockets.Socket{}
	socket.Address, err = getenv("JUJU_AGENT_SOCKET_ADDRESS")
	if err != nil {
		return sockets.Socket{}, err
	}
	socket.Network, err = getenv("JUJU_AGENT_SOCKET_NETWORK")
	if err != nil {
		return sockets.Socket{}, err
	}

	// If we are not connecting over tcp, no need for TLS.
	if socket.Network != "tcp" {
		return socket, nil
	}

	caCertFile, err := getenv("JUJU_AGENT_CA_CERT")
	if err != nil {
		return sockets.Socket{}, err
	}
	caCert, err := ioutil.ReadFile(caCertFile)
	if err != nil {
		return sockets.Socket{}, errors.Annotatef(err, "reading %s", caCertFile)
	}
	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(caCert); ok == false {
		return sockets.Socket{}, errors.Errorf("invalid ca certificate")
	}

	unitName, err := getenv("JUJU_UNIT_NAME")
	if err != nil {
		return sockets.Socket{}, err
	}
	application, err := names.UnitApplication(unitName)
	if err != nil {
		return sockets.Socket{}, errors.Trace(err)
	}
	socket.TLSConfig = &tls.Config{
		RootCAs:    rootCAs,
		ServerName: application,
	}
	return socket, nil
}

// hookToolMain uses JUJU_CONTEXT_ID and JUJU_AGENT_SOCKET_ADDRESS to ask a running unit agent
// to execute a Command on our behalf. Individual commands should be exposed
// by symlinking the command name to this executable.
func hookToolMain(commandName string, ctx *cmd.Context, args []string) (code int, err error) {
	code = 1
	contextID, err := getenv("JUJU_CONTEXT_ID")
	if err != nil {
		return
	}
	dir, err := getwd()
	if err != nil {
		return
	}
	req := jujuc.Request{
		ContextId:   contextID,
		Dir:         dir,
		CommandName: commandName,
		Args:        args[1:],
		Token:       os.Getenv("JUJU_AGENT_TOKEN"),
	}
	socket, err := getSocket()
	if err != nil {
		return
	}
	client, err := sockets.Dial(socket)
	if err != nil {
		return code, err
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
	defer logger.Debugf("jujud complete, code %d, err %v", code, err)
	bufferedLogger, err := logsender.InstallBufferedLogWriter(1048576)
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

	jujud := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "jujud",
		Doc:  jujudDoc,
	})

	jujud.Log.NewWriter = func(target io.Writer) loggo.Writer {
		return &jujudWriter{target: target}
	}

	jujud.Register(agentcmd.NewBootstrapCommand())
	jujud.Register(agentcmd.NewCAASUnitInitCommand())
	jujud.Register(agentcmd.NewModelCommand())

	// TODO(katco-): AgentConf type is doing too much. The
	// MachineAgent type has called out the separate concerns; the
	// AgentConf should be split up to follow suit.
	agentConf := agentcmd.NewAgentConf("")
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(
		agentConf,
		bufferedLogger,
		agentcmd.DefaultIntrospectionSocketName,
		upgrades.PreUpgradeSteps,
		"",
	)
	jujud.Register(agentcmd.NewMachineAgentCmd(ctx, machineAgentFactory, agentConf, agentConf))

	unitAgent, err := agentcmd.NewUnitAgent(ctx, bufferedLogger)
	if err != nil {
		return -1, errors.Trace(err)
	}
	jujud.Register(unitAgent)

	caasOperatorAgent, err := agentcmd.NewCaasOperatorAgent(ctx, bufferedLogger, func(mc *caasoperator.ManifoldsConfig) error {
		mc.NewExecClient = k8sexec.NewInCluster
		return nil
	})
	if err != nil {
		return -1, errors.Trace(err)
	}
	jujud.Register(caasOperatorAgent)

	jujud.Register(agentcmd.NewCheckConnectionCommand(agentConf, agentcmd.ConnectAsAgent))

	code = cmd.Main(jujud, ctx, args[1:])
	return code, nil
}

// MainWrapper exists to preserve test functionality.
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

	ctx, err := cmd.DefaultContext()
	if err != nil {
		cmd.WriteError(os.Stderr, err)
		os.Exit(exit_err)
	}

	var code int
	commandName := filepath.Base(args[0])
	switch commandName {
	case jujunames.Jujud:
		code, err = jujuDMain(args, ctx)
	case jujunames.JujuRun:
		lock, err := machinelock.New(machinelock.Config{
			AgentName:   "juju-run",
			Clock:       clock.WallClock,
			Logger:      loggo.GetLogger("juju.machinelock"),
			LogFilename: filepath.Join(cmdutil.LogDir, machinelock.Filename),
		})
		if err != nil {
			code = exit_err
		} else {
			run := &RunCommand{MachineLock: lock}
			code = cmd.Main(run, ctx, args[1:])
		}
	case jujunames.JujuDumpLogs:
		code = cmd.Main(dumplogs.NewCommand(), ctx, args[1:])
	case jujunames.JujuIntrospect:
		code = cmd.Main(&introspect.IntrospectCommand{}, ctx, args[1:])
	default:
		code, err = hookToolMain(commandName, ctx, args)
	}
	if err != nil {
		cmd.WriteError(ctx.Stderr, err)
	}
	return code
}

type jujudWriter struct {
	target io.Writer
}

func (w *jujudWriter) Write(entry loggo.Entry) {
	if strings.HasPrefix(entry.Module, "unit.") {
		fmt.Fprintln(w.target, w.unitFormat(entry))
	} else {
		fmt.Fprintln(w.target, loggo.DefaultFormatter(entry))
	}
}

func (w *jujudWriter) unitFormat(entry loggo.Entry) string {
	ts := entry.Timestamp.In(time.UTC).Format("2006-01-02 15:04:05")
	// Just show the last element of the module.
	lastDot := strings.LastIndex(entry.Module, ".")
	module := entry.Module[lastDot+1:]
	return fmt.Sprintf("%s %s %s %s", ts, entry.Level, module, entry.Message)
}
