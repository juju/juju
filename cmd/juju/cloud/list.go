// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"io"
	"sort"
	"strings"

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
	"Provided information includes 'cloud' (as understood by Juju), cloud\n" +
	"'type', and cloud 'regions'.\n" +
	"The listing will consist of public clouds and any custom clouds made\n" +
	"available through the `juju add-cloud` command. The former can be updated\n" +
	"via the `juju update-cloud` command.\n" +
	"By default, the tabular format is used.\n" + listCloudsDocExamples

var listCloudsDocExamples = `
Examples:

    juju clouds

See also: show-cloud
          update-clouds
          add-cloud
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

const localPrefix = "local:"

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
	for name, cloud := range common.BuiltInClouds() {
		cloudDetails := makeCloudDetails(cloud)
		cloudDetails.Source = "built-in"
		details.builtin[name] = cloudDetails
	}

	personalClouds, err := jujucloud.PersonalCloudMetadata()
	if err != nil {
		return nil, err
	}
	for name, cloud := range personalClouds {
		// Add to result with "local:" prefix.
		cloudDetails := makeCloudDetails(cloud)
		cloudDetails.Source = "local"
		details.personal[localPrefix+name] = cloudDetails
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
	p := func(values ...string) {
		text := strings.Join(values, "\t")
		fmt.Fprintln(tw, text)
	}
	p("CLOUD\tTYPE\tREGIONS")

	cloudNamesSorted := func(someClouds map[string]*cloudDetails) []string {
		// For tabular we'll sort alphabetically, user clouds last.
		var names []string
		for name, _ := range someClouds {
			names = append(names, name)
		}
		sort.Strings(names)
		return names
	}

	printClouds := func(someClouds map[string]*cloudDetails) {
		cloudNames := cloudNamesSorted(someClouds)

		for _, name := range cloudNames {
			info := someClouds[name]
			var regions []string
			for _, region := range info.Regions {
				regions = append(regions, fmt.Sprint(region.Key))
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
			p(name, info.CloudType, regionText)
		}
	}
	printClouds(clouds.public)
	printClouds(clouds.builtin)
	printClouds(clouds.personal)

	tw.Flush()
	return nil
}
