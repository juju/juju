package local

import (
	"bytes"
	"os/exec"
	"strings"
)

// container represents an LXC container with the given name.
type container struct {
	Name string
}

// runLXCCommand runs an LXC command with the given arguments,
// strip outing the usage message.
func runLXCCommand(args ...string) ([]byte, error) {
	output, err := exec.Command(args[0], args...).Output()
	if i := bytes.Index(output, []byte("\nusage: ")); i > 0 {
		output = output[:i]
	}
	return output, err
}

// rootPath returns the LXC container root filesystem path.
func (c *container) rootPath() string {
	return "/var/lib/lxc/" + c.Name + "/rootfs/"
}

// create creates the LXC container.
func (c *container) create() ([]byte, error) {
	return runLXCCommand("sudo", "lxc-create", "-n", c.Name)
}

// start starts the LXC container.
func (c *container) start() ([]byte, error) {
	return runLXCCommand("sudo", "lxc-start", "--daemon", "-n", c.Name)
}

// stop stops the LXC container.
func (c *container) stop() ([]byte, error) {
	return runLXCCommand("sudo", "lxc-stop", "-n", c.Name)
}

// destroy destroys the LXC container.
func (c *container) destroy() ([]byte, error) {
	return runLXCCommand("sudo", "lxc-destroy", "-n", c.Name)
}

// running returns true if the container name is in the
// list of the containers that are running.
func (c *container) running() bool {
	for _, containerName := range list() {
		if containerName == c.Name {
			return true
		}
	}
	return false
}

// list returns a list with the names of containers.
func list() []string {
	output, _ := runLXCCommand("sudo", "lxc-ls")
	return strings.Fields(string(output))
}
