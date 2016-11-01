// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"sort"
	"strings"

	yaml "gopkg.in/yaml.v1"

	"github.com/juju/ansiterm"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/jsonschema"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/interact"
	"github.com/juju/juju/environs"
)

var usageAddCloudSummary = `
Adds a user-defined cloud to Juju from among known cloud types.`[1:]

var usageAddCloudDetails = `
A cloud definition file has the following YAML format:

clouds:
  mycloud:
    type: openstack
    auth-types: [ userpass ]
    regions:
      london:
        endpoint: https://london.mycloud.com:35574/v3.0/

If the named cloud already exists, the `[1:] + "`--replace`" + ` option is required to 
overwrite its configuration.
Known cloud types: azure, cloudsigma, ec2, gce, joyent, lxd, maas, manual,
openstack, rackspace

Examples:
    juju add-cloud mycloud ~/mycloud.yaml

See also: 
    clouds`

type addCloudCommand struct {
	cmd.CommandBase

	// Replace, if true, existing cloud information is overwritten.
	Replace bool

	// Cloud is the name fo the cloud to add.
	Cloud string

	// CloudFile is the name of the cloud YAML file.
	CloudFile string
}

// NewAddCloudCommand returns a command to add cloud information.
func NewAddCloudCommand() cmd.Command {
	return &addCloudCommand{}
}

func (c *addCloudCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-cloud",
		Args:    "<cloud name> <cloud definition file>",
		Purpose: usageAddCloudSummary,
		Doc:     usageAddCloudDetails,
	}
}

func (c *addCloudCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.Replace, "replace", false, "Overwrite any existing cloud information")
}

func (c *addCloudCommand) Init(args []string) (err error) {
	if len(args) > 0 {
		c.Cloud = args[0]
	}
	if len(args) > 1 {
		c.CloudFile = args[1]
	}
	if len(args) > 2 {
		return cmd.CheckEmpty(args[2:])
	}
	return nil
}

func (c *addCloudCommand) Run(ctxt *cmd.Context) error {
	if c.CloudFile == "" {
		return c.runInteractive(ctxt)
	}
	specifiedClouds, err := cloud.ParseCloudMetadataFile(c.CloudFile)
	if err != nil {
		return err
	}
	if specifiedClouds == nil {
		return errors.New("no personal clouds are defined")
	}
	newCloud, ok := specifiedClouds[c.Cloud]
	if !ok {
		return errors.Errorf("cloud %q not found in file %q", c.Cloud, c.CloudFile)
	}

	if err := c.verifyName(c.Cloud); err != nil {
		return errors.Trace(err)
	}

	return addCloud(c.Cloud, newCloud)
}

func (c *addCloudCommand) runInteractive(ctxt *cmd.Context) error {
	allproviders := environs.RegisteredProviders()

	public, _, err := cloud.PublicCloudMetadata()
	if err != nil {
		return err
	}
	personal, err := cloud.PersonalCloudMetadata()
	if err != nil {
		return err
	}

	var unsupported []string
	var providers []string
	for _, name := range allproviders {
		provider, err := environs.Provider(name)
		if err != nil {
			// should be impossible
			return errors.Trace(err)
		}

		if provider.CloudSchema() != nil {
			providers = append(providers, name)
		} else {
			unsupported = append(unsupported, name)
		}
	}
	sort.Strings(providers)

	supportedCloud := interact.VerifyOptions("cloud type", providers, false)

	cloudVerify := func(s string) (ok bool, errmsg string, err error) {
		ok, errmsg, err = supportedCloud(s)
		if err != nil {
			return false, "", errors.Trace(err)
		}
		if ok {
			return true, "", nil
		}
		// Print out a different message if they entered a valid provider that
		// just isn't something we want people to add (like ec2).
		for _, name := range unsupported {
			if strings.ToLower(name) == strings.ToLower(s) {
				return false, fmt.Sprintf("Cloud type %q not supported for interactive add-cloud.", s), nil
			}
		}
		return false, errmsg, nil
	}

	w := ansiterm.NewWriter(ctxt.Stdout)
	pollster := interact.New(ctxt.Stdin, ctxt.Stdout, errWriter{w})
	cloudType, err := pollster.SelectVerify(interact.List{
		Singular: "cloud type",
		Plural:   "cloud types",
		Options:  providers,
	}, cloudVerify)

	if err != nil {
		return errors.Trace(err)
	}

	var name string
	for {
		name, err = pollster.Enter("a name for the cloud")
		if err != nil {
			return errors.Trace(err)
		}
		err = nameExists(name, public, personal)
		if err == nil {
			break
		}
		// we do this instead of returning the error message
		override, err2 := pollster.YN(err.Error()+", do you want to override that definition", false)
		if err2 != nil {
			return errors.Trace(err)
		}
		if override {
			break
		}
		// else, ask again
	}

	provider, err := environs.Provider(cloudType)
	if err != nil {
		return errors.Trace(err)
	}

	schema := provider.CloudSchema()
	v, err := pollster.QuerySchema(schema)
	if err != nil {
		return errors.Trace(err)
	}
	b, err := yaml.Marshal(v)
	if err != nil {
		return errors.Trace(err)
	}
	newCloud, err := cloud.ParseOneCloud(b)
	if err != nil {
		return errors.Trace(err)
	}
	newCloud.Type = cloudType
	err = addCloud(name, newCloud)
	if err == nil {
		ctxt.Infof("Cloud %q successfully added", name)
		ctxt.Infof("You may bootstrap with 'juju bootstrap %s'", name)
	}
	return err
}

type errWriter struct {
	w *ansiterm.Writer
}

func (w errWriter) Write(b []byte) (n int, err error) {
	w.w.SetForeground(ansiterm.BrightRed)
	defer w.w.Reset()
	return w.w.Write(b)
}

func (c *addCloudCommand) verifyName(name string) error {
	if c.Replace {
		return nil
	}
	public, _, err := cloud.PublicCloudMetadata()
	if err != nil {
		return err
	}
	personal, err := cloud.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if err := nameExists(name, public, personal); err != nil {
		return errors.Errorf(err.Error() + "; use --replace to override this definition")
	}
	return nil
}

func nameExists(name string, public, personal map[string]cloud.Cloud) error {
	if _, ok := public[name]; ok {
		return errors.Errorf("%q is the name of a public cloud", name)
	}
	builtinClouds := common.BuiltInClouds()
	if _, ok := builtinClouds[name]; ok {
		return errors.Errorf("%q is the name of a built-in cloud", name)
	}
	if _, ok := personal[name]; ok {
		return errors.Errorf("%q already exists", name)
	}
	return nil
}

func querySchema(schema *jsonschema.Schema, pollster *interact.Pollster) error {
	return nil
}

func addCloud(name string, newCloud cloud.Cloud) error {
	personalClouds, err := cloud.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if personalClouds == nil {
		personalClouds = make(map[string]cloud.Cloud)
	}
	personalClouds[name] = newCloud
	return cloud.WritePersonalCloudMetadata(personalClouds)
}
