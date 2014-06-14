package cmd

import (
	"os"

	"github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/version"
)

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
	p.Version = version.Current.String()
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
	logger.Infof("running %s [%s %s]", name, version.Current, version.Compiler)
}
