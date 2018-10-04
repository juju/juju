// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"io"
	"sort"

	"github.com/juju/ansiterm"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/output"
)

var logger = loggo.GetLogger("juju.cmd.juju.cloud")

type listCloudsCommand struct {
	cmd.CommandBase
	out cmd.Output
}

// listCloudsDoc is multi-line since we need to use ` to denote
// commands for ease in markdown.
var listCloudsDoc = "" +
        "Output includes fundamental properties for each cloud known to Juju.\n" +
        "These are: name, number of regions, default region, type, and description.\n" +
        "\nThe cloud name is what's used to refer to a cloud in any Juju command.\n" +
        "\nThe `regions` command lists a cloud's regions. Both regions and endpoints\n" +
	"are exposed with `show-cloud`.\n" +
        "\nThe default output shows those clouds known to Juju out of the box (the\n" +
	"\"baked in\" clouds). With the exception of the 'localhost' cloud, these\n" +
	"are all the supported public clouds. Other (private) clouds need to be\n" +
	"added via the `add-cloud` command. These are 'lxd' (remote), 'maas',\n" +
	"'manual', 'openstack', and 'vsphere'.\n" +
        "\nCloud metadata sometimes changes (e.g. AWS adds a new region). To\n" +
	"synchronise Juju with the \"baked in\" clouds the `update-clouds` command\n" +
	"is used.\n" +
        "\nThis command's default output format is 'tabular'.\n" +
        "\nFurther reading: https://docs.jujucharms.com/stable/clouds\n" + listCloudsDocExamples

var listCloudsDocExamples = `
Examples:

    juju clouds
    juju clouds --format yaml

See also:
    add-cloud
    regions
    show-cloud
    update-clouds
`

// NewListCloudsCommand returns a command to list cloud information.
func NewListCloudsCommand() cmd.Command {
	return &listCloudsCommand{}
}

func (c *listCloudsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "clouds",
		Purpose: "Lists all clouds available to Juju.",
		Doc:     listCloudsDoc,
		Aliases: []string{"list-clouds"},
	}
}

func (c *listCloudsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatCloudsTabular,
	})
}

func (c *listCloudsCommand) Run(ctxt *cmd.Context) error {
	details, err := listCloudDetails()
	if err != nil {
		return err
	}

	var output interface{}
	switch c.out.Name() {
	case "yaml", "json":
		output = details.all()
	default:
		output = details
	}
	err = c.out.Write(ctxt, output)
	if err != nil {
		return err
	}
	return nil
}

type cloudList struct {
	public   map[string]*cloudDetails
	builtin  map[string]*cloudDetails
	personal map[string]*cloudDetails
}

func newCloudList() *cloudList {
	return &cloudList{
		make(map[string]*cloudDetails),
		make(map[string]*cloudDetails),
		make(map[string]*cloudDetails),
	}
}

func (c *cloudList) all() map[string]*cloudDetails {
	if len(c.personal) == 0 && len(c.builtin) == 0 && len(c.personal) == 0 {
		return nil
	}

	result := make(map[string]*cloudDetails)
	addAll := func(someClouds map[string]*cloudDetails) {
		for name, cloud := range someClouds {
			result[name] = cloud
		}
	}

	addAll(c.public)
	addAll(c.builtin)
	addAll(c.personal)
	return result
}

func listCloudDetails() (*cloudList, error) {
	clouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return nil, err
	}
	details := newCloudList()
	for name, cloud := range clouds {
		cloudDetails := makeCloudDetails(cloud)
		details.public[name] = cloudDetails
	}

	// Add in built in clouds like localhost (lxd).
	builtinClouds, err := common.BuiltInClouds()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for name, cloud := range builtinClouds {
		cloudDetails := makeCloudDetails(cloud)
		cloudDetails.Source = "built-in"
		details.builtin[name] = cloudDetails
	}

	personalClouds, err := jujucloud.PersonalCloudMetadata()
	if err != nil {
		return nil, err
	}
	for name, cloud := range personalClouds {
		cloudDetails := makeCloudDetails(cloud)
		cloudDetails.Source = "local"
		details.personal[name] = cloudDetails
		// Delete any built-in or public clouds with same name.
		delete(details.builtin, name)
		delete(details.public, name)
	}

	return details, nil
}

// formatCloudsTabular writes a tabular summary of cloud information.
func formatCloudsTabular(writer io.Writer, value interface{}) error {
	clouds, ok := value.(*cloudList)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", clouds, value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	w.Println("Cloud", "Regions", "Default", "Type", "Description")
	w.SetColumnAlignRight(1)

	cloudNamesSorted := func(someClouds map[string]*cloudDetails) []string {
		// For tabular we'll sort alphabetically, user clouds last.
		var names []string
		for name := range someClouds {
			names = append(names, name)
		}
		sort.Strings(names)
		return names
	}

	printClouds := func(someClouds map[string]*cloudDetails, color *ansiterm.Context) {
		cloudNames := cloudNamesSorted(someClouds)

		for _, name := range cloudNames {
			info := someClouds[name]
			defaultRegion := ""
			if len(info.Regions) > 0 {
				defaultRegion = info.RegionsMap[info.Regions[0].Key.(string)].Name
			}
			description := info.CloudDescription
			if len(description) > 40 {
				description = description[:39]
			}
			w.PrintColor(color, name)
			w.Println(len(info.Regions), defaultRegion, info.CloudType, description)
		}
	}
	printClouds(clouds.public, nil)
	printClouds(clouds.builtin, nil)
	printClouds(clouds.personal, ansiterm.Foreground(ansiterm.BrightBlue))

	tw.Flush()
	return nil
}
