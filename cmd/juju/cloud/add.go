// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"io/ioutil"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/interact"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/utils"
	"github.com/juju/utils/cert"
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

	// Ping contains the logic for pinging a cloud endpoint to know whether or
	// not it really has a valid cloud of the same type as the provider.  By
	// default it just calls the correct provider's Ping method.
	Ping func(p environs.EnvironProvider, endpoint string) error

	// CloudCallCtx contains context to be used for any cloud calls.
	CloudCallCtx       *context.CloudCallContext
	cloudMetadataStore CloudMetadataStore
}

// NewAddCloudCommand returns a command to add cloud information.
func NewAddCloudCommand(cloudMetadataStore CloudMetadataStore) *AddCloudCommand {
	cloudCallCtx := context.NewCloudCallContext()
	return &AddCloudCommand{
		cloudMetadataStore: cloudMetadataStore,
		CloudCallCtx:       cloudCallCtx,
		// Ping is provider.Ping except in tests where we don't actually want to
		// require a valid cloud.
		Ping: func(p environs.EnvironProvider, endpoint string) error {
			return p.Ping(cloudCallCtx, endpoint)
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
	f.StringVar(&c.CloudFile, "f", "", "The path to a cloud definition file")
}

// Init populates the command with the args from the command line.
func (c *AddCloudCommand) Init(args []string) (err error) {
	if len(args) > 0 {
		c.Cloud = args[0]
		if ok := names.IsValidCloud(c.Cloud); !ok {
			return errors.NotValidf("cloud name %q", c.Cloud)
		}
	}
	if len(args) > 1 {
		if c.CloudFile != args[1] && c.CloudFile != "" {
			return errors.BadRequestf("cannot specify cloud file with flag and argument")
		}
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

	// first validate cloud input
	data, err := ioutil.ReadFile(c.CloudFile)
	if err != nil {
		return errors.Trace(err)
	}
	if err = cloud.ValidateCloudSet(data); err != nil {
		ctxt.Warningf(err.Error())
	}

	// validate cloud data
	provider, err := environs.Provider(newCloud.Type)
	if err != nil {
		return errors.Trace(err)
	}
	schemas := provider.CredentialSchemas()
	for _, authType := range newCloud.AuthTypes {
		if _, defined := schemas[authType]; !defined {
			return errors.NotSupportedf("auth type %q", authType)
		}
	}
	if err := c.verifyName(c.Cloud); err != nil {
		return errors.Trace(err)
	}

	return addCloud(c.cloudMetadataStore, newCloud)
}

func (c *AddCloudCommand) runInteractive(ctxt *cmd.Context) error {
	errout := interact.NewErrWriter(ctxt.Stdout)
	pollster := interact.New(ctxt.Stdin, ctxt.Stdout, errout)

	cloudType, err := queryCloudType(pollster)
	if err != nil {
		return errors.Trace(err)
	}

	name, err := queryName(c.cloudMetadataStore, c.Cloud, cloudType, pollster)
	if err != nil {
		return errors.Trace(err)
	}

	provider, err := environs.Provider(cloudType)
	if err != nil {
		return errors.Trace(err)
	}

	// At this stage, since we do not have a reference to any model, nor can we get it,
	// nor do we need to have a model for anything that this command does,
	// no cloud credential stored server-side can be invalidated.
	// So, just log an informative message.
	c.CloudCallCtx.InvalidateCredentialFunc = func(reason string) error {
		ctxt.Infof("Cloud credential is not accepted by cloud provider: %v", reason)
		return nil
	}

	// VerifyURLs will return true if a schema format type jsonschema.FormatURI is used
	// and the value will Ping().
	pollster.VerifyURLs = func(s string) (bool, string, error) {
		err := c.Ping(provider, s)
		if err != nil {
			return false, "Can't validate endpoint: " + err.Error(), nil
		}
		return true, "", nil
	}

	// VerifyCertFile will return true if the schema format type "cert-filename" is used
	// and the value is readable and a valid cert file.
	pollster.VerifyCertFile = func(s string) (bool, string, error) {
		out, err := ioutil.ReadFile(s)
		if err != nil {
			return false, "Can't validate CA Certificate file: " + err.Error(), nil
		}
		if _, err := cert.ParseCert(string(out)); err != nil {
			return false, fmt.Sprintf("Can't validate CA Certificate %s: %s", s, err.Error()), nil
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

	filename, alt, err := addCertificate(b)
	switch {
	case errors.IsNotFound(err):
	case err != nil:
		return errors.Annotate(err, "CA Certificate")
	default:
		ctxt.Infof("Successfully read CA Certificate from %s", filename)
		b = alt
	}

	newCloud, err := c.cloudMetadataStore.ParseOneCloud(b)
	if err != nil {
		return errors.Trace(err)
	}
	newCloud.Name = name
	newCloud.Type = cloudType
	if err := addCloud(c.cloudMetadataStore, newCloud); err != nil {
		return errors.Trace(err)
	}
	ctxt.Infof("Cloud %q successfully added", name)
	ctxt.Infof("")
	ctxt.Infof("You will need to add credentials for this cloud (`juju add-credential %s`)", name)
	ctxt.Infof("before creating a controller (`juju bootstrap %s`).", name)

	return nil
}

// addCertificate reads the cloud certificate file if available and adds the contents
// to the byte slice with the appropriate key.  A NotFound error is returned if
// a cloud.CertFilenameKey is not contained in the data, or the value is empty, this is
// not a fatal error.
func addCertificate(data []byte) (string, []byte, error) {
	vals, err := ensureStringMaps(string(data))
	if err != nil {
		return "", nil, err
	}
	name, ok := vals[cloud.CertFilenameKey]
	if !ok {
		return "", nil, errors.NotFoundf("yaml has no certificate file")
	}
	filename := name.(string)
	if ok && filename != "" {
		out, err := ioutil.ReadFile(filename)
		if err != nil {
			return filename, nil, err
		}
		certificate := string(out)
		if _, err := cert.ParseCert(certificate); err != nil {
			return filename, nil, errors.Annotate(err, "bad cloud CA certificate")
		}
		vals["ca-certificates"] = []string{certificate}

	} else {
		return filename, nil, errors.NotFoundf("yaml has no certificate file")
	}
	alt, err := yaml.Marshal(vals)
	return filename, alt, err
}

func ensureStringMaps(in string) (map[string]interface{}, error) {
	userDataMap := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(in), &userDataMap); err != nil {
		return nil, errors.Annotate(err, "must be valid YAML")
	}
	out, err := utils.ConformYAML(userDataMap)
	if err != nil {
		return nil, err
	}
	return out.(map[string]interface{}), nil
}

func queryName(
	cloudMetadataStore CloudMetadataStore,
	cloudName string,
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
		if cloudName == "" {
			name, err := pollster.Enter(fmt.Sprintf("a name for your %s cloud", cloudType))
			if err != nil {
				return "", errors.Trace(err)
			}
			cloudName = name
		}
		if _, ok := personal[cloudName]; ok {
			override, err := pollster.YN(fmt.Sprintf("A cloud named %q already exists. Do you want to replace that definition", cloudName), false)
			if err != nil {
				return "", errors.Trace(err)
			}
			if override {
				return cloudName, nil
			}
			// else, ask again
			cloudName = ""
			continue
		}
		msg, err := nameExists(cloudName, public)
		if err != nil {
			return "", errors.Trace(err)
		}
		if msg == "" {
			return cloudName, nil
		}
		override, err := pollster.YN(msg+", do you want to override that definition", false)
		if err != nil {
			return "", errors.Trace(err)
		}
		if override {
			return cloudName, nil
		}
		// else, ask again
	}
}

// addableCloudProviders returns the names of providers supported by add-cloud,
// and also the names of those which are not supported.
func addableCloudProviders() (providers []string, unsupported []string, _ error) {
	allproviders := environs.RegisteredProviders()
	for _, name := range allproviders {
		provider, err := environs.Provider(name)
		if err != nil {
			// should be impossible
			return nil, nil, errors.Trace(err)
		}

		if provider.CloudSchema() != nil {
			providers = append(providers, name)
		} else {
			unsupported = append(unsupported, name)
		}
	}
	sort.Strings(providers)
	return providers, unsupported, nil
}

func queryCloudType(pollster *interact.Pollster) (string, error) {
	providers, unsupported, err := addableCloudProviders()
	if err != nil {
		// should be impossible
		return "", errors.Trace(err)
	}
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
	msg, err := nameExists(name, public)
	if err != nil {
		return errors.Trace(err)
	}
	if msg != "" {
		return errors.Errorf(msg + "; use --replace to override this definition")
	}
	return nil
}

// nameExists returns either an empty string if the name does not exist, or a
// non-empty string with an error message if it does exist.
func nameExists(name string, public map[string]cloud.Cloud) (string, error) {
	if _, ok := public[name]; ok {
		return fmt.Sprintf("%q is the name of a public cloud", name), nil
	}
	builtin, err := common.BuiltInClouds()
	if err != nil {
		return "", errors.Trace(err)
	}
	if _, ok := builtin[name]; ok {
		return fmt.Sprintf("%q is the name of a built-in cloud", name), nil
	}
	return "", nil
}

func addCloud(cloudMetadataStore CloudMetadataStore, newCloud cloud.Cloud) error {
	personalClouds, err := cloudMetadataStore.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if personalClouds == nil {
		personalClouds = make(map[string]cloud.Cloud)
	}
	personalClouds[newCloud.Name] = newCloud
	return cloudMetadataStore.WritePersonalCloudMetadata(personalClouds)
}
