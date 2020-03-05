// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/juju/juju/juju/osenv"
	jujuversion "github.com/juju/juju/version"
)

func init() {
	// If the environment key is empty, ConfigureLoggers returns nil and does
	// nothing.
	err := loggo.ConfigureLoggers(os.Getenv(osenv.JujuStartupLoggingConfigEnvKey))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR parsing %s: %s\n\n", osenv.JujuStartupLoggingConfigEnvKey, err)
	}
}

var logger = loggo.GetLogger("juju.cmd")

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
}

// NewSuperCommand is like cmd.NewSuperCommand but
// it adds juju-specific functionality:
// - The default logging configuration is taken from the environment;
// - The version is configured to the current juju version;
// - The additional version information is sourced from juju/juju/version;
// - The command emits a log message when a command runs.
func NewSuperCommand(p cmd.SuperCommandParams) *cmd.SuperCommand {
	p.Log = &cmd.Log{
		DefaultConfig: os.Getenv(osenv.JujuLoggingConfigEnvKey),
	}
	current := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	detail := versionDetail{
		Version:       current.String(),
		GitCommit:     jujuversion.GitCommit,
		GitTreeState:  jujuversion.GitTreeState,
		Compiler:      jujuversion.Compiler,
		OfficialBuild: jujuversion.OfficialBuild,
	}

	// p.Version should be a version.Binary, but juju/cmd does not
	// import juju/juju/version so this cannot happen. We have
	// tests to assert that this string value is correct.
	p.Version = detail.Version
	p.VersionDetail = detail
	if p.NotifyRun != nil {
		messenger := p.NotifyRun
		p.NotifyRun = func(str string) {
			messenger(str)
			runNotifier(str)
		}
	} else {
		p.NotifyRun = runNotifier
	}
	p.FlagKnownAs = "option"
	return cmd.NewSuperCommand(p)
}

func runNotifier(name string) {
	logger.Infof("running %s [%s %d %s %s %s]", name, jujuversion.Current, jujuversion.OfficialBuild, jujuversion.GitCommit, runtime.Compiler, runtime.Version())
	logger.Debugf("  args: %#v", os.Args)
}

func Info(i *cmd.Info) *cmd.Info {
	info := *i
	info.FlagKnownAs = "option"
	info.ShowSuperFlags = []string{"show-log", "debug", "logging-config", "verbose", "quiet", "h", "help"}
	return &info
}

// IsPiped determines if the command was used in a pipe and,
// hence, it's stdin is not usable for user input.
func IsPiped(ctx *cmd.Context) bool {
	stdIn, ok := ctx.Stdin.(*os.File)
	return ok && !terminal.IsTerminal(int(stdIn.Fd()))
}
