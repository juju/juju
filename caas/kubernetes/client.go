// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"

	"github.com/juju/errors"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.kubernetes")

// NewK8sClients returns the k8s clients to access a cluster.
// Override for testing.
var NewK8sClients = func(c *rest.Config) (
	k8sClient kubernetes.Interface,
	apiextensionsclient apiextensionsclientset.Interface,
	dynamicClient dynamic.Interface,
	err error,
) {
	k8sClient, err = kubernetes.NewForConfig(c)
	if err != nil {
		return nil, nil, nil, err
	}
	apiextensionsclient, err = apiextensionsclientset.NewForConfig(c)
	if err != nil {
		return nil, nil, nil, err
	}
	dynamicClient, err = dynamic.NewForConfig(c)
	if err != nil {
		return nil, nil, nil, err
	}
	return k8sClient, apiextensionsclient, dynamicClient, nil
}

// CloudSpecToK8sRestConfig translates cloudspec to k8s rest config.
func CloudSpecToK8sRestConfig(cloudSpec environscloudspec.CloudSpec) (*rest.Config, error) {
	if cloudSpec.IsControllerCloud {
		rc, err := rest.InClusterConfig()
		if err != nil && err != rest.ErrNotInCluster {
			return nil, errors.Trace(err)
		}
		if rc != nil {
			logger.Tracef(context.TODO(), "using in-cluster config")
			return rc, nil
		}
	}

	if cloudSpec.Credential == nil {
		return nil, errors.Errorf("cloud %v has no credential", cloudSpec.Name)
	}

	var caData []byte
	for _, cacert := range cloudSpec.CACertificates {
		caData = append(caData, cacert...)
	}

	credentialAttrs := cloudSpec.Credential.Attributes()
	return &rest.Config{
		Host:        cloudSpec.Endpoint,
		Username:    credentialAttrs[k8scloud.CredAttrUsername],
		Password:    credentialAttrs[k8scloud.CredAttrPassword],
		BearerToken: credentialAttrs[k8scloud.CredAttrToken],
		TLSClientConfig: rest.TLSClientConfig{
			CertData: []byte(credentialAttrs[k8scloud.CredAttrClientCertificateData]),
			KeyData:  []byte(credentialAttrs[k8scloud.CredAttrClientKeyData]),
			CAData:   caData,
			Insecure: cloudSpec.SkipTLSVerify,
		},
	}, nil
}
