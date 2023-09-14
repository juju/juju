// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/v3"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
)

var logger = loggo.GetLogger("juju.caas.kubernetes.clientconfig")

// K8sCredentialResolver defines the function for resolving non supported k8s credential.
type K8sCredentialResolver func(string, *clientcmdapi.Config, string) (*clientcmdapi.Config, error)

// GetJujuAdminServiceAccountResolver returns a function for ensuring juju admin service account created with admin cluster role binding setup.
func GetJujuAdminServiceAccountResolver(clock jujuclock.Clock) K8sCredentialResolver {
	return func(credentialUID string, config *clientcmdapi.Config, contextName string) (*clientcmdapi.Config, error) {
		clientset, err := newK8sClientSet(config, contextName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ensureJujuAdminServiceAccount(clientset, credentialUID, config, contextName, clock)
	}
}

// GetLocalKubeConfig attempts to load up the current users local Kubernetes
// configuration.
func GetLocalKubeConfig() (*clientcmdapi.Config, error) {
	// Confined snaps mess with the home path so ensure we
	// include that in the config loader.
	possibleRealHome := filepath.Join(utils.Home(),
		clientcmd.RecommendedHomeDir, clientcmd.RecommendedFileName)
	loader := clientcmd.NewDefaultClientConfigLoadingRules()
	realHomeUsed := false
	for i, opt := range loader.Precedence {
		// Insert the real home before the sandboxed snap home.
		if opt == clientcmd.RecommendedHomeFile {
			precedence := append([]string{}, loader.Precedence[:i]...)
			precedence = append(precedence, possibleRealHome)
			precedence = append(precedence, loader.Precedence[i:]...)
			loader.Precedence = precedence
			realHomeUsed = true
			break
		}
	}
	if !realHomeUsed {
		loader.Precedence = append(loader.Precedence, possibleRealHome)
	}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loader,
		&clientcmd.ConfigOverrides{},
	)
	r, err := kubeConfig.RawConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &r, nil
}

// configFromPossibleReader attempts to read in kubeconfig from the supplied
// reader, otherwise defaulting over to the default Kubernetes config loading
func configFromPossibleReader(reader io.Reader) (*clientcmdapi.Config, error) {
	if reader != nil {
		contents, err := io.ReadAll(reader)
		if err != nil {
			return nil, errors.Annotate(err, "failed to read Kubernetes config")
		}
		config, err := clientcmd.Load(contents)
		if err != nil {
			return nil, errors.Annotate(err, "failed parsing Kubernetes config from reader")
		}
		return config, nil
	}
	return nil, errors.NotFoundf("kubernetes config in reader")
}

func NewK8sClientConfig(
	credentialUID string, config *clientcmdapi.Config,
	contextName, clusterName string,
	credentialResolver K8sCredentialResolver,
) (*ClientConfig, error) {
	contexts, err := contextsFromConfig(config)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read contexts from kubernetes config")
	}
	var context Context
	if contextName == "" {
		contextName = config.CurrentContext
	}
	if clusterName != "" {
		context, contextName, err = pickContextByClusterName(contexts, clusterName)
		if err != nil {
			return nil, errors.Annotatef(err, "picking context by cluster name %q", clusterName)
		}
	} else if contextName != "" {
		context = contexts[contextName]
		logger.Debugf("no cluster name specified, so use current context %q", config.CurrentContext)
	}

	if contextName == "" || context.isEmpty() {
		return nil, errors.NewNotFound(nil,
			fmt.Sprintf("no context found for context name: %q, cluster name: %q", contextName, clusterName))
	}
	// Exclude not related contexts.
	contexts = map[string]Context{contextName: context}

	// try find everything below based on context.
	clouds, err := cloudsFromConfig(config, context.CloudName)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read clouds from kubernetes config")
	}

	if credentialResolver != nil {
		// Try to create service account, cluster role and cluster role binding for k8s credential using provided credential.
		// Name credential resources using cloud name.
		config, err = credentialResolver(credentialUID, config, contextName)
		if err != nil {
			return nil, errors.Annotatef(err, "ensuring k8s credential %q with RBAC setup", credentialUID)
		}
	}
	logger.Debugf("get credentials from kubeconfig")
	credential, err := k8scloud.CredentialFromKubeConfig(context.CredentialName, config)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read credentials from kubernetes config")
	}
	return &ClientConfig{
		Type:           "kubernetes",
		Contexts:       contexts,
		CurrentContext: config.CurrentContext,
		Clouds:         clouds,
		Credentials:    map[string]cloud.Credential{context.CredentialName: credential},
	}, nil
}

// NewK8sClientConfigFromReader returns a new Kubernetes client, reading the config from the specified reader.
func NewK8sClientConfigFromReader(
	credentialUID string, reader io.Reader,
	contextName, clusterName string,
	credentialResolver K8sCredentialResolver,
) (*ClientConfig, error) {
	config, err := configFromPossibleReader(reader)
	if errors.Is(err, errors.NotFound) {
		config, err = GetLocalKubeConfig()
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewK8sClientConfig(
		credentialUID,
		config,
		contextName,
		clusterName,
		credentialResolver)
}

func pickContextByClusterName(contexts map[string]Context, clusterName string) (Context, string, error) {
	for contextName, context := range contexts {
		if clusterName == context.CloudName {
			return context, contextName, nil
		}
	}
	return Context{}, "", errors.NotFoundf("context for cluster name %q", clusterName)
}

func contextsFromConfig(config *clientcmdapi.Config) (map[string]Context, error) {
	rv := map[string]Context{}
	for name, ctx := range config.Contexts {
		rv[name] = Context{
			CredentialName: ctx.AuthInfo,
			CloudName:      ctx.Cluster,
		}
	}
	return rv, nil
}

func cloudsFromConfig(config *clientcmdapi.Config, cloudName string) (map[string]CloudConfig, error) {
	clusterToCloud := func(cluster *clientcmdapi.Cluster) (CloudConfig, error) {
		attrs := map[string]interface{}{}

		k8sCAData := cluster.CertificateAuthorityData
		if len(cluster.CertificateAuthorityData) == 0 && cluster.CertificateAuthority != "" {
			caData, err := os.ReadFile(cluster.CertificateAuthority)
			if err != nil {
				return CloudConfig{}, errors.Trace(err)
			}
			k8sCAData = caData
		}
		attrs["CAData"] = string(k8sCAData)

		return CloudConfig{
			Endpoint:      cluster.Server,
			SkipTLSVerify: cluster.InsecureSkipTLSVerify,
			Attributes:    attrs,
		}, nil
	}

	clusters := config.Clusters
	if cloudName != "" {
		cluster, ok := clusters[cloudName]
		if !ok {
			return nil, errors.NotFoundf("cluster %q", cloudName)
		}
		clusters = map[string]*clientcmdapi.Cluster{cloudName: cluster}
	}

	rv := map[string]CloudConfig{}
	for name, cluster := range clusters {
		c, err := clusterToCloud(cluster)
		if err != nil {
			return nil, errors.Trace(err)
		}
		rv[name] = c
	}
	return rv, nil
}

// GetKubeConfigPath returns the most likely kube config file to use. It first
// looks at all the files defined in the user KUBECONF env var and selects the
// first available. If the list is empty the default kube config path is
// returned.
func GetKubeConfigPath() string {
	pathOpts := clientcmd.NewDefaultPathOptions()
	// Confined snaps mess with the home path so ensure we
	// include that in the config loader.
	possibleRealHome := filepath.Join(utils.Home(),
		clientcmd.RecommendedHomeDir, clientcmd.RecommendedFileName)
	pathOpts.LoadingRules.Precedence = append([]string{possibleRealHome}, pathOpts.LoadingRules.Precedence...)
	envFiles := pathOpts.GetEnvVarFiles()
	if len(envFiles) == 0 {
		configPath := pathOpts.LoadingRules.GetDefaultFilename()
		if configPath == "" {
			configPath = pathOpts.GetDefaultFilename()
		}
		return configPath
	}
	logger.Debugf("The kubeconfig file path is %s", envFiles[0])
	return envFiles[0]
}
