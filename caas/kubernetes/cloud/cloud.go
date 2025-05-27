// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"io"

	"github.com/juju/errors"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/juju/juju/cloud"
)

// CloudParameters describes basic properties that should be set on a Juju
// cloud.Cloud object. This struct exists to help form Cloud structs from
// Kubernetes config structs.
type CloudParamaters struct {
	Name            string
	Description     string
	HostCloudRegion string
	Regions         []cloud.Region
}

// buildCloudFromCluster finishes building a cloud from the provided kube
// cluster
func buildCloudFromCluster(c *cloud.Cloud, cluster *clientcmdapi.Cluster) error {
	c.Endpoint = cluster.Server
	c.SkipTLSVerify = cluster.InsecureSkipTLSVerify
	c.AuthTypes = SupportedAuthTypes()

	clusterCAData, err := dataOrFile(cluster.CertificateAuthorityData, cluster.CertificateAuthority)
	if err != nil {
		return errors.Annotate(err, "getting cluster CA data")
	}
	c.CACertificates = []string{string(clusterCAData)}

	return nil
}

// ConfigFromReader does the heavy lifting of transforming a reader object into
// a kubernetes api config
func ConfigFromReader(reader io.Reader) (*clientcmdapi.Config, error) {
	confBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.Annotate(err, "reading kubernetes configuration data")
	}

	conf, err := clientcmd.NewClientConfigFromBytes(confBytes)
	if err != nil {
		return nil, errors.Annotate(err, "parsing kubernetes configuration data")
	}

	apiConf, err := conf.RawConfig()
	if err != nil {
		return nil, errors.Annotate(err, "fetching kubernetes configuration")
	}
	return &apiConf, nil
}

// CloudsFromKubeConfigContexts generates a list of clouds from the supplied
// config context slice
func CloudsFromKubeConfigContexts(config *clientcmdapi.Config) ([]cloud.Cloud, error) {
	return CloudsFromKubeConfigContextsWithParams(CloudParamaters{}, config)
}

// CloudsFromKubeConfigContextsWithParams generates a list of clouds from the
// supplied config context slice. Uses params to help seed values for the
// resulting clouds. Currently only description is taken from params attribute.
func CloudsFromKubeConfigContextsWithParams(
	params CloudParamaters,
	config *clientcmdapi.Config,
) ([]cloud.Cloud, error) {
	clouds := []cloud.Cloud{}
	for ctxName := range config.Contexts {
		cloud, err := CloudFromKubeConfigContext(
			ctxName,
			config,
			CloudParamaters{
				Description: params.Description,
				Name:        ctxName,
			},
		)
		if err != nil {
			return clouds, errors.Trace(err)
		}
		clouds = append(clouds, cloud)
	}
	return clouds, nil
}

// CloudFromKubeConfigContext generates a juju cloud based on the supplied
// context and config
func CloudFromKubeConfigContext(
	ctxName string,
	config *clientcmdapi.Config,
	params CloudParamaters,
) (cloud.Cloud, error) {
	newCloud := cloud.Cloud{
		Name:            params.Name,
		Type:            cloud.CloudTypeKubernetes,
		HostCloudRegion: params.HostCloudRegion,
		Regions:         params.Regions,
		Description:     params.Description,
	}

	context, exists := config.Contexts[ctxName]
	if !exists {
		return newCloud, errors.NotFoundf("kubernetes context %q", ctxName)
	}

	cluster, exists := config.Clusters[context.Cluster]
	if !exists {
		return newCloud, errors.NotFoundf("kubernetes cluster %q associated with context %q",
			context.Cluster, ctxName)
	}
	err := buildCloudFromCluster(&newCloud, cluster)
	return newCloud, err
}

// CloudFromKubeConfigContextReader constructs a Juju cloud object using the
// supplied Kubernetes context name and parsing the raw Kubernetes config
// located in reader.
func CloudFromKubeConfigContextReader(
	ctxName string,
	reader io.Reader,
	params CloudParamaters,
) (cloud.Cloud, error) {
	config, err := ConfigFromReader(reader)
	if err != nil {
		return cloud.Cloud{}, err
	}
	return CloudFromKubeConfigContext(ctxName, config, params)
}

// CloudFromKubeConfigCluster attempts to construct a Juju cloud object using
// the supplied Kubernetes config and the cluster name. This function attempts
// to find a context that it can leverage that uses the specificed cluster name.
// The first context using the cluster name is taken and if no options exists
// results in an error.
func CloudFromKubeConfigCluster(
	clusterName string,
	config *clientcmdapi.Config,
	params CloudParamaters,
) (cloud.Cloud, error) {
	newCloud := cloud.Cloud{
		Name:            params.Name,
		Type:            cloud.CloudTypeKubernetes,
		HostCloudRegion: params.HostCloudRegion,
		Regions:         params.Regions,
		Description:     params.Description,
	}

	cluster, exists := config.Clusters[clusterName]
	if !exists {
		return newCloud, errors.NotFoundf("kubernetes cluster %q not found", clusterName)
	}
	err := buildCloudFromCluster(&newCloud, cluster)
	return newCloud, err
}

// CloudFromKubeConfigClusterReader attempts to construct a Juju cloud object
// using the supplied raw Kubernetes config in reader and the cluster name. This
// function attempts to find a context that it can leverage that uses the
// specificed cluster name. The first context using the cluster name is taken
// and if no options exists results in an error.
func CloudFromKubeConfigClusterReader(
	clusterName string,
	reader io.Reader,
	params CloudParamaters,
) (cloud.Cloud, error) {
	config, err := ConfigFromReader(reader)
	if err != nil {
		return cloud.Cloud{}, err
	}
	return CloudFromKubeConfigCluster(clusterName, config, params)
}
