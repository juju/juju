package caas

import (
	"os"
	"path/filepath"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/cloud"
)

var logger = loggo.GetLogger("juju.caas.clientconfig")

type K8SClientConfigReader struct {
}

func (r *K8SClientConfigReader) GetClientConfig() (*ClientConfig, error) {

	configPath := getKubeConfigPath()

	config, err := clientcmd.LoadFromFile(configPath)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to read kubernetes config from '%s'", configPath)
	}
	contexts, err := contextsFromConfig(config)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read contexts from kubernetes config.")
	}

	clouds, err := cloudsFromConfig(config)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read clouds from kubernetes config.")
	}

	credentials, err := credentialsFromConfig(config)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read credentials from kubernetes config.")
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
		attrs["CAData"] = cluster.CertificateAuthorityData

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
		var hasCert bool
		attrs := map[string]string{}
		if len(user.ClientCertificateData) > 0 {
			attrs["ClientCertificateData"] = string(user.ClientCertificateData[:])
			hasCert = true
		}
		if len(user.ClientKeyData) > 0 {
			attrs["ClientKeyData"] = string(user.ClientKeyData[:])
		}

		var authType cloud.AuthType
		if user.Token != "" {
			if user.Username != "" || user.Password != "" {
				logger.Warningf("Invalid AuthInfo: '%s' has both Token and User/Pass: skipping", name)
				continue
			}
			attrs["Token"] = user.Token
			if hasCert {
				authType = cloud.OAuth2WithCertAuthType
			} else {
				authType = cloud.OAuth2AuthType
			}
		} else if user.Username != "" {
			if user.Password == "" {
				logger.Warningf("empty password")
			}
			attrs["Username"] = user.Username
			attrs["Password"] = user.Password
			if hasCert {
				authType = cloud.UserPassWithCertAuthType
			} else {
				authType = cloud.UserPassAuthType
			}
		} else if hasCert {
			authType = cloud.CertificateAuthType
		} else {
			logger.Warningf("Unsupported configuration for AuthInfo '%s'", name)
		}

		rv[name] = cloud.NewCredential(authType, attrs)
	}
	return rv, nil
}

func getKubeConfigPath() string {
	envPath := os.Getenv("KUBECONFIG")
	if envPath == "" {
		return filepath.Join(os.Getenv("HOME"), "/.kube/config")
	}
	return envPath
}
