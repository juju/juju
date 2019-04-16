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
	"gopkg.in/juju/names.v2"

	cloudapi "github.com/juju/juju/api/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/jujuclient"
)

var logger = loggo.GetLogger("juju.cmd.juju.cloud")

type listCloudsCommand struct {
	modelcmd.OptionalControllerCommand
	out cmd.Output

	// Used when querying a controller for its cloud details
	controllerName    string
	store             jujuclient.ClientStore
	listCloudsAPIFunc func(controllerName string) (ListCloudsAPI, error)
}

// listCloudsDoc is multi-line since we need to use ` to denote
// commands for ease in markdown.
var listCloudsDoc = "" +
	"Output includes fundamental properties for each cloud known to the\n" +
	"current Juju client: name, number of regions, default region, type,\n" +
	"and description.\n" +
	"\nThe default behaviour is to show clouds available on the current controller,\n" +
	"or another controller specified using --controller.\n" +
	"\nIf --local is specified, the output shows public clouds known to Juju out of the box;\n" +
	"these can be used to bootstrap a controller. Clouds may change between Juju versions.\n" +
	"In addition to these public clouds, the 'localhost' cloud (local LXD) is also listed.\n" +
	"If you supply a controller name the clouds known on the controller will be displayed.\n" +
	"\nThis command's default output format is 'tabular'.\n" +
	"\nCloud metadata sometimes changes, e.g. AWS adds a new region. Use the\n" +
	"`update-clouds` command to update the current Juju client accordingly.\n" +
	"\nUse the `add-cloud` command to add a private cloud to the list of\n" +
	"clouds known to the current Juju client.\n" +
	"\nUse the `regions` command to list a cloud's regions. Use the\n" +
	"`show-cloud` command to get more detail, such as regions and endpoints.\n" +
	"\nFurther reading: https://docs.jujucharms.com/stable/clouds\n" + listCloudsDocExamples

var listCloudsDocExamples = `
Examples:

    juju clouds
    juju clouds --format yaml
    juju clouds --controller mycontroller
    juju clouds --local

See also:
    add-cloud
    regions
    show-cloud
    update-clouds
`

type ListCloudsAPI interface {
	Clouds() (map[names.CloudTag]jujucloud.Cloud, error)
	Close() error
}

// NewListCloudsCommand returns a command to list cloud information.
func NewListCloudsCommand() cmd.Command {
	store := jujuclient.NewFileClientStore()
	c := &listCloudsCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		store:                     store,
	}
	c.listCloudsAPIFunc = c.cloudAPI

	return modelcmd.WrapBase(c)
}

func (c *listCloudsCommand) cloudAPI(controllerName string) (ListCloudsAPI, error) {
	root, err := c.NewAPIRoot(c.store, controllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloudapi.NewClient(root), nil

}

func (c *listCloudsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "clouds",
		Purpose: "Lists all clouds available to Juju.",
		Doc:     listCloudsDoc,
		Aliases: []string{"list-clouds"},
	})
}

func (c *listCloudsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatCloudsTabular,
	})
}

// Init populates the command with the args from the command line.
func (c *listCloudsCommand) Init(args []string) (err error) {
	c.controllerName, err = c.ControllerNameFromArg()
	if err != nil && errors.Cause(err) != modelcmd.ErrNoControllersDefined {
		return errors.Trace(err)
	}
	return nil
}

func (c *listCloudsCommand) getCloudList() (*cloudList, error) {
	if c.controllerName == "" {
		details, err := listCloudDetails()

		if err != nil {
			return nil, err
		}
		return details, nil
	}

	api, err := c.listCloudsAPIFunc(c.controllerName)
	if err != nil {
		return nil, err
	}
	defer api.Close()
	controllerClouds, err := api.Clouds()
	if err != nil {
		return nil, err
	}
	details := newCloudList()
	for _, cloud := range controllerClouds {
		cloudDetails := makeCloudDetails(cloud)
		// TODO: Better categorization than public.
		details.public[cloud.Name] = cloudDetails
	}
	return details, nil
}

func (c *listCloudsCommand) Run(ctxt *cmd.Context) error {
	details, err := c.getCloudList()
	if err != nil {
		return err
	}
	var output interface{}
	switch c.out.Name() {
	case "yaml", "json":
		output = details.all()
	default:
		if c.controllerName == "" && !c.Local {
			ctxt.Infof(
				"There are no controllers running.\nYou can bootstrap a new controller using one of these clouds:\n")
		}
		if c.controllerName != "" {
			ctxt.Infof(
				"Clouds on controller %q:\n\n", c.controllerName)
		}
		output = details
	}

	err = c.out.Write(ctxt, output)
	if err != nil {
		return err
	}
	return nil
}

type cloudList struct {
	public   map[string]*CloudDetails
	builtin  map[string]*CloudDetails
	personal map[string]*CloudDetails
}

func newCloudList() *cloudList {
	return &cloudList{
		make(map[string]*CloudDetails),
		make(map[string]*CloudDetails),
		make(map[string]*CloudDetails),
	}
}

func (c *cloudList) all() map[string]*CloudDetails {
	if len(c.personal) == 0 && len(c.builtin) == 0 && len(c.public) == 0 {
		return nil
	}

	result := make(map[string]*CloudDetails)
	addAll := func(someClouds map[string]*CloudDetails) {
		for name, cloud := range someClouds {
			displayCloud := cloud
			displayCloud.CloudType = displayCloudType(displayCloud.CloudType)
			result[name] = displayCloud
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

	cloudNamesSorted := func(someClouds map[string]*CloudDetails) []string {
		// For tabular we'll sort alphabetically, user clouds last.
		var names []string
		for name := range someClouds {
			names = append(names, name)
		}
		sort.Strings(names)
		return names
	}

	printClouds := func(someClouds map[string]*CloudDetails, color *ansiterm.Context) {
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
			w.Println(len(info.Regions), defaultRegion, displayCloudType(info.CloudType), description)
		}
	}
	printClouds(clouds.public, nil)
	printClouds(clouds.builtin, nil)
	printClouds(clouds.personal, ansiterm.Foreground(ansiterm.BrightBlue))

	tw.Flush()
	return nil
}

func displayCloudType(in string) string {
	if in == "kubernetes" {
		return "k8s"
	}
	return in
}
