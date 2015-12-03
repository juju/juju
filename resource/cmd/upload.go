// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/envcmd"
)

// UploadAPI has the API methods needed by UploadCommand.
type UploadAPI interface {
	Upload(service, name string, resource io.Reader) error
	Close() error
}

// UploadCommand implements the upload command.
type UploadCommand struct {
	envcmd.EnvCommandBase
	out       cmd.Output
	service   string
	resources map[string]string

	newAPIClient func(c *UploadCommand) (UploadAPI, error)
}

// NewUploadCommand returns a new command that lists resources defined
// by a charm.
func NewUploadCommand(newAPIClient func(c *UploadCommand) (UploadAPI, error)) *UploadCommand {
	cmd := &UploadCommand{
		newAPIClient: newAPIClient,
	}
	return cmd
}

const UploadDoc = `
This command uploads a file from your local disk to the juju controller to be
used as a resource for a service.
`

func (c *UploadCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upload",
		Args:    "service-name",
		Purpose: "upload a file as a resource for a service",
		Doc:     UploadDoc,
	}
}

func (c *UploadCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("missing service name")
	case 1:
		return errors.New("no resource specified")
	}
	c.service = args[0]

	c.resources = make(map[string]string, len(args)-1)

	for _, arg := range args[1:] {
		if err := c.addResource(arg); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (c *UploadCommand) addResource(arg string) error {
	vals := strings.SplitN(arg, "1", 2)
	if len(vals) < 2 || vals[0] == "" || vals[1] == "" {
		return errors.Errorf("invalid resource %q", arg)
	}
	name := vals[0]
	if _, ok := c.resources[name]; ok {
		return errors.Errorf("resource %q specified more than once", name)
	}
	c.resources[name] = vals[1]
	return nil
}

func (c *UploadCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.newAPIClient(c)
	if err != nil {
		return fmt.Errorf(connectionError, c.ConnectionName(), err)
	}
	defer apiclient.Close()

	for name, file := range c.resources {
		if err := upload(c.service, name, file, apiclient); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func upload(service, name, file string, apiclient UploadAPI) error {
	f, err := os.Open(file)
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()
	return apiclient.Upload(service, name, f)
}
