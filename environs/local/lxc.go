package local

import (
	"os/exec"
	"strings"
)

// Represents a lxc container.
type container struct {
	Name string
}

// RootPath returns the lxc container root filesystem path
func (c *container) RootPath() string {
	return "/var/lib/lxc/" + c.Name + "/rootfs/"
}

// Create the container executing lxc-create
// to create a container and returns
// the output and error from the this command.
func (c *container) Create() ([]byte, error) {
	return exec.Command("sudo", "lxc-create", "-n", c.Name).Output()
}

// Starts the container executing lxc-start
// and returns the output and error from this command.
func (c *container) Start() ([]byte, error) {
	return exec.Command("sudo", "lxc-start", "--daemon", "-n", c.Name).Output()
}

// Stops the container executing lxc-stop
// and returns the output and error from this command.
func (c *container) Stop() ([]byte, error) {
	return exec.Command("sudo", "lxc-stop", "-n", c.Name).Output()
}

// Destroy the container using lxc-destroy
// and returns the output and the error from this command.
func (c *container) Destroy() ([]byte, error) {
	return exec.Command("sudo", "lxc-destroy", "-n", c.Name).Output()
}

// Running returns true if the container name is in the
// list of the containers that are running.
func (c *container) Running() bool {
	for _, containerName := range List() {
		if containerName == c.Name {
			return true
		}
	}
	return false
}

// List returns a slice with the names of containers
// that are running using the lxc-ls output for this.
func List() []string {
	output, _ := exec.Command("sudo", "lxc-ls").Output()
	return strings.Fields(string(output))
}
