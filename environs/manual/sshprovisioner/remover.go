// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshprovisioner

import (
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/mongo"
	"github.com/juju/utils"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/ssh"
)

//go:generate mockgen -package mocks -destination mocks/manual_mock.go github.com/juju/juju/environs/manual CommandExec,CommandRunner

// RemoveMachine returns nil if the removal process is done successfully
func RemoveMachine(args manual.RemoveMachineArgs) error {
	// When removing a manual machine, it should be expected that Juju already
	// existed on the machine. With that in mind, there should also be an
	// existing ubuntu user and we have all the authorized keys setup. If that's
	// not the case, do we know what we're logging into?
	if err := LoginUbuntuUser(args.CommandExec, args.Host); err != nil {
		return errors.Trace(err)
	}

	// validate the machine has been provisioned with Juju.
	provisioned, err := checkManualProvisioned(args.CommandExec, args.Host)
	if err != nil {
		return errors.Annotatef(err, "error checking if provisioned")
	}
	if !provisioned {
		return errors.Errorf("machine not provisioned")
	}
	script := TearDownScript(false)
	if err := runSSHCommand(
		args.CommandExec,
		"ubuntu@"+args.Host,
		[]string{"sudo", "/bin/bash"}, script,
		args.Stdout,
		args.Stderr,
	); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// LoginUbuntuUser will attempt to login as the ubuntu user for the host.
func LoginUbuntuUser(cmdExec manual.CommandExec, host string) error {
	cmd := cmdExec.Command("ubuntu@"+host, []string{"sudo", "-n", "true"})
	if cmd.Run() != nil {
		// Failed to login as ubuntu (or passwordless sudo is not enabled).
		return errors.Errorf("unable to login to remove machine on %s", host)
	}
	return nil
}

func runSSHCommand(cmdExec manual.CommandExec, host string, command []string, stdin string, stdout, stderr io.Writer) error {
	cmd := cmdExec.Command(host, command)
	cmd.SetStdin(strings.NewReader(stdin))
	cmd.SetStdout(stdout)
	cmd.SetStderr(stderr)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// TearDownScript creates a script that will enable the tearing down of a jujud
// agent, along with cleaning up any log files.
func TearDownScript(removeMongo bool) string {
	script := `
# Signal the jujud process to stop, then check it has done so before cleaning-up
# after it.
set -x
touch %[1]s

stopped=0
function wait_for_jujud {
    for i in {1..30}; do
        if pgrep jujud > /dev/null ; then
            sleep 1
        else
            echo jujud stopped
            stopped=1
            logger --id jujud stopped on attempt $i
            break
        fi
    done
}

# There might be no jujud at all (for example, after a failed deployment) so
# don't require pkill to succeed before looking for a jujud process.
# SIGABRT not SIGTERM, as abort lets the worker know it should uninstall itself,
# rather than terminate normally.
pkill -SIGABRT jujud
wait_for_jujud

[[ $stopped -ne 1 ]] && {
    # If jujud didn't stop nicely, we kill it hard here.
    %[2]spkill -SIGKILL jujud && wait_for_jujud
}
[[ $stopped -ne 1 ]] && {
    echo jujud removal failed
    logger --id $(ps -o pid,cmd,state -p $(pgrep jujud) | awk 'NR != 1 {printf("Process %%d (%%s) has state %%s\n", $1, $2, $3)}')
    exit 1
}
[[ -z "%[3]s" ]] && {
	service %[3]s stop && logger --id stopped %[3]s
	apt-get -y purge juju-mongo*
	apt-get -y autoremove
}
rm -f /etc/init/juju*
rm -f /etc/systemd/system{,/multi-user.target.wants}/juju*
rm -fr %[4]s %[5]s
exit 0
`
	var diagnostics string
	if featureflag.Enabled(feature.DeveloperMode) {
		diagnostics = `
    echo "Dump engine report and goroutines for stuck jujud"
    source /etc/profile.d/juju-introspection.sh
    juju-engine-report
    juju-goroutines
`
	}
	// only remove mongo if we've been asked to via the argument
	var mongoServiceName string
	if removeMongo {
		mongoServiceName = mongo.ServiceName
	}
	return fmt.Sprintf(
		script,
		// WARNING: this is linked with the use of uninstallFile in
		// the agent package. Don't change it without extreme care,
		// and handling for mismatches with already-deployed agents.
		utils.ShQuote(path.Join(
			agent.DefaultPaths.DataDir,
			agent.UninstallFile,
		)),
		diagnostics,
		mongoServiceName,
		utils.ShQuote(agent.DefaultPaths.DataDir),
		utils.ShQuote(agent.DefaultPaths.LogDir),
	)
}

// DefaultCommandExec creates a command execution context to run commands against.
func DefaultCommandExec() manual.CommandExec {
	return commandExecShim{}
}

type commandExecShim struct{}

func (commandExecShim) Command(host string, command []string) manual.CommandRunner {
	return commandRunnerShim{
		cmd: ssh.Command(host, command, nil),
	}
}

type commandRunnerShim struct {
	cmd *ssh.Cmd
}

func (s commandRunnerShim) SetStdin(r io.Reader) {
	s.cmd.Stdin = r
}

func (s commandRunnerShim) SetStdout(w io.Writer) {
	s.cmd.Stdout = w
}

func (s commandRunnerShim) SetStderr(w io.Writer) {
	s.cmd.Stderr = w
}

func (s commandRunnerShim) Run() error {
	return s.cmd.Run()
}
