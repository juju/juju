// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"io"

	"github.com/juju/ansiterm"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
	"gopkg.in/yaml.v2"

	cloudapi "github.com/juju/juju/api/client/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/jujuclient"
)

type listRegionsCommand struct {
	modelcmd.OptionalControllerCommand
	out       cmd.Output
	cloudName string

	cloudAPIFunc func() (CloudRegionsAPI, error)
	found        *FoundRegions
}

var listRegionsDoc = `
List regions for a given cloud.

Use ` + "`--controller`" + ` option to list regions from the cloud from a controller.

Use ` + "`--client`" + ` option to list regions known locally on this client.
`

const listRegionsExamples = `
    juju regions aws
    juju regions aws --controller mycontroller
    juju regions aws --client
    juju regions aws --client --controller mycontroller
`

type CloudRegionsAPI interface {
	Cloud(tag names.CloudTag) (jujucloud.Cloud, error)
	Close() error
}

// NewListRegionsCommand returns a command to list cloud region information.
func NewListRegionsCommand() cmd.Command {
	store := jujuclient.NewFileClientStore()
	c := &listRegionsCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store:    store,
			ReadOnly: true,
		},
	}
	c.cloudAPIFunc = c.cloudAPI
	return modelcmd.WrapBase(c)
}

func (c *listRegionsCommand) cloudAPI() (CloudRegionsAPI, error) {
	root, err := c.NewAPIRoot(c.Store, c.ControllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloudapi.NewClient(root), nil
}

// Info implements Command.Info.
func (c *listRegionsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "regions",
		Args:     "<cloud>",
		Purpose:  "Lists regions for a given cloud.",
		Doc:      listRegionsDoc,
		Aliases:  []string{"list-regions"},
		Examples: listRegionsExamples,
		SeeAlso: []string{
			"add-cloud",
			"clouds",
			"show-cloud",
			"update-cloud",
			"update-public-clouds",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *listRegionsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatRegionsListTabular,
	})
}

// Init implements Command.Init.
func (c *listRegionsCommand) Init(args []string) error {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
	switch len(args) {
	case 0:
		return errors.New("no cloud specified")
	case 1:
		c.cloudName = args[0]
	}
	return cmd.CheckEmpty(args[1:])
}

type FoundRegions struct {
	Local  interface{} `yaml:"client-cloud-regions,omitempty" json:"client-cloud-regions,omitempty"`
	Remote interface{} `yaml:"controller-cloud-regions,omitempty" json:"controller-cloud-regions,omitempty"`
}

func (f *FoundRegions) IsEmpty() bool {
	return f == &FoundRegions{}
}

// Run implements Command.Run.
func (c *listRegionsCommand) Run(ctxt *cmd.Context) error {
	if err := c.MaybePrompt(ctxt, fmt.Sprintf("list regions for cloud %q from", c.cloudName)); err != nil {
		return errors.Trace(err)
	}
	c.found = &FoundRegions{}
	var returnErr error
	if c.Client {
		if err := c.findLocalRegions(ctxt); err != nil {
			ctxt.Infof("ERROR %v", err)
			returnErr = cmd.ErrSilent
		}
	}

	if c.ControllerName != "" {
		if err := c.findRemoteRegions(ctxt); err != nil {
			ctxt.Infof("ERROR %v", err)
			returnErr = cmd.ErrSilent
		}
	}
	if !c.found.IsEmpty() {
		if err := c.out.Write(ctxt, *c.found); err != nil {
			return err
		}
	}
	return returnErr
}

func (c *listRegionsCommand) findRemoteRegions(ctxt *cmd.Context) error {
	api, err := c.cloudAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()
	aCloud, err := api.Cloud(names.NewCloudTag(c.cloudName))
	if err != nil {
		return errors.Annotatef(err, "on controller %q", c.ControllerName)
	}

	if len(aCloud.Regions) == 0 {
		fmt.Fprintf(ctxt.GetStdout(), "Cloud %q has no regions defined on controller %q.\n", c.cloudName, c.ControllerName)
		return nil
	}
	c.found.Remote = c.parseRegions(aCloud)
	return nil
}

func (c *listRegionsCommand) findLocalRegions(ctxt *cmd.Context) error {
	cloud, err := common.CloudByName(c.cloudName)
	if err != nil {
		return errors.Trace(err)
	}
	if len(cloud.Regions) == 0 {
		fmt.Fprintf(ctxt.GetStdout(), "Cloud %q has no regions defined locally on this client.\n", c.cloudName)
		return nil
	}
	c.found.Local = c.parseRegions(*cloud)
	return nil
}

func (c *listRegionsCommand) parseRegions(aCloud jujucloud.Cloud) interface{} {
	if c.out.Name() == "json" {
		details := make(map[string]RegionDetails)
		for _, r := range aCloud.Regions {
			details[r.Name] = RegionDetails{
				Endpoint:         r.Endpoint,
				IdentityEndpoint: r.IdentityEndpoint,
				StorageEndpoint:  r.StorageEndpoint,
			}
		}
		return details
	}
	details := make(yaml.MapSlice, len(aCloud.Regions))
	for i, r := range aCloud.Regions {
		details[i] = yaml.MapItem{r.Name, RegionDetails{
			Name:             r.Name,
			Endpoint:         r.Endpoint,
			IdentityEndpoint: r.IdentityEndpoint,
			StorageEndpoint:  r.StorageEndpoint,
		}}
	}
	return details
}

func (c *listRegionsCommand) formatRegionsListTabular(writer io.Writer, value interface{}) error {
	regions, ok := value.(FoundRegions)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", regions, value)
	}
	return formatRegionsTabular(writer, regions)
}

func formatRegionsTabular(writer io.Writer, regions FoundRegions) error {
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}

	if locals, ok := regions.Local.(yaml.MapSlice); ok {
		w.Println("\nClient Cloud Regions")
		for _, r := range locals {
			w.Println(r.Key)
		}
	}
	if remotes, ok := regions.Remote.(yaml.MapSlice); ok {
		w.Println("\nController Cloud Regions")
		for _, r := range remotes {
			w.PrintColor(ansiterm.Foreground(ansiterm.BrightBlue), r.Key)
			w.Println()
		}
	}
	tw.Flush()
	return nil
}
