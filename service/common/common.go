package common

// Conf is responsible for defining services. Its fields
// represent elements of a service configuration.
type Conf struct {
	// Desc is the upstart service's description.
	Desc string
	// Env holds the environment variables that will be set when the command runs.
	// Currently not used on Windows
	Env map[string]string
	// Limit holds the ulimit values that will be set when the command runs.
	// Currently not used on Windows
	Limit map[string]string
	// Cmd is the command (with arguments) that will be run.
	// The command will be restarted if it exits with a non-zero exit code.
	Cmd string
	// Out, if set, will redirect output to that path.
	Out string
	// InitDir is the folder in which the init/upstart script should be written
	// defaults to "/etc/init" on Ubuntu
	// Currently not used on Windows

	InitDir string
}
