package lxc

import (
	"bytes"
	"os/exec"
	"strings"
)

type Container struct {
	Name string
}

func (c *Container) Rootfs() string {
	return "/var/lib/lxc/" + c.Name + "/rootfs/"
}

func (c *Container) Create() error {
	return exec.Command("sudo", "lxc-create", "-n", c.Name).Run()
}

func (c *Container) Start() error {
	return exec.Command("sudo", "lxc-start", "--daemon", "-n", c.Name).Run()
}

func (c *Container) Stop() error {
	return exec.Command("sudo", "lxc-stop", "-n", c.Name).Run()
}

func (c *Container) Destroy() error {
	return exec.Command("sudo", "lxc-destroy", "-n", c.Name).Run()
}

func (c *Container) Running() bool {
	return strings.Contains(Ls(), c.Name)
}

func Ls() string {
	var out bytes.Buffer
	cmd := exec.Command("sudo", "lxc-ls")
	cmd.Stdout = &out
	cmd.Run()
	return out.String()
}
