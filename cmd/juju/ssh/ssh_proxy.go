// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

func NewSSHProxyCommand() cmd.Command {
	c := &sshProxyCommand{}
	return modelcmd.Wrap(c)
}

// sshProxyCommand is responsible for launching a ssh shell on a given unit or machine.
type sshProxyCommand struct {
	modelcmd.ModelCommandBase

	target    names.Tag
	container string
}

func (c *sshProxyCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.container, "container", "", "container to connect to (k8s only)")
}

func (c *sshProxyCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "ssh-proxy",
		Args:     "<target>",
		Purpose:  "",
		Doc:      "",
		Examples: "",
		SeeAlso:  []string{},
	})
}

func (c *sshProxyCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.Errorf("no target name specified")
	}
	target := args[0]
	switch {
	case names.IsValidMachine(target):
		c.target = names.NewMachineTag(target)
	case names.IsValidUnit(target):
		c.target = names.NewUnitTag(target)
	default:
		return errors.Errorf("target %q is not a valid machine or unit name", c.target)
	}
	return nil
}

func (c *sshProxyCommand) Run(ctx *cmd.Context) error {
	apiConn, err := c.NewAPIRoot()
	if err != nil {
		return err
	}

	bestFacadeVersion := apiConn.BestFacadeVersion("SSHClient")
	if bestFacadeVersion < 5 {
		return fmt.Errorf("controller does not support ssh proxying")
	}

	req, dialer, err := apiConn.NewHTTPRequest()
	if err != nil {
		return err
	}
	switch tag := c.target.(type) {
	case names.MachineTag:
		req.URL.Path, err = url.JoinPath(req.URL.Path, "machine", tag.Id(), "ssh")
		if err != nil {
			return err
		}
	case names.UnitTag:
		app, err := names.UnitApplication(tag.Id())
		if err != nil {
			return err
		}
		num, err := names.UnitNumber(tag.Id())
		if err != nil {
			return err
		}
		req.URL.Path, err = url.JoinPath(req.URL.Path, "application", app, "unit", strconv.Itoa(num), "ssh")
		if err != nil {
			return err
		}
	}
	if c.container != "" {
		q := req.URL.Query()
		q.Add("container", c.container)
		req.URL.RawQuery = q.Encode()
	}
	req.Method = http.MethodConnect
	req.Header.Add("Connection", "Upgrade")
	req.Header.Add("Upgrade", "ssh")

	tlsConn, err := dialer.DialContext(ctx, "tcp", req.URL.Host)
	if err != nil {
		return err
	}
	defer tlsConn.Close()

	err = req.Write(tlsConn)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(tlsConn)
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		defer resp.Body.Close()
		return fmt.Errorf("could not establish ssh proxy: %s", resp.Status)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(os.Stdout, tlsConn)
	}()

	_, err = io.Copy(tlsConn, os.Stdin)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	wg.Wait()
	return tlsConn.Close()
}
