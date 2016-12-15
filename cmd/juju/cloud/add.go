// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"sort"
	"strings"

	yaml "gopkg.in/yaml.v1"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/interact"
	"github.com/juju/juju/environs"
)

type CloudMetadataStore interface {
	ParseCloudMetadataFile(path string) (map[string]cloud.Cloud, error)
	ParseOneCloud(data []byte) (cloud.Cloud, error)
	PublicCloudMetadata(searchPaths ...string) (result map[string]cloud.Cloud, fallbackUsed bool, _ error)
	PersonalCloudMetadata() (map[string]cloud.Cloud, error)
	WritePersonalCloudMetadata(cloudsMap map[string]cloud.Cloud) error
}

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

// AddCloudCommand is the command that allows you to add a cloud configuration
// for use with juju bootstrap.
type AddCloudCommand struct {
	cmd.CommandBase

	// Replace, if true, existing cloud information is overwritten.
	Replace bool

	// Cloud is the name fo the cloud to add.
	Cloud string

	// CloudFile is the name of the cloud YAML file.
	CloudFile string

	cloudMetadataStore CloudMetadataStore

	// Ping contains the logic for pinging a cloud endpoint to know whether or
	// not it really has a valid cloud of the same type as the provider.  By
	// default it just calls the correct provider's Ping method.
	Ping func(p environs.EnvironProvider, endpoint string) error
}

// NewAddCloudCommand returns a command to add cloud information.
func NewAddCloudCommand(cloudMetadataStore CloudMetadataStore) *AddCloudCommand {
	// Ping is provider.Ping except in tests where we don't actually want to
	// require a valid cloud.
	return &AddCloudCommand{
		cloudMetadataStore: cloudMetadataStore,
		Ping: func(p environs.EnvironProvider, endpoint string) error {
			return p.Ping(endpoint)
		},
	}
}

// Info returns help information about the command.
func (c *AddCloudCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-cloud",
		Args:    "<cloud name> <cloud definition file>",
		Purpose: usageAddCloudSummary,
		Doc:     usageAddCloudDetails,
	}
}

// SetFlags initializes the flags supported by the command.
func (c *AddCloudCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.Replace, "replace", false, "Overwrite any existing cloud information")
}

// Init populates the command with the args from the command line.
func (c *AddCloudCommand) Init(args []string) (err error) {
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

// Run executes the add cloud command, adding a cloud based on a passed-in yaml
// file or interactive queries.
func (c *AddCloudCommand) Run(ctxt *cmd.Context) error {
	if c.CloudFile == "" {
		return c.runInteractive(ctxt)
	}
	specifiedClouds, err := c.cloudMetadataStore.ParseCloudMetadataFile(c.CloudFile)
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

	return addCloud(c.cloudMetadataStore, c.Cloud, newCloud)
}

func (c *AddCloudCommand) runInteractive(ctxt *cmd.Context) error {
	errout := interact.NewErrWriter(ctxt.Stdout)
	pollster := interact.New(ctxt.Stdin, ctxt.Stdout, errout)

	cloudType, err := queryCloudType(pollster)
	if err != nil {
		return errors.Trace(err)
	}

	name, err := queryName(c.cloudMetadataStore, cloudType, pollster)
	if err != nil {
		return errors.Trace(err)
	}

	provider, err := environs.Provider(cloudType)
	if err != nil {
		return errors.Trace(err)
	}

	pollster.VerifyURLs = func(s string) (ok bool, msg string, err error) {
		err = c.Ping(provider, s)
		if err != nil {
			return false, "Can't validate endpoint: " + err.Error(), nil
		}
		return true, "", nil
	}

	v, err := pollster.QuerySchema(provider.CloudSchema())
	if err != nil {
		return errors.Trace(err)
	}
	b, err := yaml.Marshal(v)
	if err != nil {
		return errors.Trace(err)
	}
	newCloud, err := c.cloudMetadataStore.ParseOneCloud(b)
	if err != nil {
		return errors.Trace(err)
	}
	newCloud.Type = cloudType
	if err := addCloud(c.cloudMetadataStore, name, newCloud); err != nil {
		return errors.Trace(err)
	}
	ctxt.Infof("Cloud %q successfully added", name)
	ctxt.Infof("You may bootstrap with 'juju bootstrap %s'", name)

	return nil
}

func queryName(
	cloudMetadataStore CloudMetadataStore,
	cloudType string,
	pollster *interact.Pollster,
) (string, error) {
	public, _, err := cloudMetadataStore.PublicCloudMetadata()
	if err != nil {
		return "", err
	}
	personal, err := cloudMetadataStore.PersonalCloudMetadata()
	if err != nil {
		return "", err
	}

	for {
		name, err := pollster.Enter(fmt.Sprintf("a name for your %s cloud", cloudType))
		if err != nil {
			return "", errors.Trace(err)
		}
		if _, ok := personal[name]; ok {
			override, err := pollster.YN(fmt.Sprintf("A cloud named %q already exists. Do you want to replace that definition", name), false)
			if err != nil {
				return "", errors.Trace(err)
			}
			if override {
				return name, nil
			}
			// else, ask again
			continue
		}
		msg := nameExists(name, public)
		if msg == "" {
			return name, nil
		}
		override, err := pollster.YN(msg+", do you want to override that definition", false)
		if err != nil {
			return "", errors.Trace(err)
		}
		if override {
			return name, nil
		}
		// else, ask again
	}
}

func queryCloudType(pollster *interact.Pollster) (string, error) {
	allproviders := environs.RegisteredProviders()
	var unsupported []string
	var providers []string
	for _, name := range allproviders {
		provider, err := environs.Provider(name)
		if err != nil {
			// should be impossible
			return "", errors.Trace(err)
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

	return pollster.SelectVerify(interact.List{
		Singular: "cloud type",
		Plural:   "cloud types",
		Options:  providers,
	}, cloudVerify)
}

func (c *AddCloudCommand) verifyName(name string) error {
	if c.Replace {
		return nil
	}
	public, _, err := c.cloudMetadataStore.PublicCloudMetadata()
	if err != nil {
		return err
	}
	personal, err := c.cloudMetadataStore.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if _, ok := personal[name]; ok {
		return errors.Errorf("%q already exists; use --replace to replace this existing cloud", name)
	}
	if msg := nameExists(name, public); msg != "" {
		return errors.Errorf(msg + "; use --replace to override this definition")
	}
	return nil
}

// nameExists returns either an empty string if the name does not exist, or a
// non-empty string with an error message if it does exist.
func nameExists(name string, public map[string]cloud.Cloud) string {
	if _, ok := public[name]; ok {
		return fmt.Sprintf("%q is the name of a public cloud", name)
	}
	if _, ok := common.BuiltInClouds()[name]; ok {
		return fmt.Sprintf("%q is the name of a built-in cloud", name)
	}
	return ""
}

func addCloud(cloudMetadataStore CloudMetadataStore, name string, newCloud cloud.Cloud) error {
	personalClouds, err := cloudMetadataStore.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if personalClouds == nil {
		personalClouds = make(map[string]cloud.Cloud)
	}
	personalClouds[name] = newCloud
	return cloudMetadataStore.WritePersonalCloudMetadata(personalClouds)
}
