// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	jujucloud "github.com/juju/juju/cloud"
)

type listCloudsCommand struct {
	cmd.CommandBase
	out cmd.Output
}

var listCloudsDoc = `
The list-clouds command lists the clouds on which Juju workloads can be deployed.
The available clouds will be the publicly available clouds like AWS, Google, Azure,
as well as any custom clouds make available by the add-cloud command.

Example:
   juju list-clouds
`

// NewListCloudsCommand returns a command to list cloud information.
func NewListCloudsCommand() cmd.Command {
	return &listCloudsCommand{}
}

func (c *listCloudsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-clouds",
		Purpose: "list clouds available to run Juju workloads",
		Doc:     listCloudsDoc,
	}
}

func (c *listCloudsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatCloudsTabular,
	})
}

func (c *listCloudsCommand) Run(ctxt *cmd.Context) error {
	publicClouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicClouds())
	if err != nil {
		return err
	}
	return c.out.Write(ctxt, publicClouds.Clouds)
}

// formatCloudsTabular returns a tabular summary of cloud information.
func formatCloudsTabular(value interface{}) ([]byte, error) {
	clouds, ok := value.(map[string]jujucloud.Cloud)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", clouds, value)
	}

	// For tabular we'll sort alphabetically.
	var cloudNames []string
	for name, _ := range clouds {
		cloudNames = append(cloudNames, name)
	}
	sort.Strings(cloudNames)

	var out bytes.Buffer
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	p := func(values ...string) {
		text := strings.Join(values, "\t")
		fmt.Fprintln(tw, text)
	}
	p("CLOUD\tTYPE\tREGIONS")
	for _, name := range cloudNames {
		info := clouds[name]
		var regions []string
		for region, _ := range info.Regions {
			regions = append(regions, region)
		}
		// TODO(wallyworld) - we should be smarter about handling
		// long region text, for now we'll display the first 7 as
		// that covers all clouds except AWS and Azure and will
		// prevent wrapping on a reasonable terminal width.
		regionCount := len(regions)
		if regionCount > 7 {
			regionCount = 7
		}
		regionText := strings.Join(regions[:regionCount], ", ")
		if len(regions) > 7 {
			regionText = regionText + " ..."
		}
		p(name, info.Type, regionText)
	}
	tw.Flush()

	return out.Bytes(), nil
}
