// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/juju/names.v2"

	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/caas/kubernetes/provider"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	jujucmdcloud "github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
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

// AddCloudAPI - Implemented by cloudapi.Client.
type AddCloudAPI interface {
	AddCloud(jujucloud.Cloud) error
	AddCredential(tag string, credential jujucloud.Credential) error
	Close() error
}

// BrokerGetter returns caas broker instance.
type BrokerGetter func(cloud jujucloud.Cloud, credential jujucloud.Credential) (caas.ClusterMetadataChecker, error)

var usageAddCAASSummary = `
Adds a k8s endpoint and credential to Juju.`[1:]

var usageAddCAASDetails = `
Creates a user-defined cloud based on a k8s cluster.

The new k8s cloud can then be used to bootstrap into, or it
can be added to an existing controller; the current controller
is used unless the --controller option is specified. If you just
want to update the local cache and not a running controller, use
the --local option.

Specify a non default kubeconfig file location using $KUBECONFIG
environment variable or pipe in file content from stdin.

The config file can contain definitions for different k8s clusters,
use --cluster-name to pick which one to use.
It's also possible to select a context by name using --context-name.

When running add-k8s the underlying cloud/region hosting the cluster needs to be
detected to enable storage to be correctly configured. If the cloud/region cannot
be detected automatically, use --region <cloudType/region> to specify the host
cloud type and region.

When adding a GKE or AKS cluster, you can use the --gke or --aks option to
interactively be stepped through the registration process, or you can supply the
necessary parameters directly.

Examples:
    juju add-k8s myk8scloud
    juju add-k8s myk8scloud --local
    juju add-k8s myk8scloud --controller mycontroller
    juju add-k8s --context-name mycontext myk8scloud
    juju add-k8s myk8scloud --region <cloudType/region>

    KUBECONFIG=path-to-kubuconfig-file juju add-k8s myk8scloud --cluster-name=my_cluster_name
    kubectl config view --raw | juju add-k8s myk8scloud --cluster-name=my_cluster_name

    juju add-k8s --gke myk8scloud
    juju add-k8s --gke --project=myproject myk8scloud
    juju add-k8s --gke --credential=myaccount --project=myproject myk8scloud
    juju add-k8s --gke --credential=myaccount --project=myproject --region=someregion myk8scloud

    juju add-k8s --aks myk8scloud
    juju add-k8s --aks --cluster-name mycluster myk8scloud
    juju add-k8s --aks --cluster-name mycluster --resource-group myrg myk8scloud

See also:
    remove-k8s
`

// AddCAASCommand is the command that allows you to add a caas and credential
type AddCAASCommand struct {
	modelcmd.OptionalControllerCommand

	// These attributes are used when adding a cluster to a controller.
	controllerName  string
	credentialName  string
	store           jujuclient.ClientStore
	addCloudAPIFunc func() (AddCloudAPI, error)

	// caasName is the name of the caas to add.
	caasName string

	// caasType is the type of CAAS being added.
	caasType string

	// clusterName is the name of the cluster (k8s) or credential to import.
	clusterName string

	// contextName is the name of the contex to import.
	contextName string

	// project is the project id for the cluster.
	project string

	// credential is the credential to use when accessing the cluster.
	credential string

	// resourceGroup is the resource group name for the cluster.
	resourceGroup string

	// hostCloudRegion is the cloud region that the nodes of cluster (k8s) are running in.
	// The format is <cloudType/region>.
	hostCloudRegion string

	// workloadStorage is a storage class specified by the user.
	workloadStorage string

	// brokerGetter returns caas broker instance.
	brokerGetter BrokerGetter

	gke        bool
	aks        bool
	k8sCluster k8sCluster

	cloudMetadataStore    CloudMetadataStore
	newClientConfigReader func(string) (clientconfig.ClientConfigFunc, error)

	getAllCloudDetails func() (map[string]*jujucmdcloud.CloudDetails, error)
}

// NewAddCAASCommand returns a command to add caas information.
func NewAddCAASCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	store := jujuclient.NewFileClientStore()
	cmd := &AddCAASCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		cloudMetadataStore:        cloudMetadataStore,
		store:                     store,
		newClientConfigReader: func(caasType string) (clientconfig.ClientConfigFunc, error) {
			return clientconfig.NewClientConfigReader(caasType)
		},
	}
	cmd.addCloudAPIFunc = func() (AddCloudAPI, error) {
		root, err := cmd.NewAPIRoot(cmd.store, cmd.controllerName, "")
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cloudapi.NewClient(root), nil
	}

	cmd.brokerGetter = cmd.newK8sClusterBroker
	cmd.getAllCloudDetails = jujucmdcloud.GetAllCloudDetails
	return modelcmd.WrapBase(cmd)
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
	c.OptionalControllerCommand.SetFlags(f)
	f.StringVar(&c.clusterName, "cluster-name", "", "Specify the k8s cluster to import")
	f.StringVar(&c.contextName, "context-name", "", "Specify the k8s context to import")
	f.StringVar(&c.hostCloudRegion, "region", "", "kubernetes cluster cloud and/or region")
	f.StringVar(&c.workloadStorage, "storage", "", "kubernetes storage class for workload storage")
	f.StringVar(&c.project, "project", "", "project to which the cluster belongs")
	f.StringVar(&c.credential, "credential", "", "the credential to use when accessing the cluster")
	f.StringVar(&c.resourceGroup, "resource-group", "", "the Azure resource group of the AKS cluster")
	f.BoolVar(&c.gke, "gke", false, "used when adding a GKE cluster")
	f.BoolVar(&c.aks, "aks", false, "used when adding an AKS cluster")
}

// Init populates the command with the args from the command line.
func (c *AddCAASCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.Errorf("missing k8s name.")
	}
	if c.gke && c.aks {
		return errors.BadRequestf("only one of '--gke' or '--aks' can be supplied")
	}
	c.caasType = "kubernetes"
	c.caasName = args[0]

	if c.contextName != "" && c.clusterName != "" {
		return errors.New("only specify one of cluster-name or context-name, not both")
	}
	if c.gke {
		if c.contextName != "" {
			return errors.New("do not specify context name when adding a GKE cluster")
		}
		if c.k8sCluster == nil {
			c.k8sCluster = newGKECluster()
		}
		if err := c.k8sCluster.ensureExecutable(); err != nil {
			return errors.Trace(err)
		}
	} else {
		if c.project != "" {
			return errors.New("do not specify project unless adding a GKE cluster")
		}
		if c.credential != "" {
			return errors.New("do not specify credential unless adding a GKE cluster")
		}
		if c.aks {
			if c.contextName != "" {
				return errors.New("do not specify context name when adding a AKS cluster")
			}
			if c.k8sCluster == nil {
				c.k8sCluster = newAKSCluster()
			}
			if err := c.k8sCluster.ensureExecutable(); err != nil {
				return errors.Trace(err)
			}
		}
	}

	c.controllerName, err = c.ControllerNameFromArg()
	if err != nil && errors.Cause(err) != modelcmd.ErrNoControllersDefined {
		return errors.Trace(err)
	}
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

func (c *AddCAASCommand) getConfigReader(ctx *cmd.Context) (io.Reader, string, error) {
	if c.gke {
		return c.getGKEKubeConfig(ctx)
	}
	if c.aks {
		return c.getAKSKubeConfig(ctx)
	}
	rdr, err := getStdinPipe(ctx)
	return rdr, c.clusterName, err
}

func (c *AddCAASCommand) getGKEKubeConfig(ctx *cmd.Context) (io.Reader, string, error) {
	p := &clusterParams{
		name:       c.clusterName,
		region:     c.hostCloudRegion,
		project:    c.project,
		credential: c.credential,
	}

	// If any items are missing, prompt for them.
	if p.name == "" || p.project == "" || p.region == "" {
		var err error
		p, err = c.k8sCluster.interactiveParams(ctx, p)
		if err != nil {
			return nil, "", errors.Trace(err)
		}
	}
	c.clusterName = p.name
	c.hostCloudRegion = c.k8sCluster.cloud() + "/" + p.region
	return c.k8sCluster.getKubeConfig(p)
}

func (c *AddCAASCommand) getAKSKubeConfig(ctx *cmd.Context) (io.Reader, string, error) {
	p := &clusterParams{
		name:          c.clusterName,
		resourceGroup: c.resourceGroup,
	}

	// If any items are missing, prompt for them. Don't pass in region as we'll query the resource group for it.
	// maybe we just always want to call interactive params so it takes care of the region/location issue.
	if p.name == "" || p.resourceGroup == "" || p.region == "" {
		var err error
		p, err = c.k8sCluster.interactiveParams(ctx, p)
		if err != nil {
			return nil, "", errors.Trace(err)
		}
	}
	c.clusterName = p.name
	c.hostCloudRegion = c.k8sCluster.cloud() + "/" + p.region
	return c.k8sCluster.getKubeConfig(p)
}

var clusterQueryErrMsg = `
	Juju needs to query the k8s cluster to ensure that the recommended
	storage defaults are available and to detect the cluster's cloud/region.
	This was not possible in this case so run add-k8s again, using
	--storage=<name> to specify the storage class to use and
	--region=<cloud>/<region> to specify the cloud/region.
`[1:]

var unknownClusterErrMsg = `
	Juju needs to query the k8s cluster to ensure that the recommended
	storage defaults are available and to detect the cluster's cloud/region.
	This was not possible in this case because the cloud %q is not known to Juju.
	Run add-k8s again, using --storage=<name> to specify the storage class to use.
`[1:]

var noRecommendedStorageErrMsg = `
	No recommended storage configuration is defined on this cluster.
	Run add-k8s again with --storage=<name> and Juju will use the
	specified storage class or create a storage-class using the recommended
	%q provisioner.
`[1:]

// Run is defined on the Command interface.
func (c *AddCAASCommand) Run(ctx *cmd.Context) error {
	if err := c.verifyName(c.caasName); err != nil {
		return errors.Trace(err)
	}
	rdr, clusterName, err := c.getConfigReader(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if closer, ok := rdr.(io.Closer); ok {
		defer closer.Close()
	}

	config := provider.KubeCloudParams{
		ClusterName:        clusterName,
		CaasName:           c.caasName,
		ContextName:        c.contextName,
		HostCloudRegion:    c.hostCloudRegion,
		CaasType:           c.caasType,
		ClientConfigGetter: c.newClientConfigReader,
	}

	newCloud, credential, credentialName, err := provider.CloudFromKubeConfig(rdr, config)
	if err != nil {
		return errors.Trace(err)
	}

	broker, err := c.brokerGetter(newCloud, credential)
	if err != nil {
		return errors.Trace(err)
	}
	storageParams := provider.KubeCloudStorageParams{
		WorkloadStorage:        c.workloadStorage,
		HostCloudRegion:        c.hostCloudRegion,
		MetadataChecker:        broker,
		GetClusterMetadataFunc: c.getClusterMetadataFunc(ctx),
	}

	storageMsg, err := provider.UpdateKubeCloudWithStorage(&newCloud, credential, storageParams)
	if err != nil {
		if provider.IsClusterQueryError(err) {
			if err.Error() == "" {
				return errors.New(clusterQueryErrMsg)
			}
			return errors.Annotate(err, clusterQueryErrMsg)
		}
		if provider.IsNoRecommendedStorageError(err) {
			return errors.Errorf(noRecommendedStorageErrMsg, err.(provider.NoRecommendedStorageError).StorageProvider())
		}
		if provider.IsUnknownClusterError(err) {
			return errors.Errorf(unknownClusterErrMsg, err.(provider.UnknownClusterError).CloudName)
		}
		return errors.Trace(err)
	}

	if err := addCloudToLocal(c.cloudMetadataStore, newCloud); err != nil {
		return errors.Trace(err)
	}

	if err := c.addCredentialToLocal(c.caasName, credential, credentialName); err != nil {
		return errors.Trace(err)
	}

	if clusterName == "" {
		clusterName = c.hostCloudRegion
	}
	if c.controllerName == "" {
		successMsg := fmt.Sprintf("k8s substrate %q added as cloud %q%s", clusterName, c.caasName, storageMsg)
		successMsg += fmt.Sprintf("\nYou can now bootstrap to this cloud by running 'juju bootstrap %s'.", c.caasName)
		fmt.Fprintln(ctx.Stdout, successMsg)
		return nil
	}

	cloudClient, err := c.addCloudAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer cloudClient.Close()

	if err := c.addCloudToControllerWithRegion(cloudClient, newCloud); err != nil {
		return errors.Trace(err)
	}
	if err := c.addCredentialToController(cloudClient, credential, credentialName); err != nil {
		return errors.Trace(err)
	}
	successMsg := fmt.Sprintf("k8s substrate %q added as cloud %q%s", clusterName, c.caasName, storageMsg)
	fmt.Fprintln(ctx.Stdout, successMsg)

	return nil
}

func (c *AddCAASCommand) addCloudToControllerWithRegion(apiClient AddCloudAPI, newCloud jujucloud.Cloud) (err error) {
	if newCloud.HostCloudRegion != "" {
		hostCloudRegion, err := c.validateCloudRegion(newCloud.HostCloudRegion)
		if err != nil {
			return errors.Trace(err)
		}
		newCloud.HostCloudRegion = hostCloudRegion
	}
	if err := addCloudToController(apiClient, newCloud); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *AddCAASCommand) newK8sClusterBroker(cloud jujucloud.Cloud, credential jujucloud.Credential) (caas.ClusterMetadataChecker, error) {
	openParams, err := provider.BaseKubeCloudOpenParams(cloud, credential)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if c.controllerName != "" {
		ctrlUUID, err := c.ControllerUUID(c.store, c.controllerName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		openParams.ControllerUUID = ctrlUUID
	}
	return caas.New(openParams)
}

func (c *AddCAASCommand) validateCloudRegion(cloudRegion string) (_ string, err error) {
	defer errors.DeferredAnnotatef(&err, "validating cloud region %q", cloudRegion)

	cloudNameOrType, region, err := provider.ParseCloudRegion(cloudRegion)
	if err != nil {
		return "", errors.Annotate(err, "parsing cloud region")
	}
	// microk8s is special.
	if cloudNameOrType == caas.K8sCloudMicrok8s && region == caas.Microk8sRegion {
		return cloudRegion, nil
	}

	clouds, err := c.getAllCloudDetails()
	if err != nil {
		return "", errors.Annotate(err, "listing cloud regions")
	}
	for name, details := range clouds {
		// User may have specified cloud name or type so match on both.
		if name == cloudNameOrType || details.CloudType == cloudNameOrType {
			if region == "" && len(details.RegionsMap) == 0 {
				return details.CloudType, nil
			}
			for k := range details.RegionsMap {
				if k == region {
					logger.Debugf("cloud region %q is valid", cloudRegion)
					return details.CloudType + "/" + region, nil
				}
			}
		}
	}
	return "", errors.NotValidf("cloud region %q", cloudRegion)
}

func (c *AddCAASCommand) getClusterMetadataFunc(ctx *cmd.Context) provider.GetClusterMetadataFunc {
	return func(broker caas.ClusterMetadataChecker) (*caas.ClusterMetadata, error) {
		interrupted := make(chan os.Signal, 1)
		defer close(interrupted)
		ctx.InterruptNotify(interrupted)
		defer ctx.StopInterruptNotify(interrupted)

		result := make(chan *caas.ClusterMetadata, 1)
		errChan := make(chan error, 1)
		go func() {
			clusterMetadata, err := broker.GetClusterMetadata(c.workloadStorage)
			if err != nil {
				errChan <- err
			}
			result <- clusterMetadata
		}()

		timeout := 30 * time.Second
		defer fmt.Fprintln(ctx.Stdout, "")
		for {
			select {
			case <-time.After(1 * time.Second):
				fmt.Fprintf(ctx.Stdout, ".")
			case <-interrupted:
				ctx.Infof("ctrl+c detected, aborting...")
				return nil, nil
			case <-time.After(timeout):
				return nil, errors.Timeoutf("timeout after %v", timeout)
			case err := <-errChan:
				return nil, err
			case clusterMetadata := <-result:
				return clusterMetadata, nil
			}
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
	err := c.store.UpdateCredential(cloudName, *newCredentials)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *AddCAASCommand) addCredentialToController(apiClient AddCloudAPI, newCredential jujucloud.Credential, credentialName string) error {
	_, err := c.store.ControllerByName(c.controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	currentAccountDetails, err := c.store.AccountDetails(c.controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	id := fmt.Sprintf("%s/%s/%s", c.caasName, currentAccountDetails.User, credentialName)
	if !names.IsValidCloudCredential(id) {
		return errors.NotValidf("cloud credential ID %q", id)
	}
	cloudCredTag := names.NewCloudCredentialTag(id)

	if err := apiClient.AddCredential(cloudCredTag.String(), newCredential); err != nil {
		return errors.Trace(err)
	}
	return nil
}
