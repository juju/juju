// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/juju/names.v2"

	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var logger = loggo.GetLogger("juju.cmd.juju.k8s")

type CloudMetadataStore interface {
	ParseCloudMetadataFile(path string) (map[string]cloud.Cloud, error)
	ParseOneCloud(data []byte) (cloud.Cloud, error)
	PublicCloudMetadata(searchPaths ...string) (result map[string]cloud.Cloud, fallbackUsed bool, _ error)
	PersonalCloudMetadata() (map[string]cloud.Cloud, error)
	WritePersonalCloudMetadata(cloudsMap map[string]cloud.Cloud) error
}

// AddCloudAPI - Implemented by cloudapi.Client
type AddCloudAPI interface {
	AddCloud(cloud.Cloud) error
	AddCredential(tag string, credential cloud.Credential) error
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

	cloudMetadataStore    CloudMetadataStore
	fileCredentialStore   jujuclient.CredentialStore
	apiFunc               func() (AddCloudAPI, error)
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
	cmd.apiFunc = func() (AddCloudAPI, error) {
		root, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cloudapi.NewClient(root), nil
	}
	return modelcmd.WrapController(cmd)
}

// Info returns help information about the command.
func (c *AddCAASCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "add-k8s",
		Args:    "<k8s name>",
		Purpose: usageAddCAASSummary,
		Doc:     usageAddCAASDetails,
	})
}

// SetFlags initializes the flags supported by the command.
func (c *AddCAASCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.clusterName, "cluster-name", "", "Specify the k8s cluster to import")
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
func getStdinPipe(ctxt *cmd.Context) (io.Reader, error) {
	if stdIn, ok := ctxt.Stdin.(*os.File); ok && !terminal.IsTerminal(int(stdIn.Fd())) {
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
func (c *AddCAASCommand) Run(ctxt *cmd.Context) error {
	if err := c.verifyName(c.caasName); err != nil {
		return errors.Trace(err)
	}

	clientConfigFunc, err := c.newClientConfigReader(c.caasType)
	if err != nil {
		return errors.Trace(err)
	}
	stdIn, err := getStdinPipe(ctxt)
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

	newCloud := cloud.Cloud{
		Name:           c.caasName,
		Type:           c.caasType,
		Endpoint:       currentCloud.Endpoint,
		AuthTypes:      []cloud.AuthType{credential.AuthType()},
		CACertificates: []string{cloudCAData},
	}

	if err := addCloudToLocal(c.cloudMetadataStore, newCloud); err != nil {
		return errors.Trace(err)
	}

	cloudClient, err := c.apiFunc()
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

func addCloudToLocal(cloudMetadataStore CloudMetadataStore, newCloud cloud.Cloud) error {
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

func addCloudToController(apiClient AddCloudAPI, newCloud cloud.Cloud) error {
	err := apiClient.AddCloud(newCloud)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *AddCAASCommand) addCredentialToLocal(cloudName string, newCredential cloud.Credential, credentialName string) error {
	newCredentials := &cloud.CloudCredential{
		AuthCredentials: make(map[string]cloud.Credential),
	}
	newCredentials.AuthCredentials[credentialName] = newCredential
	err := c.fileCredentialStore.UpdateCredential(cloudName, *newCredentials)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *AddCAASCommand) addCredentialToController(apiClient AddCloudAPI, newCredential cloud.Credential, credentialName string) error {
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
