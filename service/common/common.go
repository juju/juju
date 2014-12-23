// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package common

// Conf is responsible for defining services. Its fields
// represent elements of a service configuration.
type Conf struct {
	// Desc is the service's description.
	Desc string
	// Env holds the environment variables that will be set when the command runs.
	// Currently not used on Windows.
	Env map[string]string
	// Limit holds the ulimit values that will be set when the command runs.
	// Currently not used on Windows.
	Limit map[string]string
	// Cmd is the command (with arguments) that will be run.
	// The command will be restarted if it exits with a non-zero exit code.
	Cmd string
	// Out, if set, will redirect output to that path.
	// Currently unusable on systemd-based systems,
	// use `# journalctl -unit=servicename` instead.
	Out string
	// InitDir is the folder in which the upstart config/systemd service file
	// should be written under.
	// defaults to "/etc/init" on Ubuntu.
	// defaults to "/etc/systemd/system" on systemd-based systems.
	// Currently not used on Windows.
	InitDir string
	// ExtraScript allows the insertion of a script before command execution.
	// Currently unused under Windows.
	ExtraScript string
}
