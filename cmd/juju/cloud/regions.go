// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/output"
)

type listRegionsCommand struct {
	cmd.CommandBase
	out       cmd.Output
	cloudName string
}

var listRegionsDoc = `
Examples:

    juju regions aws

See also:
    add-cloud
    clouds
    show-cloud
    update-clouds
`

// NewListRegionsCommand returns a command to list cloud region information.
func NewListRegionsCommand() cmd.Command {
	return &listRegionsCommand{}
}

// Info implements Command.Info.
func (c *listRegionsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "regions",
		Args:    "<cloud>",
		Purpose: "Lists regions for a given cloud.",
		Doc:     listRegionsDoc,
		Aliases: []string{"list-regions"},
	}
}

// SetFlags implements Command.SetFlags.
func (c *listRegionsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatRegionsListTabular,
	})
}

// Init implements Command.Init.
func (c *listRegionsCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no cloud specified")
	case 1:
		c.cloudName = args[0]
	}
	return cmd.CheckEmpty(args[1:])
}

// Run implements Command.Run.
func (c *listRegionsCommand) Run(ctxt *cmd.Context) error {
	cloud, err := common.CloudByName(c.cloudName)
	if err != nil {
		return errors.Trace(err)
	}

	if len(cloud.Regions) == 0 {
		fmt.Fprintf(ctxt.GetStdout(), "Cloud %q has no regions defined.\n", c.cloudName)
		return nil
	}
	var regions interface{}
	if c.out.Name() == "json" {
		details := make(map[string]regionDetails)
		for _, r := range cloud.Regions {
			details[r.Name] = regionDetails{
				Endpoint:         r.Endpoint,
				IdentityEndpoint: r.IdentityEndpoint,
				StorageEndpoint:  r.StorageEndpoint,
			}
		}
		regions = details
	} else {
		details := make(yaml.MapSlice, len(cloud.Regions))
		for i, r := range cloud.Regions {
			details[i] = yaml.MapItem{r.Name, regionDetails{
				Name:             r.Name,
				Endpoint:         r.Endpoint,
				IdentityEndpoint: r.IdentityEndpoint,
				StorageEndpoint:  r.StorageEndpoint,
			}}
		}
		regions = details
	}
	err = c.out.Write(ctxt, regions)
	if err != nil {
		return err
	}
	return nil
}

func (c *listRegionsCommand) formatRegionsListTabular(writer io.Writer, value interface{}) error {
	regions, ok := value.(yaml.MapSlice)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", regions, value)
	}
	return formatRegionsTabular(writer, regions)
}

func formatRegionsTabular(writer io.Writer, regions yaml.MapSlice) error {
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	for _, r := range regions {
		w.Println(r.Key)
	}
	tw.Flush()
	return nil
}
