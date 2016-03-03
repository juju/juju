package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/version"
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

// NewSuperCommand is like cmd.NewSuperCommand but
// it adds juju-specific functionality:
// - The default logging configuration is taken from the environment;
// - The version is configured to the current juju version;
// - The command emits a log message when a command runs.
func NewSuperCommand(p cmd.SuperCommandParams) *cmd.SuperCommand {
	p.Log = &cmd.Log{
		DefaultConfig: os.Getenv(osenv.JujuLoggingConfigEnvKey),
	}
	current := version.Binary{
		Number: version.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}

	// p.Version should be a version.Binary, but juju/cmd does not
	// import juju/juju/version so this cannot happen. We have
	// tests to assert that this string value is correct.
	p.Version = current.String()
	p.NotifyRun = runNotifier
	return cmd.NewSuperCommand(p)
}

// NewSubSuperCommand should be used to create a SuperCommand
// that runs as a subcommand of some other SuperCommand.
func NewSubSuperCommand(p cmd.SuperCommandParams) *cmd.SuperCommand {
	p.NotifyRun = runNotifier
	return cmd.NewSuperCommand(p)
}

func runNotifier(name string) {
	logger.Infof("running %s [%s %s %s]", name, version.Current, runtime.Compiler, runtime.Version())
}
