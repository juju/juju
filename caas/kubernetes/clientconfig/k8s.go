// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/juju/juju/cloud"
)

var logger = loggo.GetLogger("juju.caas.kubernetes.clientconfig")

type k8sCredentialResolver func(config *clientcmdapi.Config, contextName string) (*clientcmdapi.Config, error)

// EnsureK8sCredential ensures juju admin service account created with admin cluster role binding setup.
func EnsureK8sCredential(config *clientcmdapi.Config, contextName string) (*clientcmdapi.Config, error) {
	clientset, err := newK8sClientSet(config, contextName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ensureJujuAdminServiceAccount(clientset, config, contextName)
}

// NewK8sClientConfig returns a new Kubernetes client, reading the config from the specified reader.
func NewK8sClientConfig(reader io.Reader, clusterName string, credentialResolver k8sCredentialResolver) (*ClientConfig, error) {
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

	var contextName string
	var context Context
	if clusterName != "" {
		context, contextName, err = pickContextByClusterName(contexts, clusterName)
		if err != nil {
			return nil, errors.Annotatef(err, "picking context by cluster name %q", clusterName)
		}
	} else if config.CurrentContext != "" {
		contextName = config.CurrentContext
		context = contexts[contextName]
		logger.Debugf("No cluster name specified, so use current context %q", config.CurrentContext)
	}
	// exclude on related contexts.
	contexts = map[string]Context{}
	if contextName != "" && !context.isEmpty() {
		contexts[contextName] = context
	}

	// try find everything below based on context.
	clouds, err := cloudsFromConfig(config, context.CloudName)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read clouds from kubernetes config")
	}

	credentials, err := credentialsFromConfig(config, context.CredentialName)
	if errors.IsNotSupported(err) && credentialResolver != nil {
		// try to generate supported credential using provided credential.
		config, err = credentialResolver(config, contextName)
		if err != nil {
			return nil, errors.Annotatef(
				err, "ensuring k8s credential because auth info %q is not valid", context.CredentialName)
		}
		// try again using the generated auth info.
		credentials, err = credentialsFromConfig(config, context.CredentialName)
	}
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

		// TODO(axw) if the CA cert is specified by path, then we
		// should just store the path in the cloud definition, and
		// rely on cloud finalization to read it at time of use.
		if cluster.CertificateAuthority != "" {
			caData, err := ioutil.ReadFile(cluster.CertificateAuthority)
			if err != nil {
				return CloudConfig{}, errors.Trace(err)
			}
			cluster.CertificateAuthorityData = caData
		}
		attrs["CAData"] = string(cluster.CertificateAuthorityData)

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

	authInfoToCredential := func(name string, user *clientcmdapi.AuthInfo) (cloud.Credential, error) {
		logger.Debugf("name %q, user %#v", name, user)

		var hasCert bool
		var cred cloud.Credential
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
		if len(user.ClientKeyData) > 0 {
			attrs["ClientKeyData"] = string(user.ClientKeyData)
		}

		var authType cloud.AuthType
		if user.Token != "" {
			if user.Username != "" || user.Password != "" {
				return cred, errors.NotValidf("AuthInfo: %q with both Token and User/Pass", name)
			}
			attrs["Token"] = user.Token
			if hasCert {
				authType = cloud.OAuth2WithCertAuthType
			} else {
				authType = cloud.OAuth2AuthType
			}
		} else if user.Username != "" {
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
		} else if hasCert {
			authType = cloud.CertificateAuthType
			if len(user.ClientKeyData) == 0 {
				return cred, errors.NotValidf("empty ClientKeyData for %q with auth type %q", name, authType)
			}
		} else {
			return cred, errors.NotSupportedf("configuration for %q", name)
		}

		cred = cloud.NewCredential(authType, attrs)
		cred.Label = fmt.Sprintf("kubernetes credential %q", name)
		return cred, nil
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

// getKubeConfigPath - define kubeconfig file path to use
func getKubeConfigPath() string {
	envPath := os.Getenv(clientcmd.RecommendedConfigPathEnvVar)
	if envPath == "" {
		return clientcmd.RecommendedHomeFile
	}
	logger.Debugf("The kubeconfig file path: %q", envPath)
	return envPath
}

func readKubeConfigFile() (reader io.Reader, err error) {
	// Try to read from kubeconfig file.
	filename := getKubeConfigPath()
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
