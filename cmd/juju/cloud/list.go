// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"io"
	"sort"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

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

	listCloudsAPIFunc func() (ListCloudsAPI, error)

	all            bool
	showAllMessage bool
}

// listCloudsDoc is multi-line since we need to use ` to denote
// commands for ease in markdown.
var listCloudsDoc = "" +
	"Display the fundamental properties for each cloud known to Juju:\n" +
	"name, number of regions, number of registered credentials, default region, type, etc...\n" +
	"\n" +
	"Clouds known to this client are the clouds known to Juju out of the box \n" +
	"along with any which have been added with `add-cloud --client`. These clouds can be\n" +
	"used to create a controller and can be displayed using --client option.\n" +
	"\n" +
	"Clouds may be listed that are co-hosted with the Juju client.  When the LXD hypervisor\n" +
	"is detected, the 'localhost' cloud is made available.  When a microk8s installation is\n" +
	"detected, the 'microk8s' cloud is displayed.\n" +
	"\n" +
	"Use --controller option to list clouds from a controller. \n" +
	"Use --client option to list clouds from this client. \n" +
	"This command's default output format is 'tabular'. Use 'json' and 'yaml' for\n" +
	"machine-readable output.\n" +
	"\n" +
	"Cloud metadata sometimes changes, e.g. providers add regions. Use the `update-public-clouds`\n" +
	"command to update public clouds or `update-cloud` to update other clouds.\n" +
	"\n" +
	"Use the `regions` command to list a cloud's regions.\n" +
	"\n" +
	"Use the `show-cloud` command to get more detail, such as regions and endpoints.\n" +
	"\n" +
	"Further reading:\n " +
	"\n" +
	"    Documentation:   https://jaas.ai/docs/clouds\n" +
	"    microk8s:        https://microk8s.io/\n" +
	"    LXD hypervisor:  https://linuxcontainers.org/lxd/\n" +
	listCloudsDocExamples

var listCloudsDocExamples = `
Examples:

    juju clouds
    juju clouds --format yaml
    juju clouds --controller mycontroller 
    juju clouds --controller mycontroller --client
    juju clouds --client

See also:
    add-cloud
    credentials
    controllers
    regions
    default-credential
    default-region
    show-cloud
    update-cloud
    update-public-clouds
`

type ListCloudsAPI interface {
	Clouds() (map[names.CloudTag]jujucloud.Cloud, error)
	Close() error
}

// NewListCloudsCommand returns a command to list cloud information.
func NewListCloudsCommand() cmd.Command {
	store := jujuclient.NewFileClientStore()
	c := &listCloudsCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store:    store,
			ReadOnly: true,
		},
	}
	c.listCloudsAPIFunc = c.cloudAPI

	return modelcmd.WrapBase(c)
}

func (c *listCloudsCommand) cloudAPI() (ListCloudsAPI, error) {
	root, err := c.NewAPIRoot(c.Store, c.ControllerName, "")
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
	f.BoolVar(&c.all, "all", false, "Show all available clouds")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatCloudsTabular,
	})
}

func (c *listCloudsCommand) getCloudList(ctxt *cmd.Context) (*cloudList, error) {
	var returnErr error
	warn := func(anErr error) {
		ctxt.Warningf("%v", anErr)
		returnErr = cmd.ErrSilent
	}
	details := newCloudList()
	if c.Client {
		var err error
		if details, err = listLocalCloudDetails(c.Store); err != nil {
			warn(err)
		}
	}

	if c.ControllerName != "" {
		remotes := func() error {
			api, err := c.listCloudsAPIFunc()
			if err != nil {
				return errors.Trace(err)
			}
			defer api.Close()
			controllerClouds, err := api.Clouds()
			if err != nil {
				return errors.Trace(err)
			}
			for _, cloud := range controllerClouds {
				cloudDetails := makeCloudDetails(c.Store, cloud)
				details.remote[cloud.Name] = cloudDetails
			}
			return nil
		}
		if err := remotes(); err != nil {
			warn(err)
		}
	}
	c.showAllMessage = details.filter(c.all)
	return details, returnErr
}

func (c *listCloudsCommand) Run(ctxt *cmd.Context) error {
	if err := c.MaybePrompt(ctxt, "list clouds from"); err != nil {
		return errors.Trace(err)
	}

	details, err := c.getCloudList(ctxt)
	if err != nil {
		return errors.Trace(err)
	}
	if c.showAllMessage {
		if details.len() != 0 {
			ctxt.Infof("Only clouds with registered credentials are shown.")
		} else {
			ctxt.Infof("No clouds with registered credentials to show.")
		}
		ctxt.Infof("There are more clouds, use --all to see them.")
	}
	var result interface{}
	switch c.out.Name() {
	case "yaml", "json":
		clouds := details.all()
		for _, cloud := range clouds {
			cloud.CloudType = displayCloudType(cloud.CloudType)
		}
		result = clouds
	default:
		result = details
	}
	return c.out.Write(ctxt, result)
}

type cloudList struct {
	public   map[string]*CloudDetails
	builtin  map[string]*CloudDetails
	personal map[string]*CloudDetails
	remote   map[string]*CloudDetails
}

func newCloudList() *cloudList {
	return &cloudList{
		make(map[string]*CloudDetails),
		make(map[string]*CloudDetails),
		make(map[string]*CloudDetails),
		make(map[string]*CloudDetails),
	}
}

func (c *cloudList) len() int {
	return len(c.personal) + len(c.builtin) + len(c.public) + len(c.remote)
}

func (c *cloudList) all() map[string]*CloudDetails {
	if len(c.personal) == 0 && len(c.builtin) == 0 && len(c.public) == 0 && len(c.remote) == 0 {
		return nil
	}

	result := make(map[string]*CloudDetails)
	addAll := func(someClouds map[string]*CloudDetails) {
		for name, cloud := range someClouds {
			result[name] = cloud
		}
	}

	addAll(c.public)
	addAll(c.builtin)
	addAll(c.personal)
	addAll(c.remote)
	return result
}

func (c *cloudList) local() map[string]*CloudDetails {
	if len(c.personal) == 0 && len(c.builtin) == 0 && len(c.public) == 0 {
		return nil
	}

	result := make(map[string]*CloudDetails)
	addAll := func(someClouds map[string]*CloudDetails) {
		for name, cloud := range someClouds {
			result[name] = cloud
		}
	}

	addAll(c.public)
	addAll(c.builtin)
	addAll(c.personal)
	return result
}

func (c *cloudList) filter(all bool) bool {
	if all {
		return false
	}
	if len(c.personal) == 0 && len(c.builtin) == 0 && len(c.public) == 0 && len(c.remote) == 0 {
		return false
	}

	result := false
	examine := func(someClouds map[string]*CloudDetails) {
		for name, cloud := range someClouds {
			if cloud.CredentialCount == 0 {
				result = true
				delete(someClouds, name)
			}
		}
	}

	examine(c.public)
	return result
}

func clientPublicClouds() (map[string]jujucloud.Cloud, error) {
	clouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return clouds, nil
}

func listLocalCloudDetails(store jujuclient.CredentialGetter) (*cloudList, error) {
	clouds, err := clientPublicClouds()
	if err != nil {
		return nil, errors.Trace(err)
	}
	details := newCloudList()
	for name, cloud := range clouds {
		cloudDetails := makeCloudDetails(store, cloud)
		details.public[name] = cloudDetails
	}

	// Add in built in clouds like localhost (lxd).
	builtinClouds, err := common.BuiltInClouds()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for name, cloud := range builtinClouds {
		cloudDetails := makeCloudDetails(store, cloud)
		cloudDetails.Source = "built-in"
		details.builtin[name] = cloudDetails
	}

	personalClouds, err := jujucloud.PersonalCloudMetadata()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for name, cloud := range personalClouds {
		cloudDetails := makeCloudDetails(store, cloud)
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
	w.SetColumnAlignRight(1)

	cloudNamesSorted := func(someClouds map[string]*CloudDetails) []string {
		var cloudNames []string
		for name := range someClouds {
			cloudNames = append(cloudNames, name)
		}
		sort.Strings(cloudNames)
		return cloudNames
	}

	printClouds := func(someClouds map[string]*CloudDetails, showTail bool) {
		cloudNames := cloudNamesSorted(someClouds)

		for _, name := range cloudNames {
			info := someClouds[name]
			defaultRegion := info.DefaultRegion
			if defaultRegion == "" {
				if len(info.Regions) > 0 {
					defaultRegion = info.RegionsMap[info.Regions[0].Key.(string)].Name
				}
			}
			description := info.CloudDescription
			if len(description) > 40 {
				description = description[:39]
			}
			w.Print(name, len(info.Regions), defaultRegion, displayCloudType(info.CloudType))
			if showTail {
				w.Println(info.CredentialCount, info.Source, description)
			} else {
				w.Println()
			}
		}
	}
	var hasRemotes bool
	if len(clouds.remote) > 0 {
		w.Println("\nClouds available on the controller:")
		w.Println("Cloud", "Regions", "Default", "Type")
		printClouds(clouds.remote, false)
		hasRemotes = true
	}
	if localClouds := clouds.local(); len(localClouds) > 0 {
		if !hasRemotes {
			w.Println("You can bootstrap a new controller using one of these clouds...")
		}
		w.Println("\nClouds available on the client:")
		w.Println("Cloud", "Regions", "Default", "Type", "Credentials", "Source", "Description")
		printClouds(localClouds, true)
	}
	tw.Flush()
	return nil
}

func displayCloudType(in string) string {
	if in == "kubernetes" {
		return "k8s"
	}
	return in
}
