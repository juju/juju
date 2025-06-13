// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"bytes"
	stdcontext "context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"golang.org/x/crypto/ssh/terminal"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	cloudapi "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
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
	ReadCloudData(path string) ([]byte, error)
	ParseOneCloud(data []byte) (jujucloud.Cloud, error)
	PublicCloudMetadata(searchPaths ...string) (result map[string]jujucloud.Cloud, fallbackUsed bool, _ error)
	PersonalCloudMetadata() (map[string]jujucloud.Cloud, error)
	WritePersonalCloudMetadata(cloudsMap map[string]jujucloud.Cloud) error
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/storeapi_mock.go github.com/juju/juju/cmd/juju/caas CredentialStoreAPI
type CredentialStoreAPI interface {
	UpdateCredential(cloudName string, details jujucloud.CloudCredential) error
}

// AddCloudAPI - Implemented by cloudapi.Client.
type AddCloudAPI interface {
	AddCloud(jujucloud.Cloud, bool) error
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
can be added to an existing controller.

Use --controller option to add k8s cloud to a controller.
Use --client option to add k8s cloud to this client.

Specify a non default kubeconfig file location using $KUBECONFIG
environment variable or pipe in file content from stdin.

The config file can contain definitions for different k8s clusters,
use --cluster-name to pick which one to use.
It's also possible to select a context by name using --context-name.

When running add-k8s the underlying cloud/region hosting the cluster needs to be
detected to enable storage to be correctly configured. If the cloud/region cannot
be detected automatically, use either
  --cloud <cloudType|cloudName> to specify the host cloud
or
  --region <cloudType|cloudName>/<someregion> to specify the host
  cloud type and region.

Region is strictly necessary only when adding a k8s cluster to a JAAS controller.
When using a standalone Juju controller, usually just --cloud is required.

Once Juju is aware of the underlying cloud type, it looks for a suitably configured
storage class to provide operator and workload storage. If none is found, use
of the --storage option is required so that Juju will create a storage class
with the specified name.

If the cluster does not have a storage provisioning capability, use the
--skip-storage option to add the cluster without any workload storage configured.

When adding a GKE or AKS cluster, you can use the --gke or --aks option to
interactively be stepped through the registration process, or you can supply the
necessary parameters directly.

Examples:
    juju add-k8s myk8scloud
    juju add-k8s myk8scloud --client
    juju add-k8s myk8scloud --controller mycontroller
    juju add-k8s --context-name mycontext myk8scloud
    juju add-k8s myk8scloud --region cloudNameOrCloudType/someregion
    juju add-k8s myk8scloud --cloud cloudNameOrCloudType
    juju add-k8s myk8scloud --cloud cloudNameOrCloudType --region=someregion
    juju add-k8s myk8scloud --cloud cloudNameOrCloudType --storage mystorageclass

    KUBECONFIG=path-to-kubeconfig-file juju add-k8s myk8scloud --cluster-name=my_cluster_name
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

	clock jujuclock.Clock

	// These attributes are used when adding a cluster to a controller.
	addCloudAPIFunc func() (AddCloudAPI, error)

	// caasName is the name of the CAAS to add.
	caasName string

	// caasType is the type of CAAS being added.
	caasType string

	// clusterName is the name of the cluster (k8s) or credential to import.
	clusterName string

	// contextName is the name of the context to import.
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
	// hostCloud is an alias of the hostCloudRegion.
	hostCloud string

	// givenHostCloudRegion holds a copy of cloud name/type and/or region as supplied by the user via options.
	givenHostCloudRegion string

	// workloadStorage is a storage class specified by the user.
	workloadStorage string

	// skipStorage is used to signal that we don't need to configure storage provisioning.
	skipStorage bool

	// brokerGetter returns CAAS broker instance.
	brokerGetter BrokerGetter

	gke        bool
	aks        bool
	eks        bool
	k8sCluster k8sCluster

	adminServiceAccountResolver func(jujuclock.Clock) clientconfig.K8sCredentialResolver
	cloudMetadataStore          CloudMetadataStore
	credentialStoreAPI          CredentialStoreAPI
	newClientConfigReader       func(string) (clientconfig.ClientConfigFunc, error)
	credentialUIDGetter         func(credentialGetter, string, string) (string, error)

	getAllCloudDetails func(jujuclient.CredentialGetter) (map[string]*jujucmdcloud.CloudDetails, error)
}

// NewAddCAASCommand returns a command to add CAAS information.
func NewAddCAASCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	return newAddCAASCommand(cloudMetadataStore, jujuclock.WallClock)
}

func newAddCAASCommand(cloudMetadataStore CloudMetadataStore, clock jujuclock.Clock) cmd.Command {
	store := jujuclient.NewFileClientStore()
	command := &AddCAASCommand{
		clock: clock,
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store: store,
		},
		cloudMetadataStore:          cloudMetadataStore,
		credentialStoreAPI:          store,
		adminServiceAccountResolver: clientconfig.GetJujuAdminServiceAccountResolver,
		newClientConfigReader: func(caasType string) (clientconfig.ClientConfigFunc, error) {
			return clientconfig.NewClientConfigReader(caasType)
		},
		credentialUIDGetter: decideCredentialUID,
	}
	command.addCloudAPIFunc = func() (AddCloudAPI, error) {
		root, err := command.NewAPIRoot(command.Store, command.ControllerName, "")
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
	f.StringVar(&c.hostCloudRegion, "region", "", "k8s cluster region or cloud/region")
	f.StringVar(&c.hostCloud, "cloud", "", "k8s cluster cloud")
	f.StringVar(&c.workloadStorage, "storage", "", "k8s storage class for workload storage")
	f.BoolVar(&c.skipStorage, "skip-storage", false, "used when adding a cluster that doesn't have storage")
	f.StringVar(&c.project, "project", "", "project to which the cluster belongs")
	f.StringVar(&c.credential, "credential", "", "the credential to use when accessing the cluster")
	f.StringVar(&c.resourceGroup, "resource-group", "", "the Azure resource group of the AKS cluster")
	f.BoolVar(&c.gke, "gke", false, "used when adding a GKE cluster")
	f.BoolVar(&c.aks, "aks", false, "used when adding an AKS cluster")
	f.BoolVar(&c.eks, "eks", false, "used when adding an EKS cluster")
}

func countTrue(items ...bool) (count int) {
	for _, item := range items {
		if item {
			count++
		}
	}
	return count
}

// Init populates the command with the args from the command line.
func (c *AddCAASCommand) Init(args []string) (err error) {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.Errorf("missing k8s name.")
	}

	switch count := countTrue(c.aks, c.gke, c.eks); count {
	case 1:
		if c.contextName != "" {
			return errors.New("do not specify context name when adding a AKS/GKE/EKS cluster")
		}
	default:
		if count > 1 {
			return errors.BadRequestf("only one of '--gke', '--eks' or '--aks' can be supplied")
		}
	}
	c.caasType = "kubernetes"
	c.caasName = args[0]

	if c.contextName != "" && c.clusterName != "" {
		return errors.New("only specify one of cluster-name or context-name, not both")
	}
	if c.hostCloudRegion != "" || c.hostCloud != "" {
		if c.gke || c.aks || c.eks {
			if c.hostCloud != "" {
				return errors.Errorf("do not specify --cloud when adding a GKE, EKS or AKS cluster")
			}
			if strings.Contains(c.hostCloudRegion, "/") {
				return errors.Errorf("only specify region, not cloud/region, when adding a GKE, EKS or AKS cluster")
			}
		} else {
			c.hostCloudRegion, err = c.tryEnsureCloudTypeForHostRegion(c.hostCloud, c.hostCloudRegion)
			if err != nil {
				return errors.Trace(err)
			}
		}
		// Keep a copy of the original user supplied value for comparison and validation later.
		c.givenHostCloudRegion = c.hostCloudRegion
	}

	// TODO(caas): consider to change --gke|--aks|--eks flag to sub commands in future version then we can move these
	// cloud specific options/flags' validation to sub commands level.
	if c.gke {
		if c.k8sCluster == nil {
			c.k8sCluster = newGKECluster()
		}
	} else {
		if c.project != "" {
			return errors.New("do not specify project unless adding a GKE cluster")
		}
		if c.credential != "" {
			return errors.New("do not specify credential unless adding a GKE cluster")
		}
	}

	if c.aks {
		if c.k8sCluster == nil {
			c.k8sCluster = newAKSCluster()
		}
	} else {
		if c.resourceGroup != "" {
			return errors.New("do not specify resource-group unless adding a AKS cluster")
		}
	}

	if c.eks && c.k8sCluster == nil {
		c.k8sCluster = newEKSCluster()
	}

	if c.k8sCluster != nil {
		if err := c.k8sCluster.ensureExecutable(); err != nil {
			return errors.Trace(err)
		}
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
	if c.eks {
		return c.getEKSKubeConfig(ctx)
	}
	rdr, err := getStdinPipe(ctx)
	return rdr, c.clusterName, err
}

func (c *AddCAASCommand) getGKEKubeConfig(ctx *cmd.Context) (io.Reader, string, error) {
	p := &clusterParams{
		openFile:   c.Filesystem().Open,
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

func (c *AddCAASCommand) getEKSKubeConfig(ctx *cmd.Context) (io.Reader, string, error) {
	p := &clusterParams{
		openFile: c.Filesystem().Open,
		name:     c.clusterName,
	}
	var err error
	if len(c.hostCloudRegion) > 0 {
		if _, p.region, err = jujucloud.SplitHostCloudRegion(c.hostCloudRegion); err != nil {
			return nil, "", errors.Annotatef(err, "getting region from host cloud region")
		}
	}

	// If any items are missing, prompt for them.
	if p.name == "" || p.region == "" {
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
		openFile:      c.Filesystem().Open,
		name:          c.clusterName,
		region:        c.hostCloudRegion,
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
	%s.
`[1:]

var unknownClusterErrMsg = `
	Juju needs to query the k8s cluster to ensure that the recommended
	storage defaults are available and to detect the cluster's cloud/region.
	This was not possible in this case because the cloud %q is not known to Juju.
	Run add-k8s again, using --storage=<name> to specify the storage class to use.
`[1:]

var noClusterSpecifiedErrMsg = `
	Juju needs to know what storage class to use to provision workload storage.
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

	cloudName := c.caasName
	credentialName := c.caasName

	credentialUID, err := c.credentialUIDGetter(c.Store, cloudName, credentialName)
	if err != nil {
		return errors.Trace(err)
	}

	var k8sConfig *clientcmdapi.Config
	if rdr != nil {
		k8sConfig, err = k8scloud.ConfigFromReader(rdr)
	} else {
		k8sConfig, err = clientconfig.GetLocalKubeConfig()
	}

	if err != nil {
		return errors.Annotate(err, "processing kubernetes config for add-k8s")
	}

	k8sCtxName := c.contextName
	if c.contextName == "" && clusterName == "" {
		k8sCtxName = k8sConfig.CurrentContext
	} else if c.contextName == "" && clusterName != "" {
		k8sCtxName, err = k8scloud.PickContextByClusterName(k8sConfig, clusterName)
		if err != nil {
			return errors.Trace(err)
		}
	}

	k8sConfig, err = c.adminServiceAccountResolver(c.clock)(
		credentialUID,
		k8sConfig,
		k8sCtxName,
	)

	if err != nil {
		return errors.Annotate(err, "making juju admin credentials in cluster")
	}

	newCloud, err := k8scloud.CloudFromKubeConfigContext(
		k8sCtxName,
		k8sConfig,
		k8scloud.CloudParamaters{
			Name:            cloudName,
			HostCloudRegion: c.hostCloudRegion,
		},
	)
	if err != nil {
		return errors.Trace(err)
	}

	newCredential, err := k8scloud.CredentialFromKubeConfigContext(k8sCtxName, k8sConfig)
	if err != nil {
		return errors.Trace(err)
	}

	if newCloud.SkipTLSVerify {
		if len(newCloud.CACertificates) > 0 && newCloud.CACertificates[0] != "" {
			return errors.NotValidf("cloud with both skip-TLS-verify=true and CA certificates")
		}
		logger.Warningf("k8s cloud %v is configured to skip server certificate validity checks", newCloud.Name)
	}
	newCredential, err = ensureCredentialUID(credentialName, credentialUID, newCredential)
	if err != nil {
		return errors.Trace(err)
	}
	// We need to have c.ControllerName after this, so this is the latest time to
	// prompt user for client and controller options.
	// We need to do this later then other commands since
	// piping is done regularly with add-k8s.
	if err := c.MaybePrompt(ctx, fmt.Sprintf("add k8s cloud %v to", c.caasName)); err != nil {
		return errors.Trace(err)
	}

	broker, err := c.brokerGetter(newCloud, newCredential)
	if err != nil {
		return errors.Trace(err)
	}
	var storageMsg string
	if c.skipStorage {
		storageMsg = " with no configured storage provisioning capability"
	} else {
		storageParams := provider.KubeCloudStorageParams{
			WorkloadStorage:        c.workloadStorage,
			HostCloudRegion:        c.hostCloudRegion,
			MetadataChecker:        broker,
			GetClusterMetadataFunc: c.getClusterMetadataFunc(ctx),
		}

		var err error
		storageMsg, err = provider.UpdateKubeCloudWithStorage(&newCloud, storageParams)
		if err != nil {
			if provider.IsClusterQueryError(err) {
				cloudArg := "--cloud=<cloud> to specify the cloud"
				if c.ControllerName == "jaas" {
					cloudArg = "--region=<cloud>/<someregion> to specify the cloud/region"
				}
				if err.Error() == "" {
					return errors.Errorf(clusterQueryErrMsg, cloudArg)
				}
				return errors.Annotatef(err, clusterQueryErrMsg, cloudArg)
			}
			if provider.IsNoRecommendedStorageError(err) {
				return errors.Errorf(noRecommendedStorageErrMsg, err.(provider.NoRecommendedStorageError).StorageProvider())
			}
			if provider.IsUnknownClusterError(err) {
				cloudName := err.(provider.UnknownClusterError).CloudName
				if cloudName == "" {
					return errors.New(noClusterSpecifiedErrMsg)
				}
				return errors.Errorf(unknownClusterErrMsg, cloudName)
			}
			return errors.Trace(err)
		}
	}

	if newCloud.HostCloudRegion != "" {
		newCloud.HostCloudRegion, err = c.validateCloudRegion(ctx, newCloud.HostCloudRegion)
		if err != nil {
			return errors.Trace(err)
		}
	}
	// By this stage, we know if cloud name/type and/or region input is needed from the user.
	// If we could not detect it, check what was provided.
	if err := checkCloudRegion(c.givenHostCloudRegion, newCloud.HostCloudRegion); err != nil {
		return errors.Trace(err)
	}

	if newCloud.HostCloudRegion == "" {
		newCloud.HostCloudRegion = caas.K8sCloudOther
	}

	var returnErr error
	if c.Client {
		personal, err := c.cloudMetadataStore.PersonalCloudMetadata()
		if err != nil {
			return errors.Trace(err)
		}
		for name := range personal {
			if name == newCloud.Name {
				returnErr = errors.AlreadyExistsf("use `update-k8s %s --client` to override known local definition: k8s %q", newCloud.Name, newCloud.Name)
				break
			}
		}
		if returnErr == nil {
			if err := addCloudToLocal(c.cloudMetadataStore, newCloud); err != nil {
				returnErr = err
			} else {
				if err := c.addCredentialToLocal(c.credentialStoreAPI, cloudName, newCredential, credentialName); err != nil {
					returnErr = err
				}
			}
		}
	}

	if clusterName == "" && newCloud.HostCloudRegion != caas.K8sCloudOther {
		clusterName = newCloud.HostCloudRegion
	}
	if clusterName != "" {
		clusterName = fmt.Sprintf("%q ", clusterName)
	}
	successMsg := fmt.Sprintf("k8s substrate %sadded as cloud %q%s", clusterName, c.caasName, storageMsg)
	var msgDisplayed bool
	if c.Client && returnErr == nil {
		msgDisplayed = true
		successMsg += fmt.Sprintf(".\nYou can now bootstrap to this cloud by running 'juju bootstrap %s'.", c.caasName)
		fmt.Fprintln(ctx.Stdout, successMsg)
	}
	if c.ControllerName != "" {
		// If there was an error adding locally, adjust the success message.
		if returnErr != nil {
			successMsg += "."
		} else {
			successMsg += fmt.Sprintf(" on controller %s.", c.ControllerName)
		}
		if err := c.addRemoteCloud(newCloud, newCredential, credentialName); err == nil {
			if !msgDisplayed {
				fmt.Fprintln(ctx.Stdout, successMsg)
			}
		} else {
			returnErr = errors.Annotate(err, "could not upload k8s cloud to a controller")
		}
	}
	return returnErr
}

func (c *AddCAASCommand) addRemoteCloud(newCloud jujucloud.Cloud, newCredential jujucloud.Credential, credentialName string) error {
	if err := jujuclient.ValidateControllerName(c.ControllerName); err != nil {
		return errors.Trace(err)
	}
	cloudClient, err := c.addCloudAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer cloudClient.Close()

	if err := addCloudToController(cloudClient, newCloud); err != nil {
		return errors.Trace(err)
	}
	if err := c.addCredentialToController(cloudClient, newCredential, newCloud.Name, credentialName); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func checkCloudRegion(given, detected string) error {
	if given == "" {
		// User provided no host cloud/region information.
		return nil
	}
	givenCloud, givenRegion, _ := jujucloud.SplitHostCloudRegion(given)
	detectedCloud, detectedRegion, _ := jujucloud.SplitHostCloudRegion(detected)
	if givenCloud != "" && givenCloud != detectedCloud {
		if givenRegion != "" || givenCloud != detectedRegion {
			// If givenRegion is empty, then givenCloud may be a region.
			// Check that it is not a region.
			return errors.Errorf("specified cloud %q was different to the detected cloud %q: re-run the command without specifying the cloud", givenCloud, detectedCloud)
		}
	}
	if givenRegion != "" && givenRegion != detectedRegion {
		return errors.Errorf("specified region %q was different to the detected region %q: re-run the command without specifying the region", givenRegion, detectedRegion)
	}
	return nil
}

func (c *AddCAASCommand) newK8sClusterBroker(cloud jujucloud.Cloud, credential jujucloud.Credential) (caas.ClusterMetadataChecker, error) {
	openParams, err := provider.BaseKubeCloudOpenParams(cloud, credential)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if c.ControllerName != "" {
		ctrlUUID, err := c.ControllerUUID(c.Store, c.ControllerName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		openParams.ControllerUUID = ctrlUUID
	}
	return caas.New(stdcontext.TODO(), openParams)
}

func getCloudAndRegionFromOptions(cloudOption, regionOption string) (string, string, error) {
	regionIsMaybeCloudRegion := strings.Contains(regionOption, "/")
	if cloudOption == "" && regionOption != "" && !regionIsMaybeCloudRegion {
		return "", "", errors.NewNotValid(nil, "when --region is used, --cloud is required")
	}
	cloudNameOrType := ""
	region := regionOption
	var err error
	if regionIsMaybeCloudRegion {
		cloudNameOrType, region, err = jujucloud.SplitHostCloudRegion(regionOption)
		if err != nil && cloudOption == "" {
			return "", "", errors.Annotate(err, "parsing region option")
		}
	}
	if cloudOption != "" && cloudNameOrType != "" {
		return "", "", errors.NewNotValid(nil, "when --cloud is used, --region may only specify a region, not a cloud/region")
	}
	c, r, _ := jujucloud.SplitHostCloudRegion(cloudOption)
	if region == "" && c != "" {
		// --cloud ec2 --region us-east-1
		region = r
		cloudNameOrType = c
	}
	if r != "" {
		return "", "", errors.NewNotValid(nil, "--cloud incorrectly specifies a cloud/region instead of just a cloud")
	}
	if cloudNameOrType == "" {
		cloudNameOrType = c
	}
	return cloudNameOrType, region, nil
}

// tryEnsureCloudType try to find cloud type if the cloudNameOrType is cloud name.
func (c *AddCAASCommand) tryEnsureCloudTypeForHostRegion(cloudOption, regionOption string) (string, error) {
	logger.Debugf("cloud option %q region option %q", cloudOption, regionOption)
	cloudNameOrType, region, err := getCloudAndRegionFromOptions(cloudOption, regionOption)
	if err != nil {
		return "", errors.Annotate(err, "parsing cloud region")
	}
	logger.Debugf("cloud %q region %q", cloudNameOrType, region)

	clouds, err := c.getAllCloudDetails(c.Store)
	if err != nil {
		return "", errors.Annotate(err, "listing cloud regions")
	}
	for name, details := range clouds {
		// User may have specified cloud name or type so match on both.
		if name == cloudNameOrType || details.CloudType == cloudNameOrType {
			cloudNameOrType = details.CloudType
		}
	}
	return jujucloud.BuildHostCloudRegion(cloudNameOrType, region), nil
}

func isRegionOptional(cloudType string) bool {
	for _, v := range []string{
		// Region is optional for CDK on microk8s, openstack, lxd, maas;
		caas.K8sCloudMicrok8s,
		caas.K8sCloudOpenStack,
		caas.K8sCloudLXD,
		caas.K8sCloudMAAS,
	} {
		if cloudType == v {
			return true
		}
	}
	return false
}

func (c *AddCAASCommand) validateCloudRegion(ctx *cmd.Context, cloudRegion string) (_ string, err error) {
	defer errors.DeferredAnnotatef(&err, "validating cloud region %q", cloudRegion)

	cloudType, region, err := jujucloud.SplitHostCloudRegion(cloudRegion)
	if err != nil {
		return "", errors.Annotate(err, "parsing cloud region")
	}
	// microk8s is special.
	if cloudType == caas.K8sCloudMicrok8s && region == caas.Microk8sRegion {
		return cloudRegion, nil
	}

	clouds, err := c.getAllCloudDetails(c.Store)
	if err != nil {
		return "", errors.Annotate(err, "listing cloud regions")
	}
	regionListMsg := ""
	for _, details := range clouds {
		// User may have specified cloud name or type so match on both.
		if details.CloudType == cloudType {
			if isRegionOptional(details.CloudType) && region == "" {
				return jujucloud.BuildHostCloudRegion(details.CloudType, ""), nil
			}
			if len(details.RegionsMap) == 0 {
				if region != "" {
					return "", errors.NewNotValid(nil, fmt.Sprintf(
						"cloud %q does not have a region, but %q provided", cloudType, region,
					))
				}
				return details.CloudType, nil
			}
			if region == "" && details.DefaultRegion != "" {
				logger.Debugf("cloud region not provided by user, using client default %q", details.DefaultRegion)
				region = details.DefaultRegion
			}
			for k := range details.RegionsMap {
				if k == region {
					logger.Debugf("cloud region %q is valid", cloudRegion)
					return jujucloud.BuildHostCloudRegion(details.CloudType, region), nil
				}
				regionListMsg += fmt.Sprintf("\t%q\n", k)
			}
		}
	}
	ctx.Infof("Supported regions for cloud %q: \n%s", cloudType, regionListMsg)
	return "", errors.NotValidf("cloud region %q", cloudRegion)
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
	public, _, err := c.cloudMetadataStore.PublicCloudMetadata()
	if err != nil {
		return err
	}
	msg, err := nameExists(name, public)
	if err != nil {
		return errors.Trace(err)
	}
	if msg != "" {
		return errors.New(msg)
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
		return errors.Trace(err)
	}
	if personalClouds == nil {
		personalClouds = make(map[string]jujucloud.Cloud)
	}
	personalClouds[newCloud.Name] = newCloud
	return cloudMetadataStore.WritePersonalCloudMetadata(personalClouds)
}

func addCloudToController(apiClient AddCloudAPI, newCloud jujucloud.Cloud) error {
	// No need to force this addition as k8s is special.
	err := apiClient.AddCloud(newCloud, false)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *AddCAASCommand) addCredentialToLocal(store CredentialStoreAPI, cloudName string, newCredential jujucloud.Credential, credentialName string) error {
	newCredentials := &jujucloud.CloudCredential{
		AuthCredentials: make(map[string]jujucloud.Credential),
	}
	newCredentials.AuthCredentials[credentialName] = newCredential
	err := store.UpdateCredential(cloudName, *newCredentials)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *AddCAASCommand) addCredentialToController(apiClient AddCloudAPI, newCredential jujucloud.Credential, cloudName, credentialName string) error {
	_, err := c.Store.ControllerByName(c.ControllerName)
	if err != nil {
		return errors.Trace(err)
	}

	currentAccountDetails, err := c.Store.AccountDetails(c.ControllerName)
	if err != nil {
		return errors.Trace(err)
	}

	id := fmt.Sprintf("%s/%s/%s", cloudName, currentAccountDetails.User, credentialName)
	if !names.IsValidCloudCredential(id) {
		return errors.NotValidf("cloud credential ID %q", id)
	}
	cloudCredTag := names.NewCloudCredentialTag(id)

	if err := apiClient.AddCredential(cloudCredTag.String(), newCredential); err != nil {
		return errors.Trace(err)
	}
	return nil
}
