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

	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/caas/kubernetes/provider"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	jujucmdcloud "github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/feature"
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

// CloudAPI - Implemented by cloudapi.Client.
type CloudAPI interface {
	common.CloudAPI
	common.UploadAPI

	// AddCredential uploads credential to the controller
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
be detected automatically, user has to specify desired host cloud and/or region
via a positional argument. If a region is specified without a cloud qualifier, 
then it is assumed to be in the same cloud as the controller model.

When adding a GKE or AKS cluster, you can use the --gke or --aks option to
interactively be stepped through the registration process, or you can supply the
necessary parameters directly.

Examples:
    juju add-k8s myk8scloud
    juju add-k8s myk8scloud --local
    juju add-k8s myk8scloud --controller mycontroller
    juju add-k8s --context-name mycontext myk8scloud
    juju add-k8s myk8scloud <cloudNameOrCloudType>/<someregion>
    juju add-k8s myk8scloud <cloudNameOrCloudType>
    juju add-k8s myk8scloud  <someregion>

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
	controllerName string
	credentialName string

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

	// hostCloudRegion stores user specified value of a region option.
	// TODO (anastasiamac 2019-07-24) Remove for Juju 3 as redundant, cloud/region is positional as in add-model and bootstrap.
	hostCloudRegion string

	// cloud stores user specified value of a cloud option.
	// TODO (anastasiamac 2019-07-24) Remove for Juju 3 as redundant, cloud/region is positional as in add-model and bootstrap.
	cloud string

	cloudRegion string

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

	cloudAPIFunc func() (CloudAPI, error)
}

// NewAddCAASCommand returns a command to add caas information.
func NewAddCAASCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	store := jujuclient.NewFileClientStore()
	command := &AddCAASCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store:       store,
			EnabledFlag: feature.MultiCloud,
		},
		cloudMetadataStore: cloudMetadataStore,
		newClientConfigReader: func(caasType string) (clientconfig.ClientConfigFunc, error) {
			return clientconfig.NewClientConfigReader(caasType)
		},
	}
	command.cloudAPIFunc = func() (CloudAPI, error) {
		root, err := command.NewAPIRoot(command.Store, command.controllerName, "")
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cloudapi.NewClient(root), nil
	}

	command.brokerGetter = command.newK8sClusterBroker
	command.getAllCloudDetails = jujucmdcloud.GetAllCloudDetails
	return modelcmd.WrapBase(command)
}

// Info returns help information about the command.
func (c *AddCAASCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "add-k8s",
		Args:    "<k8s name> [cloud|region|(cloud/region)]",
		Purpose: usageAddCAASSummary,
		Doc:     usageAddCAASDetails,
	})
}

// SetFlags initializes the flags supported by the command.
func (c *AddCAASCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
	f.StringVar(&c.clusterName, "cluster-name", "", "Specify the k8s cluster to import")
	f.StringVar(&c.contextName, "context-name", "", "Specify the k8s context to import")
	f.StringVar(&c.hostCloudRegion, "region", "", "DEPRECATED kubernetes cluster cloud and/or region")
	f.StringVar(&c.cloud, "cloud", "", "Kubernetes cluster cloud and/or region")
	f.StringVar(&c.workloadStorage, "storage", "", "Kubernetes storage class for workload storage")
	f.StringVar(&c.project, "project", "", "Project to which the cluster belongs")
	f.StringVar(&c.credential, "credential", "", "The credential to use when accessing the cluster")
	f.StringVar(&c.resourceGroup, "resource-group", "", "The Azure resource group of the AKS cluster")
	f.BoolVar(&c.gke, "gke", false, "Type of kubernetes cluster to add, here GKE")
	f.BoolVar(&c.aks, "aks", false, "Type of kubernetes cluster to add, here AKS")
}

// Init populates the command with the args from the command line.
func (c *AddCAASCommand) Init(args []string) (err error) {
	argsCount := len(args)
	if argsCount == 0 {
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

	end := 1
	if argsCount > 1 {
		c.cloudRegion = args[1]
		end = 2
		goto next
	}

	// TODO (anastsiamac 2019-07-24) This is unnecessary when --region and --cloud is removed.
	if c.hostCloudRegion != "" {
		if c.cloud != "" {
			if strings.Contains(c.cloud, "/") || strings.Contains(c.hostCloudRegion, "/") {
				// <cloud>/<region> definitions may conflict.
				return errors.New("provide either --region or --cloud, not both")
			}
			c.cloudRegion = jujucloud.BuildHostCloudRegion(c.cloud, c.hostCloudRegion)
			goto next
		}
		c.cloudRegion = c.hostCloudRegion
	} else {
		c.cloudRegion = c.cloud
	}

next:
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
	return cmd.CheckEmpty(args[end:])
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
	c.cloudRegion = jujucloud.BuildHostCloudRegion(c.k8sCluster.cloud(), p.region)
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
	c.cloudRegion = jujucloud.BuildHostCloudRegion(c.k8sCluster.cloud(), p.region)
	return c.k8sCluster.getKubeConfig(p)
}

var clusterQueryErrMsg = `
	Juju needs to query the k8s cluster to ensure that the recommended
	storage defaults are available and to detect the cluster's cloud/region.
	This was not possible in this case so run add-k8s again, using
	--storage=<name> to specify the storage class to use and
	'juju add-k8s <k8s name> [cloud|region|(cloud/region)]' to specify the cloud/region.
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
func (c *AddCAASCommand) Run(ctx *cmd.Context) (err error) {
	if c.hostCloudRegion != "" {
		ctx.Infof("region flag is DEPRECATED. Use 'add-k8s name cloud/region' instead")
	}
	if c.cloud != "" {
		ctx.Infof("cloud flag is DEPRECATED. Use 'add-k8s name cloud/region' instead")
	}
	if err := c.verifyName(c.caasName); err != nil {
		return errors.Trace(err)
	}

	// validate provided cloud/region
	cloudClient, err := c.cloudAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer cloudClient.Close()

	cloudRegion := ""
	if c.cloudRegion != "" {
		cloudRegionParams := common.CloudRegionValidationParams{
			LocalStore:        c.Store,
			RemoteCloudClient: cloudClient,
			LocalOnly:         c.Local,
			CloudRegion:       c.cloudRegion,
			Command:           c.Info().Name,
			UseDefaultRegion:  true,
		}
		var cloud jujucloud.Cloud
		var err error
		_, cloud, cloudRegion, err = common.ParseCloudRegionMaybeDefaultRegion(cloudRegionParams)
		if err != nil {
			return errors.Trace(err)
		}
		// TODO (anastasiamac 2019-04-24) For some reason everywhere after this point,
		// this changes from <cloud name>/<region> to <cloud type>/<region>..
		// Is this correct?
		c.cloudRegion = jujucloud.BuildHostCloudRegion(cloud.Type, cloudRegion)
		c.cloud = cloud.Name
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
		HostCloudRegion:    c.cloudRegion,
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
		HostCloudRegion:        c.cloudRegion,
		MetadataChecker:        broker,
		GetClusterMetadataFunc: c.getClusterMetadataFunc(ctx),
	}

	storageMsg, err := provider.UpdateKubeCloudWithStorage(&newCloud, storageParams)
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

	// By this stage, new cloud's host cloud region should be deduced.
	// However, if it's not, use the command's version.
	if newCloud.HostCloudRegion == "" {
		newCloud.HostCloudRegion = c.cloudRegion
	}

	if err := common.AddLocalCloud(c.cloudMetadataStore, newCloud); err != nil {
		return errors.Trace(err)
	}

	newCredentials := &jujucloud.CloudCredential{
		AuthCredentials: make(map[string]jujucloud.Credential),
	}
	newCredentials.AuthCredentials[credentialName] = credential
	if err := common.AddLocalCredentials(c.Store, c.caasName, *newCredentials); err != nil {
		return errors.Trace(err)
	}

	if clusterName == "" {
		clusterName = newCloud.HostCloudRegion
	}
	if c.controllerName == "" {
		successMsg := fmt.Sprintf("k8s substrate %q added as cloud %q%s", clusterName, c.caasName, storageMsg)
		successMsg += fmt.Sprintf("\nYou can now bootstrap to this cloud by running 'juju bootstrap %s'.", c.caasName)
		fmt.Fprintln(ctx.Stdout, successMsg)
		return nil
	}

	if err := common.AddRemoteCloud(cloudClient, newCloud); err != nil {
		return errors.Trace(err)
	}
	if err := c.addCredentialToController(ctx, cloudClient, newCloud, cloudRegion, *newCredentials); err != nil {
		return errors.Trace(err)
	}
	successMsg := fmt.Sprintf("k8s substrate %q added as cloud %q%s", clusterName, c.caasName, storageMsg)
	fmt.Fprintln(ctx.Stdout, successMsg)

	return nil
}

func (c *AddCAASCommand) newK8sClusterBroker(cloud jujucloud.Cloud, credential jujucloud.Credential) (caas.ClusterMetadataChecker, error) {
	openParams, err := provider.BaseKubeCloudOpenParams(cloud, credential)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if c.controllerName != "" {
		ctrlUUID, err := c.ControllerUUID(c.Store, c.controllerName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		openParams.ControllerUUID = ctrlUUID
	}
	return caas.New(openParams)
}

func (c *AddCAASCommand) getClusterMetadataFunc(ctx *cmd.Context) provider.GetClusterMetadataFunc {
	return func(storageParams provider.KubeCloudStorageParams) (*caas.ClusterMetadata, error) {
		interrupted := make(chan os.Signal, 1)
		defer close(interrupted)
		ctx.InterruptNotify(interrupted)
		defer ctx.StopInterruptNotify(interrupted)

		result := make(chan *caas.ClusterMetadata, 1)
		errChan := make(chan error, 1)
		go func() {
			clusterMetadata, err := storageParams.MetadataChecker.GetClusterMetadata(storageParams.WorkloadStorage)
			if err != nil {
				errChan <- err
			} else {
				result <- clusterMetadata
			}
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
	// TODO (anastasiamac 2019-0424) Is this correct?
	// only public clouds can be used here?
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

func (c *AddCAASCommand) addCredentialToController(ctx *cmd.Context, apiClient CloudAPI, newCloud jujucloud.Cloud, region string, cloudCredential jujucloud.CloudCredential) error {
	currentAccountDetails, err := c.Store.AccountDetails(c.controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	newlyVerified, err := common.VerifyCredentialsForUpload(ctx, currentAccountDetails, &newCloud, region, cloudCredential.AuthCredentials)
	if err != nil {
		return errors.Trace(err)
	}
	// There should be only one.
	if len(newlyVerified) != 1 {
		return errors.Errorf("could not verify credential")
	}

	for k, v := range newlyVerified {
		if err := apiClient.AddCredential(k, v); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
