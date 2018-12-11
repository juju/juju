// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/juju/names.v2"

	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
)

var logger = loggo.GetLogger("juju.cmd.juju.k8s")

type CloudMetadataStore interface {
	ParseCloudMetadataFile(path string) (map[string]jujucloud.Cloud, error)
	ParseOneCloud(data []byte) (jujucloud.Cloud, error)
	PublicCloudMetadata(searchPaths ...string) (result map[string]jujucloud.Cloud, fallbackUsed bool, _ error)
	PersonalCloudMetadata() (map[string]jujucloud.Cloud, error)
	WritePersonalCloudMetadata(cloudsMap map[string]jujucloud.Cloud) error
}

// AddCloudAPI - Implemented by cloudapi.Client
type AddCloudAPI interface {
	AddCloud(jujucloud.Cloud) error
	AddCredential(tag string, credential jujucloud.Credential) error
	Close() error
}

// ModelConfigAPI - Implemented by modelconfig.Client
type ModelConfigAPI interface {
	ModelGet() (map[string]interface{}, error)
	Close() error
}

var usageAddCAASSummary = `
Adds a k8s endpoint and credential to Juju.`[1:]

var usageAddCAASDetails = `
Creates a user-defined cloud and populate the selected controller with the k8s
cloud details. Speficify non default kubeconfig file location using $KUBECONFIG
environment variable or pipe in file content from stdin. The config file
can contain definitions for different k8s clusters, use --cluster-name to pick
which one to use.

Examples:
    juju add-k8s myk8scloud
    KUBECONFIG=path-to-kubuconfig-file juju add-k8s myk8scloud --cluster-name=my_cluster_name
    kubectl config view --raw | juju add-k8s myk8scloud --cluster-name=my_cluster_name

See also:
    remove-k8s
`

// AddCAASCommand is the command that allows you to add a caas and credential
type AddCAASCommand struct {
	modelcmd.ControllerCommandBase

	// caasName is the name of the caas to add.
	caasName string

	// caasType is the type of CAAS being added
	caasType string

	// clusterName is the name of the cluster (k8s) or credential to import
	clusterName string

	// region is the cloud region that the nodes of cluster (k8s) are running in.
	// The format is <cloudName/region>
	cloudRegion string

	// cloudName is name of the cloud.
	cloudName string

	cloudMetadataStore    CloudMetadataStore
	fileCredentialStore   jujuclient.CredentialStore
	cloudAPIFunc          func() (AddCloudAPI, error)
	modelConfigAPIFunc    func() (ModelConfigAPI, error)
	newClientConfigReader func(string) (clientconfig.ClientConfigFunc, error)
}

// NewAddCAASCommand returns a command to add caas information.
func NewAddCAASCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	cmd := &AddCAASCommand{
		cloudMetadataStore:  cloudMetadataStore,
		fileCredentialStore: jujuclient.NewFileCredentialStore(),
		newClientConfigReader: func(caasType string) (clientconfig.ClientConfigFunc, error) {
			return clientconfig.NewClientConfigReader(caasType)
		},
	}
	cmd.cloudAPIFunc = func() (AddCloudAPI, error) {
		root, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cloudapi.NewClient(root), nil
	}

	cmd.modelConfigAPIFunc = func() (ModelConfigAPI, error) {
		api, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return modelconfig.NewClient(api), nil
	}
	return modelcmd.WrapController(cmd)
}

// Info returns help information about the command.
func (c *AddCAASCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-k8s",
		Args:    "<k8s name>",
		Purpose: usageAddCAASSummary,
		Doc:     usageAddCAASDetails,
	}
}

// SetFlags initializes the flags supported by the command.
func (c *AddCAASCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.clusterName, "cluster-name", "", "Specify the k8s cluster to import")
	// f.Var(regionFlag{region: &c.region, cloudName: &c.cloudName}, "region", "kubernetes cluster cloud region")
	f.StringVar(&c.cloudRegion, "region", "", "kubernetes cluster cloud region")
}

// Init populates the command with the args from the command line.
func (c *AddCAASCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.Errorf("missing k8s name.")
	}
	c.caasType = "kubernetes"
	c.caasName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// getStdinPipe returns nil if the context's stdin is not a pipe.
func getStdinPipe(ctx *cmd.Context) (io.Reader, error) {
	if stdIn, ok := ctx.Stdin.(*os.File); ok && !terminal.IsTerminal(int(stdIn.Fd())) {
		// stdIn from pipe but not terminal
		stat, err := stdIn.Stat()
		if err != nil {
			return nil, err
		}
		content, err := ioutil.ReadAll(stdIn)
		if err != nil {
			return nil, err
		}
		if (stat.Mode()&os.ModeCharDevice) == 0 && len(content) > 0 {
			// workaround to get piped stdIn size because stat.Size() always == 0
			return bytes.NewReader(content), nil
		}
	}
	return nil, nil
}

// Run is defined on the Command interface.
func (c *AddCAASCommand) Run(ctx *cmd.Context) error {
	if err := c.verifyName(c.caasName); err != nil {
		return errors.Trace(err)
	}

	clientConfigFunc, err := c.newClientConfigReader(c.caasType)
	if err != nil {
		return errors.Trace(err)
	}
	stdIn, err := getStdinPipe(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	caasConfig, err := clientConfigFunc(stdIn)
	logger.Debugf("caasConfig: %+v", caasConfig)
	if err != nil {
		return errors.Trace(err)
	}

	if len(caasConfig.Contexts) == 0 {
		return errors.Errorf("No k8s cluster definitions found in config")
	}

	var context clientconfig.Context
	clusterName := c.clusterName
	if clusterName != "" {
		for _, c := range caasConfig.Contexts {
			if clusterName == c.CloudName {
				context = c
				break
			}
		}
	} else {
		context, _ = caasConfig.Contexts[caasConfig.CurrentContext]
		logger.Debugf("No cluster name specified, so use current context %q", caasConfig.CurrentContext)
	}

	if (clientconfig.Context{}) == context {
		return errors.NotFoundf("clusterName %q", clusterName)
	}
	credential := caasConfig.Credentials[context.CredentialName]
	currentCloud := caasConfig.Clouds[context.CloudName]

	cloudCAData, ok := currentCloud.Attributes["CAData"].(string)
	if !ok {
		return errors.Errorf("CAData attribute should be a string")
	}

	newCloud := jujucloud.Cloud{
		Name:           c.caasName,
		Type:           c.caasType,
		Endpoint:       currentCloud.Endpoint,
		AuthTypes:      []jujucloud.AuthType{credential.AuthType()},
		CACertificates: []string{cloudCAData},
	}

	cloudRegion := c.cloudRegion
	if cloudRegion == "" {
		cloudRegion, err = c.getClusterRegion(ctx, newCloud, credential)
		if err != nil {
			logger.Debugf("It's not possible to fetch cluster region in this case, error: %v\n\tplease use --region options to parse in if you want to.", err)
		}
	}
	if cloudRegion != "" {
		// do validation if cloudRegion is presented or ignore because it's for jaas only but not juju.
		if err = c.validateCloudRegion(cloudRegion); err != nil {
			return errors.Annotate(err, "validating cloud region")
		}
		newCloud.HostCloud = cloudRegion
		logger.Criticalf("newCloud.Regions -> %+v", newCloud.Regions)
	}

	if err := addCloudToLocal(c.cloudMetadataStore, newCloud); err != nil {
		return errors.Trace(err)
	}

	cloudClient, err := c.cloudAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer cloudClient.Close()

	if err := addCloudToController(cloudClient, newCloud); err != nil {
		return errors.Trace(err)
	}

	if err := c.addCredentialToLocal(c.caasName, credential, context.CredentialName); err != nil {
		return errors.Trace(err)
	}

	if err := c.addCredentialToController(cloudClient, credential, context.CredentialName); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func parseCloudRegion(cloudRegion string) (string, string, error) {
	fields := strings.SplitN(cloudRegion, "/", 2)
	if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
		return "", "", errors.NotValidf("expected <cloud>/<region>")
	}
	return fields[0], fields[1], nil
}

func (c *AddCAASCommand) validateCloudRegion(cloudRegion string) error {
	cloudName, region, err := parseCloudRegion(cloudRegion)
	if err != nil {
		return errors.Annotate(err, "validating cloud region")
	}
	// TODO: ensure cloudName/region is valid juju cloud region(juju regions <cloudName>).
	logger.Criticalf("cloudName -> %q, region -> %q", cloudName, region)
	return nil
}

func (c *AddCAASCommand) getClusterRegion(
	ctx *cmd.Context,
	cloud jujucloud.Cloud,
	credential jujucloud.Credential,
) (string, error) {
	modelConfigClient, err := c.modelConfigAPIFunc()
	if err != nil {
		return "", errors.Trace(err)
	}
	defer modelConfigClient.Close()

	// this current model is not really we want, but it's just for generating the config.
	attrs, err := modelConfigClient.ModelGet()
	if err != nil {
		return "", errors.Trace(err)
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return "", errors.Trace(err)
	}

	cloudSpec, err := environs.MakeCloudSpec(cloud, "", &credential)
	if err != nil {
		return "", errors.Trace(err)
	}

	broker, err := caas.New(environs.OpenParams{Cloud: cloudSpec, Config: cfg})
	if err != nil {
		return "", errors.Trace(err)
	}

	interrupted := make(chan os.Signal, 1)
	defer close(interrupted)
	ctx.InterruptNotify(interrupted)
	defer ctx.StopInterruptNotify(interrupted)

	result := make(chan string, 1)
	errChan := make(chan error, 1)
	go func() {
		cloudRegions, err := broker.ListHostCloudRegions()
		if err != nil {
			errChan <- err
		}
		if cloudRegions == nil || cloudRegions.Size() == 0 {
			errChan <- errors.New("no cloud region information is set in the custer")
		} else {
			// we currently assume it's always a single region cluster.
			result <- cloudRegions.SortedValues()[0]
		}
	}()

	timeout := 30 * time.Second
	defer fmt.Println("")
	for {
		select {
		case <-time.After(2 * time.Second):
			fmt.Printf(".")
		case <-interrupted:
			ctx.Infof("ctrl+c detected, aborting...")
			return "", nil
		case <-time.After(timeout):
			return "", errors.Timeoutf("timeout after %v", timeout)
		case err := <-errChan:
			return "", err
		case cloudRegion := <-result:
			return cloudRegion, nil
		}
	}
}

func (c *AddCAASCommand) verifyName(name string) error {
	public, _, err := c.cloudMetadataStore.PublicCloudMetadata()
	if err != nil {
		return err
	}
	msg, err := nameExists(name, public)
	if err != nil {
		return errors.Trace(err)
	}
	if msg != "" {
		return errors.Errorf(msg)
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

func addCloudToLocal(cloudMetadataStore CloudMetadataStore, newCloud jujucloud.Cloud) error {
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

func addCloudToController(apiClient AddCloudAPI, newCloud jujucloud.Cloud) error {
	err := apiClient.AddCloud(newCloud)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *AddCAASCommand) addCredentialToLocal(cloudName string, newCredential jujucloud.Credential, credentialName string) error {
	newCredentials := &jujucloud.CloudCredential{
		AuthCredentials: make(map[string]jujucloud.Credential),
	}
	newCredentials.AuthCredentials[credentialName] = newCredential
	err := c.fileCredentialStore.UpdateCredential(cloudName, *newCredentials)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *AddCAASCommand) addCredentialToController(apiClient AddCloudAPI, newCredential jujucloud.Credential, credentialName string) error {
	currentAccountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return errors.Trace(err)
	}

	cloudCredTag := names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s",
		c.caasName, currentAccountDetails.User, credentialName))

	if err := apiClient.AddCredential(cloudCredTag.String(), newCredential); err != nil {
		return errors.Trace(err)
	}
	return nil
}
