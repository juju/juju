// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

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

// NewK8sClientConfig returns a new Kubernetes client, reading the config from the specified reader.
func NewK8sClientConfig(
	credentialUID string, reader io.Reader,
	contextName, clusterName string,
	credentialResolver K8sCredentialResolver,
) (*ClientConfig, error) {
	if reader == nil {
		var err error
		reader, err = readKubeConfigFile()
		if err != nil {
			return nil, errors.Annotate(err, "failed to read Kubernetes config file")
		}
	}

	content, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read Kubernetes config")
	}

	config, err := parseKubeConfig(content)
	if err != nil {
		return nil, errors.Annotate(err, "failed to parse Kubernetes config")
	}

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
	credentials, err := credentialsFromConfig(config, context.CredentialName)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read credentials from kubernetes config")
	}
	return &ClientConfig{
		Type:           "kubernetes",
		Contexts:       contexts,
		CurrentContext: config.CurrentContext,
		Clouds:         clouds,
		Credentials:    credentials,
	}, nil
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
			caData, err := ioutil.ReadFile(cluster.CertificateAuthority)
			if err != nil {
				return CloudConfig{}, errors.Trace(err)
			}
			k8sCAData = caData
		}
		attrs["CAData"] = string(k8sCAData)
		attrs["InsecureSkipTLSVerify"] = cluster.InsecureSkipTLSVerify

		return CloudConfig{
			Endpoint:   cluster.Server,
			Attributes: attrs,
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

func credentialsFromConfig(config *clientcmdapi.Config, credentialName string) (map[string]cloud.Credential, error) {

	authInfoToCredential := func(name string, user *clientcmdapi.AuthInfo) (cred cloud.Credential, err error) {
		logger.Debugf("name %q, user %#v", name, user)

		var hasCert bool
		attrs := map[string]string{}

		// TODO(axw) if the certificate/key are specified by path,
		// then we should just store the path in the credential,
		// and rely on credential finalization to read it at time
		// of use.

		if user.ClientCertificate != "" {
			certData, err := ioutil.ReadFile(user.ClientCertificate)
			if err != nil {
				return cred, errors.Trace(err)
			}
			user.ClientCertificateData = certData
		}

		if user.ClientKey != "" {
			keyData, err := ioutil.ReadFile(user.ClientKey)
			if err != nil {
				return cred, errors.Trace(err)
			}
			user.ClientKeyData = keyData
		}

		if len(user.ClientCertificateData) > 0 {
			attrs["ClientCertificateData"] = string(user.ClientCertificateData)
			hasCert = true
		}
		hasClientKeyData := len(user.ClientKeyData) > 0
		if hasClientKeyData {
			attrs["ClientKeyData"] = string(user.ClientKeyData)
		}
		hasToken := user.Token != ""
		if hasToken {
			if user.Username != "" || user.Password != "" {
				return cred, errors.NotValidf("AuthInfo: %q with both Token and User/Pass", name)
			}
			attrs["Token"] = user.Token
		}

		var authType cloud.AuthType
		if hasClientKeyData {
			// auth type used for aks for example.
			authType = cloud.OAuth2AuthType
			if hasCert {
				authType = cloud.OAuth2WithCertAuthType
			}
			if !hasToken {
				// the Token is required.
				return cred, errors.NotValidf("missing token for %q with auth type %q", name, authType)
			}
		} else if user.Username != "" {
			// basic auth type.
			if user.Password == "" {
				logger.Debugf("credential for user %q has empty password", user.Username)
			}
			attrs["username"] = user.Username
			attrs["password"] = user.Password
			if hasCert {
				authType = cloud.UserPassWithCertAuthType
			} else {
				authType = cloud.UserPassAuthType
			}
		} else if hasCert && hasToken {
			// bearer token of service account auth type gke for example.
			authType = cloud.CertificateAuthType
		} else {
			return cred, errors.NotSupportedf("configuration for %q", name)
		}

		return cloud.NewNamedCredential(name, authType, attrs, false), nil
	}

	authInfos := config.AuthInfos
	if credentialName != "" {
		authInfo, ok := authInfos[credentialName]
		if !ok {
			return nil, errors.NotFoundf("authInfo %q", credentialName)
		}
		authInfos = map[string]*clientcmdapi.AuthInfo{credentialName: authInfo}
	}
	rv := map[string]cloud.Credential{}
	for name, user := range authInfos {
		cred, err := authInfoToCredential(name, user)
		if err != nil {
			return nil, errors.Trace(err)
		}
		rv[name] = cred
	}
	return rv, nil
}

// GetKubeConfigPath - define kubeconfig file path to use
func GetKubeConfigPath() string {
	kubeconfig := os.Getenv(clientcmd.RecommendedConfigPathEnvVar)
	if kubeconfig == "" {
		kubeconfig = clientcmd.RecommendedHomeFile
	}
	logger.Debugf("The kubeconfig file path: %q", kubeconfig)
	return kubeconfig
}

func readKubeConfigFile() (reader io.Reader, err error) {
	// Try to read from kubeconfig file.
	filename := GetKubeConfigPath()
	reader, err = os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFoundf(filename)
		}
		return nil, errors.Trace(errors.Annotatef(err, "failed to read kubernetes config from '%s'", filename))
	}
	return reader, nil
}

func parseKubeConfig(data []byte) (*clientcmdapi.Config, error) {
	config, err := clientcmd.Load(data)
	if err != nil {
		return nil, err
	}

	if config.AuthInfos == nil {
		config.AuthInfos = map[string]*clientcmdapi.AuthInfo{}
	}
	if config.Clusters == nil {
		config.Clusters = map[string]*clientcmdapi.Cluster{}
	}
	if config.Contexts == nil {
		config.Contexts = map[string]*clientcmdapi.Context{}
	}

	return config, nil
}
