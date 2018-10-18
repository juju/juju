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

// NewK8sClientConfig returns a new Kubernetes client, reading the config from the specified reader.
func NewK8sClientConfig(reader io.Reader) (*ClientConfig, error) {
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

	clouds, err := cloudsFromConfig(config)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read clouds from kubernetes config")
	}

	credentials, err := credentialsFromConfig(config)
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

func cloudsFromConfig(config *clientcmdapi.Config) (map[string]CloudConfig, error) {
	rv := map[string]CloudConfig{}
	for name, cluster := range config.Clusters {
		attrs := map[string]interface{}{}

		// TODO(axw) if the CA cert is specified by path, then we
		// should just store the path in the cloud definition, and
		// rely on cloud finalization to read it at time of use.
		if cluster.CertificateAuthority != "" {
			caData, err := ioutil.ReadFile(cluster.CertificateAuthority)
			if err != nil {
				return nil, errors.Trace(err)
			}
			cluster.CertificateAuthorityData = caData
		}
		attrs["CAData"] = string(cluster.CertificateAuthorityData)

		rv[name] = CloudConfig{
			Endpoint:   cluster.Server,
			Attributes: attrs,
		}
	}
	return rv, nil
}

func credentialsFromConfig(config *clientcmdapi.Config) (map[string]cloud.Credential, error) {
	rv := map[string]cloud.Credential{}
	for name, user := range config.AuthInfos {
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
				return nil, errors.Trace(err)
			}
			user.ClientCertificateData = certData
		}

		if user.ClientKey != "" {
			keyData, err := ioutil.ReadFile(user.ClientKey)
			if err != nil {
				return nil, errors.Trace(err)
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
				return nil, errors.NotValidf("AuthInfo: %q with both Token and User/Pass", name)
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
				return nil, errors.NotValidf("empty ClientKeyData for %q with auth type %q", name, authType)
			}
		} else {
			return nil, errors.NotSupportedf("configuration for %q", name)
		}

		cred := cloud.NewCredential(authType, attrs)
		cred.Label = fmt.Sprintf("kubernetes credential %q", name)
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
