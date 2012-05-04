package upstart

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
)

const (
	descT = `description "%s"
author "Juju Team <juju@lists.ubuntu.com>"
`
	start = `start on runlevel [2345]
stop on runlevel [!2345]
respawn
`
	envT  = "env %s=%q\n"
	outT  = "/tmp/%s.output"
	execT = "exec %s >> %s 2>&1\n"
)

// Conf is responsible for defining and installing upstart services.
type Conf struct {
	Service
	Desc string
	Env  map[string]string
	Cmd  string
	Out  string
}

// validate returns an error if the service is not adequately defined.
func (c *Conf) validate() error {
	if c.Name == "" {
		return errors.New("missing Name")
	}
	if c.InitDir == "" {
		return errors.New("missing InitDir")
	}
	if c.Desc == "" {
		return errors.New("missing Desc")
	}
	if c.Cmd == "" {
		return errors.New("missing Cmd")
	}
	return nil
}

// render returns the upstart configuration for the service as a string.
func (c *Conf) render() (string, error) {
	if err := c.validate(); err != nil {
		return "", err
	}
	parts := []string{fmt.Sprintf(descT, c.Desc), start}
	for k, v := range c.Env {
		parts = append(parts, fmt.Sprintf(envT, k, v))
	}
	out := c.Out
	if out == "" {
		out = fmt.Sprintf(outT, c.Name)
	}
	parts = append(parts, fmt.Sprintf(execT, c.Cmd, out))
	return strings.Join(parts, ""), nil
}

// Install installs and starts the service.
func (c *Conf) Install() error {
	conf, err := c.render()
	if err != nil {
		return err
	}
	if c.Installed() {
		if err := c.Stop(); err != nil {
			return err
		}
	}
	if err := ioutil.WriteFile(c.path(), []byte(conf), 0644); err != nil {
		return err
	}
	return c.Start()
}

// InstallCommands returns shell commands to install and start the service.
func (c *Conf) InstallCommands() ([]string, error) {
	conf, err := c.render()
	if err != nil {
		return nil, err
	}
	return []string{
		fmt.Sprintf("cat >> %c << EOF\n%sEOF\n", c.path(), conf),
		"start " + c.Name,
	}, nil
}
