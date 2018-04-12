package clientconfig

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/juju/juju/cloud"
)

var logger = loggo.GetLogger("juju.caas.kubernetes.clientconfig")

// K8SClientConfig parses Kubernetes client configuration from the default location or $KUBECONFIG.
func K8SClientConfig() (*ClientConfig, error) {

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
				logger.Warningf("invalid AuthInfo: '%s' has both Token and User/Pass: skipping", name)
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
			attrs["username"] = user.Username
			attrs["password"] = user.Password
			if hasCert {
				authType = cloud.UserPassWithCertAuthType
			} else {
				authType = cloud.UserPassAuthType
			}
		} else if hasCert {
			authType = cloud.CertificateAuthType
		} else {
			logger.Warningf("unsupported configuration for AuthInfo '%s'", name)
		}

		cred := cloud.NewCredential(authType, attrs)
		cred.Label = fmt.Sprintf("kubernetes credential %q", name)
		rv[name] = cred
	}
	return rv, nil
}

func getKubeConfigPath() string {
	envPath := os.Getenv(clientcmd.RecommendedConfigPathEnvVar)
	if envPath == "" {
		return clientcmd.RecommendedHomeFile
	}
	return envPath
}
