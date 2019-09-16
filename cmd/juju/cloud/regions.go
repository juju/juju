// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"io"

	"github.com/juju/ansiterm"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v3"
	"gopkg.in/yaml.v2"

	cloudapi "github.com/juju/juju/api/cloud"
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

If the current controller can be detected, a user will be prompted to 
confirm if regions for the cloud known to the controller need to be 
listed as well. If the prompt is not needed and the regions from 
current controller's cloud are always to be listed, use --no-prompt option.

Use --controller option to list regions from the cloud from a different controller.

Use --local option to only list regions known locally on this client.

Regions for a cloud known locally on this client are always listed.

Examples:

    juju regions aws
    juju regions aws --controller mycontroller
    juju regions aws --local
    juju regions aws --no-prompt

See also:
    add-cloud
    clouds
    show-cloud
    update-clouds
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
			Store: store,
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
		Name:    "regions",
		Args:    "<cloud>",
		Purpose: "Lists regions for a given cloud.",
		Doc:     listRegionsDoc,
		Aliases: []string{"list-regions"},
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
	switch len(args) {
	case 0:
		return errors.New("no cloud specified")
	case 1:
		c.cloudName = args[0]
	}
	return cmd.CheckEmpty(args[1:])
}

type FoundRegions struct {
	Local  interface{} `yaml:"local-cloud-regions,omitempty" json:"local-cloud-regions,omitempty"`
	Remote interface{} `yaml:"controller-cloud-regions,omitempty" json:"controller-cloud-regions,omitempty"`
}

// Run implements Command.Run.
func (c *listRegionsCommand) Run(ctxt *cmd.Context) error {
	c.found = &FoundRegions{}
	var returnErr error
	if err := c.findLocalRegions(ctxt); err != nil {
		ctxt.Warningf("%v", err)
		returnErr = cmd.ErrSilent
	}

	if !c.Local {
		if err := c.findRemoteRegions(ctxt); err != nil {
			ctxt.Warningf("%v", err)
			returnErr = cmd.ErrSilent
		}
	}
	if err := c.out.Write(ctxt, *c.found); err != nil {
		ctxt.Warningf("%v", err)
		returnErr = cmd.ErrSilent
	}
	return returnErr
}

func (c *listRegionsCommand) findRemoteRegions(ctxt *cmd.Context) error {
	if c.ControllerName == "" {
		// The user may have specified the controller via a --controller option.
		// If not, let's see if there is a current controller that can be detected.
		var err error
		c.ControllerName, err = c.MaybePromptCurrentController(ctxt, fmt.Sprintf("list regions for cloud %q from", c.cloudName))
		if err != nil {
			return errors.Trace(err)
		}
	}
	if c.ControllerName == "" {
		return errors.Errorf("Not listing regions for cloud %q from a controller: no controller specified.", c.cloudName)
	}

	api, err := c.cloudAPIFunc()
	if err != nil {
		return err
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
		for _, r := range locals {
			w.Println(r.Key)
		}
	}
	if remotes, ok := regions.Remote.(yaml.MapSlice); ok {
		for _, r := range remotes {
			w.PrintColor(ansiterm.Foreground(ansiterm.BrightBlue), r.Key)
			w.Println()
		}
	}
	tw.Flush()
	return nil
}
