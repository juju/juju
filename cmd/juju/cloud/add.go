// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	stdcontext "context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/cert"
	"gopkg.in/yaml.v2"

	cloudapi "github.com/juju/juju/api/client/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/interact"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type PersonalCloudMetadataStore interface {
	PersonalCloudMetadata() (map[string]jujucloud.Cloud, error)
	WritePersonalCloudMetadata(cloudsMap map[string]jujucloud.Cloud) error
}

type CloudMetadataStore interface {
	ReadCloudData(path string) ([]byte, error)
	ParseOneCloud(data []byte) (jujucloud.Cloud, error)
	PublicCloudMetadata(searchPaths ...string) (result map[string]jujucloud.Cloud, fallbackUsed bool, _ error)
	PersonalCloudMetadataStore
}

var usageAddCloudSummary = `
Add a cloud definition to Juju.`[1:]

var usageAddCloudDetails = `
Juju needs to know how to connect to clouds. A cloud definition 
describes a cloud's endpoints and authentication requirements. Each
definition is stored and accessed later as <cloud name>.

If you are accessing a public cloud, running add-cloud is unlikely to be 
necessary.  Juju already contains definitions for the public cloud 
providers it supports.

add-cloud operates in two modes:

    juju add-cloud
    juju add-cloud <cloud name> <cloud definition file>

When invoked without arguments, add-cloud begins an interactive session
designed for working with private clouds.  The session will enable you 
to instruct Juju how to connect to your private cloud.

A cloud definition can be provided in a file either as an option -f or as a 
positional argument:

    juju add-cloud mycloud ~/mycloud.yaml
    juju add-cloud mycloud -f ~/mycloud.yaml

When <cloud definition file> is provided with <cloud name>,
Juju will validate the content of the file and add this cloud 
to this client as well as upload it to a controller.

Use --controller option to upload a cloud to a controller. 

Use --client option to add cloud to the current client.

A cloud definition file has the following YAML format:

    clouds:                           # mandatory
      mycloud:                        # <cloud name> argument
        type: openstack               # <cloud type>, see below
        auth-types: [ userpass ]
        regions:
          london:
            endpoint: https://london.mycloud.com:35574/v3.0/

Cloud types for private clouds: 
 - lxd
 - maas
 - manual
 - openstack
 - vsphere

Cloud types for public clouds:
 - azure
 - ec2
 - gce
 - oci

When a running controller is updated, the credential for the cloud
is also uploaded. As with the cloud, the credential needs
to have been added to the current client, use add-credential to
do that. If there's only one credential for the cloud it will be
uploaded to the controller automatically by add-cloud command. 
However, if the cloud has multiple credentials on this client
you can specify which to upload with the --credential option.

When adding clouds to a controller, some clouds are whitelisted and can be easily added:
%v

Other cloud combinations can only be force added as the user must consider
network routability, etc - concerns that are outside of scope of Juju.
When forced addition is desired, use --force.

`

const usageAddCloudExamples = `
    juju add-cloud
    juju add-cloud --force
    juju add-cloud mycloud ~/mycloud.yaml
    juju add-cloud --controller mycontroller mycloud 
    juju add-cloud --controller mycontroller mycloud --credential mycred
    juju add-cloud --client mycloud ~/mycloud.yaml
`

// AddCloudAPI - Implemented by cloudapi.Client.
type AddCloudAPI interface {
	AddCloud(jujucloud.Cloud, bool) error
	AddCredential(tag string, credential jujucloud.Credential) error
	Close() error
}

// AddCloudCommand is the command that allows you to add a cloud configuration
// for use with juju bootstrap.
type AddCloudCommand struct {
	modelcmd.OptionalControllerCommand

	// Cloud is the name of the cloud to add.
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

	// These attributes are used when adding a cloud to a controller.
	credentialName  string
	addCloudAPIFunc func() (AddCloudAPI, error)

	// Force holds whether user wants to force addition of the cloud.
	Force bool

	// existsLocally whether this cloud already exists locally.
	existsLocally bool

	// targetController holds a controller name when adding a cloud
	// to a controller managed by JAAS.
	targetController string
}

// NewAddCloudCommand returns a command to add cloud information.
func NewAddCloudCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	cloudCallCtx := context.NewCloudCallContext(stdcontext.Background())
	store := jujuclient.NewFileClientStore()
	c := &AddCloudCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store: store,
		},
		cloudMetadataStore: cloudMetadataStore,
		CloudCallCtx:       cloudCallCtx,
		// Ping is provider.Ping except in tests where we don't actually want to
		// require a valid cloud.
		Ping: func(p environs.EnvironProvider, endpoint string) error {
			return p.Ping(cloudCallCtx, endpoint)
		},
	}
	c.addCloudAPIFunc = c.cloudAPI
	return modelcmd.WrapBase(c)
}

func (c *AddCloudCommand) cloudAPI() (AddCloudAPI, error) {
	root, err := c.NewAPIRoot(c.Store, c.ControllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloudapi.NewClient(root), nil
}

// Info returns help information about the command.
func (c *AddCloudCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-cloud",
		Args:     "<cloud name> [<cloud definition file>]",
		Purpose:  usageAddCloudSummary,
		Doc:      fmt.Sprintf(usageAddCloudDetails, jujucloud.CurrentWhiteList()),
		Examples: usageAddCloudExamples,
		SeeAlso: []string{
			"clouds",
			"update-cloud",
			"remove-cloud",
			"update-credential",
		},
	})
}

// SetFlags initializes the flags supported by the command.
func (c *AddCloudCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "Force add cloud to the controller")
	f.StringVar(&c.CloudFile, "f", "", "The path to a cloud definition file")
	f.StringVar(&c.CloudFile, "file", "", "The path to a cloud definition file")
	f.StringVar(&c.credentialName, "credential", "", "Credential to use for new cloud")
	f.StringVar(&c.targetController, "target-controller", "", "The name of a JAAS managed controller to add a cloud to")
}

// Init populates the command with the args from the command line.
func (c *AddCloudCommand) Init(args []string) (err error) {
	if c.targetController != "" {
		return cmd.ErrCommandMissing
	}

	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
	if len(args) > 0 {
		c.Cloud = args[0]
		if ok := names.IsValidCloud(c.Cloud); !ok {
			return errors.NotValidf("cloud name %q", c.Cloud)
		}
	}
	if len(args) > 1 {
		if c.CloudFile != args[1] && c.CloudFile != "" {
			return errors.BadRequestf("cannot specify cloud file with option and argument")
		}
		c.CloudFile = args[1]
	}
	if len(args) > 2 {
		return cmd.CheckEmpty(args[2:])
	}
	return nil
}

var ambiguousCredentialError = errors.New(`
more than one credential is available
specify a credential using the --credential argument`[1:],
)

func (c *AddCloudCommand) findLocalCredential(ctx *cmd.Context, cloud jujucloud.Cloud, credentialName string) (*jujucloud.Credential, string, error) {
	credential, chosenCredentialName, _, err := modelcmd.GetCredentials(ctx, c.Store, modelcmd.GetCredentialsParams{
		Cloud:          cloud,
		CredentialName: credentialName,
	})
	if err == nil {
		return credential, chosenCredentialName, nil
	}

	switch errors.Cause(err) {
	case modelcmd.ErrMultipleCredentials:
		return nil, "", ambiguousCredentialError
	}
	return nil, "", errors.Trace(err)
}

func (c *AddCloudCommand) addCredentialToController(ctx *cmd.Context, cloud jujucloud.Cloud, apiClient AddCloudAPI) error {
	_, err := c.Store.ControllerByName(c.ControllerName)
	if err != nil {
		return errors.Trace(err)
	}

	currentAccountDetails, err := c.Store.AccountDetails(c.ControllerName)
	if err != nil {
		return errors.Trace(err)
	}

	cred, credentialName, err := c.findLocalCredential(ctx, cloud, c.credentialName)
	if err != nil {
		return errors.Trace(err)
	}

	id := fmt.Sprintf("%s/%s/%s", c.Cloud, currentAccountDetails.User, credentialName)
	if !names.IsValidCloudCredential(id) {
		return errors.NotValidf("cloud credential ID %q", id)
	}
	cloudCredTag := names.NewCloudCredentialTag(id)

	if err := apiClient.AddCredential(cloudCredTag.String(), *cred); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Run executes the add cloud command, adding a cloud based on a passed-in yaml
// file or interactive queries.
func (c *AddCloudCommand) Run(ctxt *cmd.Context) error {
	if err := c.MaybePrompt(ctxt, "add cloud to"); err != nil {
		return errors.Trace(err)
	}
	var newCloud *jujucloud.Cloud
	var err error
	if c.CloudFile != "" {
		newCloud, err = c.readCloudFromFile(ctxt)
	} else {
		if c.Cloud != "" {
			// It's possible that the user wants to add an existing local cloud to a current controller,
			// i.e. 'juju add-cloud aws'. So let's see if we can find the cloud.
			newCloud, err = common.CloudByName(c.Cloud)
			c.existsLocally = err == nil
		}
		if !c.existsLocally {
			newCloud, err = c.runInteractive(ctxt)
		}
	}
	if err != nil {
		return errors.Trace(err)
	}

	// All clouds must have at least one default region - lp#1819409.
	if len(newCloud.Regions) == 0 {
		newCloud.Regions = []jujucloud.Region{{Name: jujucloud.DefaultCloudRegion}}
	}

	var returnErr error
	if c.Client && !c.existsLocally {
		returnErr = c.addLocalCloud(ctxt, newCloud)
	}
	if c.Client && c.existsLocally {
		returnErr = errors.AlreadyExistsf("use `update-cloud %s --client` to override known definition: local cloud %q", newCloud.Name, newCloud.Name)
	}

	if c.Client && c.ControllerName != "" {
		ctxt.Infof("")
	}

	if c.ControllerName != "" {
		if err = c.addRemoteCloud(ctxt, newCloud); err != nil {
			ctxt.Infof("Could not upload cloud to a controller: %v", err)
			returnErr = cmd.ErrSilent
		}
	}
	return returnErr
}

func (c *AddCloudCommand) addRemoteCloud(ctxt *cmd.Context, newCloud *jujucloud.Cloud) error {
	// A controller has been specified so upload the cloud details
	// plus a corresponding credential to the controller.
	api, err := c.addCloudAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()
	err = api.AddCloud(*newCloud, c.Force)
	if err != nil {
		if params.ErrCode(err) == params.CodeAlreadyExists {
			ctxt.Infof("Cloud %q already exists on the controller %q.", c.Cloud, c.ControllerName)
			ctxt.Infof("To upload a credential to the controller for cloud %q, use \n"+
				"* 'add-model' with --credential option or\n"+
				"* 'add-credential -c %v'.", newCloud.Name, newCloud.Name)
			return nil
		}
		if params.ErrCode(err) == params.CodeIncompatibleClouds {
			logger.Infof("%v", err)
			ctxt.Infof("Adding a cloud of type %q might not function correctly on this controller.\n"+
				"If you really want to do this, use --force.", newCloud.Type)
			return nil
		}
		return err
	}
	ctxt.Infof("Cloud %q added to controller %q.", c.Cloud, c.ControllerName)
	// Add a credential for the newly added cloud.
	err = c.addCredentialToController(ctxt, *newCloud, api)
	if err != nil {
		logger.Warningf("%v", err)
		ctxt.Infof("To upload a credential to the controller for cloud %q, use \n"+
			"* 'add-model' with --credential option or\n"+
			"* 'add-credential -c %v'.", newCloud.Name, newCloud.Name)
		return nil
	}
	ctxt.Infof("Credential for cloud %q added to controller %q.", c.Cloud, c.ControllerName)
	return nil
}

func (c *AddCloudCommand) addLocalCloud(ctxt *cmd.Context, newCloud *jujucloud.Cloud) error {
	operation := "added"
	if err := addLocalCloud(c.cloudMetadataStore, *newCloud); err != nil {
		ctxt.Infof("Cloud %q was not %v locally: %v", newCloud.Name, operation, err)
		return cmd.ErrSilent
	}
	ctxt.Infof("Cloud %q successfully %v to your local client.", newCloud.Name, operation)
	if len(newCloud.AuthTypes) != 0 {
		ctxt.Infof("You will need to add a credential for this cloud (`juju add-credential %s`)", newCloud.Name)
		ctxt.Infof("before you can use it to bootstrap a controller (`juju bootstrap %s`) or", newCloud.Name)
		ctxt.Infof("to create a model (`juju add-model <your model name> %s`).", newCloud.Name)
	}
	return nil
}

func cloudFromLocal(store jujuclient.CredentialGetter, cloudName string) (*jujucloud.Cloud, error) {
	details, err := listLocalCloudDetails(store)
	if err != nil {
		return nil, err
	}
	cloudDetails, ok := details.all()[cloudName]
	if !ok {
		return nil, errors.NotFoundf("cloud %q", cloudName)
	}
	newCloud := &jujucloud.Cloud{
		Name:             cloudName,
		Type:             cloudDetails.CloudType,
		Description:      cloudDetails.CloudDescription,
		Endpoint:         cloudDetails.Endpoint,
		IdentityEndpoint: cloudDetails.IdentityEndpoint,
		StorageEndpoint:  cloudDetails.StorageEndpoint,
		CACertificates:   cloudDetails.CACredentials,
		SkipTLSVerify:    cloudDetails.SkipTLSVerify,
		Config:           cloudDetails.Config,
		RegionConfig:     cloudDetails.RegionConfig,
	}
	for _, at := range cloudDetails.AuthTypes {
		newCloud.AuthTypes = append(newCloud.AuthTypes, jujucloud.AuthType(at))
	}
	for name, r := range cloudDetails.RegionsMap {
		newCloud.Regions = append(newCloud.Regions, jujucloud.Region{
			Name:             name,
			Endpoint:         r.Endpoint,
			StorageEndpoint:  r.StorageEndpoint,
			IdentityEndpoint: r.IdentityEndpoint,
		})
	}
	return newCloud, nil
}

func (c *AddCloudCommand) readCloudFromFile(ctxt *cmd.Context) (*jujucloud.Cloud, error) {
	r := &CloudFileReader{
		CloudMetadataStore: c.cloudMetadataStore,
		CloudName:          c.Cloud,
	}
	newCloud, err := r.ReadCloudFromFile(c.CloudFile, ctxt)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.Cloud = r.CloudName
	c.existsLocally = r.alreadyExists
	return newCloud, nil
}

func (c *AddCloudCommand) runInteractive(ctxt *cmd.Context) (*jujucloud.Cloud, error) {
	errout := interact.NewErrWriter(ctxt.Stdout)
	pollster := interact.New(ctxt.Stdin, ctxt.Stdout, errout)

	cloudType, err := queryCloudType(pollster)
	if err != nil {
		return nil, errors.Trace(err)
	}

	name, err := queryName(c.cloudMetadataStore, c.Cloud, cloudType, pollster)
	if err != nil {
		return nil, errors.Trace(err)
	}

	provider, err := environs.Provider(cloudType)
	if err != nil {
		return nil, errors.Trace(err)
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
		out, err := os.ReadFile(s)
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
		return nil, errors.Trace(err)
	}
	b, err := yaml.Marshal(v)
	if err != nil {
		return nil, errors.Trace(err)
	}

	filename, alt, err := addCertificate(b)
	switch {
	case errors.IsNotFound(err):
	case err != nil:
		return nil, errors.Annotate(err, "CA Certificate")
	default:
		ctxt.Infof("Successfully read CA Certificate from %s", filename)
		b = alt
	}

	newCloud, err := c.cloudMetadataStore.ParseOneCloud(b)
	if err != nil {
		return nil, errors.Trace(err)
	}
	newCloud.Name = name
	newCloud.Type = cloudType
	return &newCloud, nil
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
	name, ok := vals[jujucloud.CertFilenameKey]
	if !ok {
		return "", nil, errors.NotFoundf("yaml has no certificate file")
	}
	filename := name.(string)
	if ok && filename != "" {
		out, err := os.ReadFile(filename)
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

	verifyCloudName := func(name string) (bool, string, error) {
		if names.IsValidCloud(name) {
			return true, "", nil
		}
		return false, "Invalid name. Valid names start with a letter, and use only letters, numbers, hyphens, and underscores: ", nil
	}

	for {
		if cloudName == "" {
			name, err := pollster.EnterVerify(fmt.Sprintf("a name for your %s cloud", cloudType), verifyCloudName)
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
			if strings.EqualFold(name, s) {
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

func addLocalCloud(cloudMetadataStore PersonalCloudMetadataStore, newCloud jujucloud.Cloud) error {
	personalClouds, err := cloudMetadataStore.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if personalClouds == nil {
		personalClouds = make(map[string]jujucloud.Cloud)
	}
	personalClouds[newCloud.Name] = newCloud
	return cloudMetadataStore.WritePersonalCloudMetadata(personalClouds)
}

// CloudFileReader defines a struct used to read a cloud definition from a file.
type CloudFileReader struct {
	CloudMetadataStore CloudMetadataStore
	CloudName          string
	alreadyExists      bool
}

// ReadCloudFromFile reads the cloud definition from the specified file.
func (p *CloudFileReader) ReadCloudFromFile(cloudFile string, ctxt *cmd.Context) (*jujucloud.Cloud, error) {
	data, err := p.CloudMetadataStore.ReadCloudData(cloudFile)
	if err != nil {
		return nil, errors.Trace(err)
	}
	specifiedClouds, err := jujucloud.ParseCloudMetadata(data)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(specifiedClouds) == 0 {
		return nil, errors.New("no clouds found in parsed yaml, please validate yaml keys")
	}

	var newCloud jujucloud.Cloud
	if p.CloudName == "" {
		if len(specifiedClouds) == 1 {
			for k, v := range specifiedClouds {
				newCloud = v
				// User did not specify cloud name aka as a command argument,
				// use what is in th file.
				p.CloudName = k
			}
		} else {
			if p.CloudName == "" {
				return nil, errors.Errorf("there is more than one cloud defined in file %q, specify a cloud name to select one", cloudFile)
			}
		}
	} else {
		var ok bool
		newCloud, ok = specifiedClouds[p.CloudName]
		if !ok {
			return nil, errors.Errorf("cloud %q not found in file %q", p.CloudName, cloudFile)
		}
	}

	// first validate cloud input
	if err = jujucloud.ValidateCloudSet(data); err != nil {
		ctxt.Warningf(err.Error())
	}

	// validate cloud data
	provider, err := environs.Provider(newCloud.Type)
	if err != nil {
		return nil, errors.Trace(err)
	}
	schemas := provider.CredentialSchemas()
	for _, authType := range newCloud.AuthTypes {
		if _, defined := schemas[authType]; !defined {
			return nil, errors.NotSupportedf("auth type %q", authType)
		}
	}
	if err := p.verifyName(p.CloudName); err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, errors.Trace(err)
		}
		p.alreadyExists = true
	}
	return &newCloud, nil
}

func (p *CloudFileReader) verifyName(name string) error {
	public, _, err := p.CloudMetadataStore.PublicCloudMetadata()
	if err != nil {
		return err
	}
	personal, err := p.CloudMetadataStore.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if _, ok := personal[name]; ok {
		return errors.AlreadyExistsf("use `update-cloud %s --client` to replace this cloud locally: %q", name, name)
	}
	msg, err := nameExists(name, public)
	if err != nil {
		return errors.Trace(err)
	}
	if msg != "" {
		return errors.AlreadyExistsf(msg + "; use `update-cloud --client` to override this definition locally")
	}
	return nil
}

// nameExists returns either an empty string if the name does not exist, or a
// non-empty string with an error message if it does exist.
func nameExists(name string, public map[string]jujucloud.Cloud) (string, error) {
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
