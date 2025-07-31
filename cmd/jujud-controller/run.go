// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	proxyutils "github.com/juju/proxy"
	"github.com/juju/utils/v4/exec"

	"github.com/juju/juju/agent/config"
	"github.com/juju/juju/agent/introspect"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/cmd/internal/run"
	agentcmd "github.com/juju/juju/cmd/jujud-controller/agent"
	jujudagentcmd "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/featureflag"
	internallogger "github.com/juju/juju/internal/logger"
	_ "github.com/juju/juju/internal/provider/all" // Import the providers.
	proxy "github.com/juju/juju/internal/proxy/config"
	_ "github.com/juju/juju/internal/secrets/provider/all" // Import the secret providers.
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/worker/dbaccessor"
	"github.com/juju/juju/internal/worker/dbreplaccessor"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/juju/sockets"
)

var logger = internallogger.GetLogger("juju.cmd.jujud")

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

var jujudDoc = `
juju provides easy, intelligent service orchestration on top of models
such as OpenStack, Amazon AWS, or bare metal. jujud is a component of juju.

https://juju.is/

The jujud command can also forward invocations over RPC for execution by the
juju unit agent. When used in this way, it expects to be called via a symlink
named for the desired remote command, and expects JUJU_AGENT_SOCKET_ADDRESS and
JUJU_CONTEXT_ID be set in its model.
`

const (
	// ExitStatusCodeErr is the value that is returned when the user has run juju in an invalid way.
	ExitStatusCodeErr = 2
	// ExitStatusCodePanic is the value that is returned when we exit due to an unhandled panic.
	ExitStatusCodePanic = 3
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
	caCert, err := os.ReadFile(caCertFile)
	if err != nil {
		return sockets.Socket{}, errors.Annotatef(err, "reading %s", caCertFile)
	}
	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(caCert); !ok {
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
		req.Stdin, err = io.ReadAll(os.Stdin)
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

// versionDetail is populated with version information from juju/juju/cmd
// and passed into each SuperCommand. It can be printed using `juju version --all`.
type versionDetail struct {
	// Version of the current binary.
	Version string `json:"version" yaml:"version"`
	// GitCommit of tree used to build the binary.
	GitCommit string `json:"git-commit,omitempty" yaml:"git-commit,omitempty"`
	// GitTreeState is "clean" if the working copy used to build the binary had no
	// uncommitted changes or untracked files, otherwise "dirty".
	GitTreeState string `json:"git-tree-state,omitempty" yaml:"git-tree-state,omitempty"`
	// Compiler reported by runtime.Compiler
	Compiler string `json:"compiler" yaml:"compiler"`
	// OfficialBuild is a monotonic integer set by Jenkins.
	OfficialBuild int `json:"official-build,omitempty" yaml:"official-build,omitempty"`
	// GoBuildTags is the build tags used to build the binary.
	GoBuildTags string `json:"go-build-tags,omitempty" yaml:"go-build-tags,omitempty"`
}

// Main registers subcommands for the jujud executable, and hands over control
// to the cmd package.
func jujuDMain(args []string, ctx *cmd.Context) (code int, err error) {
	// Assuming an average of 200 bytes per log message, use up to
	// 200MB for the log buffer.
	defer logger.Debugf(ctx, "jujud complete, code %d, err %v", code, err)

	// Set the default transport to use the in-process proxy
	// configuration.
	if err := proxy.DefaultConfig.Set(proxyutils.DetectProxies()); err != nil {
		return 1, errors.Trace(err)
	}
	if err := proxy.DefaultConfig.InstallInDefaultTransport(); err != nil {
		return 1, errors.Trace(err)
	}

	current := semversion.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}
	detail := versionDetail{
		Version:      current.String(),
		GitCommit:    jujuversion.GitCommit,
		GitTreeState: jujuversion.GitTreeState,
		Compiler:     jujuversion.Compiler,
		GoBuildTags:  jujuversion.GoBuildTags,
	}

	jujud := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "jujud",
		Doc:  jujudDoc,
		Log:  jujucmd.DefaultLog,
		// p.Version should be a version.Binary, but juju/cmd does not
		// import juju/juju/version so this cannot happen. We have
		// tests to assert that this string value is correct.
		Version:       detail.Version,
		VersionDetail: detail,
	})

	jujud.Log.NewWriter = func(target io.Writer) loggo.Writer {
		return &jujudWriter{target: target}
	}

	bufferedLogger, err := logsender.InstallBufferedLogWriter(loggo.DefaultContext(), 1048576)
	if err != nil {
		return 1, errors.Trace(err)
	}
	jujud.Register(jujudagentcmd.NewModelCommand(bufferedLogger))
	jujud.Register(agentcmd.NewBootstrapCommand())

	// TODO(katco-): AgentConf type is doing too much. The
	// MachineAgent type has called out the separate concerns; the
	// AgentConf should be split up to follow suit.
	agentConf := agentconf.NewAgentConf("")
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(
		agentConf,
		dbaccessor.NewTrackedDBWorker,
		func(mt model.ModelType) upgrades.PreUpgradeStepsFunc {
			if mt == model.CAAS {
				return upgrades.PreUpgradeStepsCAAS
			}
			return upgrades.PreUpgradeSteps
		},
		upgrades.PerformUpgradeSteps,
		"",
	)
	jujud.Register(agentcmd.NewMachineAgentCommand(ctx, machineAgentFactory, agentConf, agentConf))

	safeModeMachineAgentFactory := agentcmd.SafeModeMachineAgentFactoryFn(
		agentConf,
		dbaccessor.NewTrackedDBWorker,
	)
	jujud.Register(agentcmd.NewSafeModeAgentCommand(ctx, safeModeMachineAgentFactory, agentConf, agentConf))

	dbReplModeMachineAgentFactory := agentcmd.DBReplMachineAgentFactoryFn(
		agentConf,
		dbreplaccessor.NewTrackedDBWorker,
	)
	jujud.Register(agentcmd.NewDBReplAgentCommand(ctx, dbReplModeMachineAgentFactory, agentConf, agentConf))

	jujud.Register(agentcmd.NewCheckConnectionCommand(agentConf, agentcmd.ConnectAsAgent))

	code = cmd.Main(jujud, ctx, args[1:])
	return code, nil
}

// Main is not redundant with main(), because it provides an entry point
// for testing with arbitrary command line arguments.
func Main(args []string) int {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			buf = buf[:runtime.Stack(buf, false)]
			logger.Criticalf(context.TODO(), "Unhandled panic: \n%v\n%s", r, buf)
			os.Exit(ExitStatusCodePanic)
		}
	}()

	ctx, err := cmd.DefaultContext()
	if err != nil {
		cmd.WriteError(os.Stderr, err)
		os.Exit(ExitStatusCodeErr)
	}

	var code int
	commandName := filepath.Base(args[0])
	switch commandName {
	case jujunames.Jujud:
		code, err = jujuDMain(args, ctx)
	case jujunames.JujudController:
		code, err = jujuDMain(args, ctx)
	case jujunames.JujuExec:
		lock, err := machinelock.New(machinelock.Config{
			AgentName:   "juju-exec",
			Clock:       clock.WallClock,
			Logger:      internallogger.GetLogger("juju.machinelock"),
			LogFilename: filepath.Join(config.LogDir, "juju", machinelock.Filename),
		})
		if err != nil {
			code = ExitStatusCodeErr
		} else {
			run := &run.RunCommand{MachineLock: lock}
			code = cmd.Main(run, ctx, args[1:])
		}
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
