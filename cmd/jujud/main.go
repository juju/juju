// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
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
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	proxyutils "github.com/juju/proxy"
	"github.com/juju/utils/v3/exec"
	"github.com/juju/version/v2"

	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	jujucmd "github.com/juju/juju/cmd"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	jujudagentcmd "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	"github.com/juju/juju/cmd/jujud/agent/caasoperator"
	"github.com/juju/juju/cmd/jujud/agent/config"
	"github.com/juju/juju/cmd/jujud/dumplogs"
	"github.com/juju/juju/cmd/jujud/introspect"
	"github.com/juju/juju/cmd/jujud/run"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/machinelock"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/debug/coveruploader"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/juju/sockets"
	_ "github.com/juju/juju/provider/all"         // Import the providers.
	_ "github.com/juju/juju/secrets/provider/all" // Import the secret providers.
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/utils/proxy"
	jujuversion "github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.cmd.jujud")

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
	caCert, err := os.ReadFile(caCertFile)
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
	// OfficialBuildNumber is a monotonic integer set by Jenkins.
	OfficialBuildNumber int `json:"official-build-number,omitempty" yaml:"official-build-number,omitempty"`
	// Official is true if this is an official build.
	Official bool `json:"official" yaml:"official"`
	// Grade reflects the snap grade value.
	Grade string `json:"grade,omitempty" yaml:"grade,omitempty"`
	// GoBuildTags is the build tags used to build the binary.
	GoBuildTags string `json:"go-build-tags,omitempty" yaml:"go-build-tags,omitempty"`
}

// Main registers subcommands for the jujud executable, and hands over control
// to the cmd package.
func jujuDMain(args []string, ctx *cmd.Context) (code int, err error) {
	// Assuming an average of 200 bytes per log message, use up to
	// 200MB for the log buffer.
	defer logger.Debugf("jujud complete, code %d, err %v", code, err)
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

	current := version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}
	detail := versionDetail{
		Version:             current.String(),
		GitCommit:           jujuversion.GitCommit,
		GitTreeState:        jujuversion.GitTreeState,
		Compiler:            jujuversion.Compiler,
		GoBuildTags:         jujuversion.GoBuildTags,
		OfficialBuildNumber: jujuversion.OfficialBuild,
		Official:            isOfficial(),
		Grade:               jujuversion.Grade,
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

	jujud.Register(agentcmd.NewBootstrapCommand())
	jujud.Register(jujudagentcmd.NewCAASUnitInitCommand())
	jujud.Register(jujudagentcmd.NewModelCommand(bufferedLogger))

	// TODO(katco-): AgentConf type is doing too much. The
	// MachineAgent type has called out the separate concerns; the
	// AgentConf should be split up to follow suit.
	agentConf := agentconf.NewAgentConf("")
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(
		agentConf,
		bufferedLogger,
		upgrades.PreUpgradeSteps,
		"",
	)
	jujud.Register(agentcmd.NewMachineAgentCmd(ctx, machineAgentFactory, agentConf, agentConf))

	caasOperatorAgent, err := jujudagentcmd.NewCaasOperatorAgent(ctx, bufferedLogger, func(mc *caasoperator.ManifoldsConfig) error {
		mc.NewExecClient = k8sexec.NewInCluster
		return nil
	})
	if err != nil {
		return -1, errors.Trace(err)
	}
	jujud.Register(caasOperatorAgent)

	jujud.Register(jujudagentcmd.NewCheckConnectionCommand(agentConf, jujudagentcmd.ConnectAsAgent))

	code = cmd.Main(jujud, ctx, args[1:])
	return code, nil
}

func isOfficial() bool {
	// If there's an error getting jujud version, play it safe.
	// We pretend it's not official.
	jujudPath, err := tools.ExistingJujuLocation()
	if err != nil {
		return false
	}
	_, err = tools.GetVersionFromFile(jujudPath)
	return err == nil
}

// MainWrapper exists to preserve test functionality.
func MainWrapper(args []string) {
	os.Exit(Main(args))
}

func main() {
	coveruploader.Enable()
	MainWrapper(os.Args)
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
	case jujunames.JujuExec:
		lock, err := machinelock.New(machinelock.Config{
			AgentName:   "juju-exec",
			Clock:       clock.WallClock,
			Logger:      loggo.GetLogger("juju.machinelock"),
			LogFilename: filepath.Join(config.LogDir, "juju", machinelock.Filename),
		})
		if err != nil {
			code = exit_err
		} else {
			run := &run.RunCommand{MachineLock: lock}
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
