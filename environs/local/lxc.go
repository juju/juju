package local

import (
	"os/exec"
	"strings"
)

// container represents an LXC container with the given name.
type container struct {
	Name string
}

// rootPath returns the LXC container root filesystem path.
func (c *container) rootPath() string {
	return "/var/lib/lxc/" + c.Name + "/rootfs/"
}

// create creates the LXC container.
func (c *container) create() ([]byte, error) {
	return exec.Command("sudo", "lxc-create", "-n", c.Name).Output()
}

// start starts the LXC container.
func (c *container) start() ([]byte, error) {
	return exec.Command("sudo", "lxc-start", "--daemon", "-n", c.Name).Output()
}

// stop stops the LXC container.
func (c *container) stop() ([]byte, error) {
	return exec.Command("sudo", "lxc-stop", "-n", c.Name).Output()
}

// destroy destroys the LXC container.
func (c *container) destroy() ([]byte, error) {
	return exec.Command("sudo", "lxc-destroy", "-n", c.Name).Output()
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
	output, _ := exec.Command("sudo", "lxc-ls").Output()
	return strings.Fields(string(output))
}
