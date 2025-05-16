// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"
	"fmt"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8sutils "github.com/juju/juju/caas/kubernetes/provider/utils"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/testhelpers"
)

func TestCloudSuite(t *stdtesting.T) { tc.Run(t, &cloudSuite{}) }

type cloudSuite struct {
	fakeBroker fakeK8sClusterMetadataChecker
	runner     dummyRunner
}

var defaultK8sCloud = jujucloud.Cloud{
	Name:           k8s.K8sCloudMicrok8s,
	Endpoint:       "http://1.1.1.1:8080",
	Type:           jujucloud.CloudTypeKubernetes,
	AuthTypes:      []jujucloud.AuthType{jujucloud.UserPassAuthType},
	CACertificates: []string{""},
	SkipTLSVerify:  true,
}

var defaultClusterMetadata = &k8s.ClusterMetadata{
	Cloud:   k8s.K8sCloudMicrok8s,
	Regions: set.NewStrings(k8s.Microk8sRegion),
	WorkloadStorageClass: &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "workload-sc",
		},
	},
}

func getDefaultCredential() jujucloud.Credential {
	defaultCredential := jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{"username": "admin", "password": ""})
	defaultCredential.Label = "kubernetes credential \"admin\""
	return defaultCredential
}

func (s *cloudSuite) SetUpTest(c *tc.C) {
	var logger loggo.Logger
	s.fakeBroker = fakeK8sClusterMetadataChecker{CallMocker: testhelpers.NewCallMocker(logger)}
	s.runner = dummyRunner{CallMocker: testhelpers.NewCallMocker(logger)}
}

func (s *cloudSuite) TestFinalizeCloudMicrok8s(c *tc.C) {
	p := s.getProvider()
	cloudFinalizer := p.(environs.CloudFinalizer)
	s.fakeBroker.Call("ListStorageClasses", k8slabels.NewSelector()).Returns(
		[]storagev1.StorageClass{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "microk8s-hostpath",
					Annotations: map[string]string{
						"storageclass.kubernetes.io/is-default-class": "true",
					},
				},
			},
		}, nil,
	)
	s.fakeBroker.Call(
		"ListPods", "kube-system",
		k8sutils.LabelsToSelector(map[string]string{"k8s-app": "kube-dns"}),
	).Returns([]corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "coredns-xx",
				Labels: map[string]string{
					"k8s-app": "kube-dns",
				},
			},
		},
	}, nil)

	ctx := mockContext{Context: c.Context()}
	cloud, err := cloudFinalizer.FinalizeCloud(&ctx, defaultK8sCloud)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloud, tc.DeepEquals, jujucloud.Cloud{
		Name:            k8s.K8sCloudMicrok8s,
		Type:            jujucloud.CloudTypeKubernetes,
		AuthTypes:       []jujucloud.AuthType{jujucloud.UserPassAuthType},
		CACertificates:  []string{""},
		SkipTLSVerify:   true,
		Endpoint:        "http://1.1.1.1:8080",
		HostCloudRegion: fmt.Sprintf("%s/%s", k8s.K8sCloudMicrok8s, k8s.Microk8sRegion),
		Config:          map[string]interface{}{"workload-storage": "workload-sc"},
		Regions:         []jujucloud.Region{{Name: k8s.Microk8sRegion, Endpoint: "http://1.1.1.1:8080"}},
	})
}

func (s *cloudSuite) getProvider() caas.ContainerEnvironProvider {
	s.fakeBroker.Call("GetClusterMetadata").Returns(defaultClusterMetadata, nil)
	s.fakeBroker.Call("CheckDefaultWorkloadStorage").Returns(nil)
	ret := builtinCloudRet{cloud: defaultK8sCloud, credential: getDefaultCredential(), err: nil}
	return provider.NewProviderWithFakes(
		s.runner,
		credentialGetterFunc(ret),
		cloudGetterFunc(ret),
		func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (provider.ClusterMetadataStorageChecker, error) {
			return &s.fakeBroker, nil
		},
	)
}

func (s *cloudSuite) TestEnsureMicroK8sSuitableSuccess(c *tc.C) {
	s.fakeBroker.Call("ListStorageClasses", k8slabels.NewSelector()).Returns(
		[]storagev1.StorageClass{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "microk8s-hostpath",
					Annotations: map[string]string{
						"storageclass.kubernetes.io/is-default-class": "true",
					},
				},
			},
		}, nil,
	)
	s.fakeBroker.Call(
		"ListPods", "kube-system",
		k8sutils.LabelsToSelector(map[string]string{"k8s-app": "kube-dns"}),
	).Returns([]corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "coredns-xx",
				Labels: map[string]string{
					"k8s-app": "kube-dns",
				},
			},
		},
	}, nil)
	c.Assert(provider.EnsureMicroK8sSuitable(c.Context(), &s.fakeBroker), tc.ErrorIsNil)
}

func (s *cloudSuite) TestEnsureMicroK8sSuitableStorageNotEnabled(c *tc.C) {
	s.fakeBroker.Call("ListStorageClasses", k8slabels.NewSelector()).Returns(
		[]storagev1.StorageClass{}, nil,
	)
	err := provider.EnsureMicroK8sSuitable(c.Context(), &s.fakeBroker)
	c.Assert(err, tc.ErrorMatches, `required storage addon is not enabled`)
}

func (s *cloudSuite) TestEnsureMicroK8sSuitableDNSNotEnabled(c *tc.C) {
	s.fakeBroker.Call("ListStorageClasses", k8slabels.NewSelector()).Returns(
		[]storagev1.StorageClass{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "microk8s-hostpath",
					Annotations: map[string]string{
						"storageclass.kubernetes.io/is-default-class": "true",
					},
				},
			},
		}, nil,
	)
	s.fakeBroker.Call(
		"ListPods", "kube-system",
		k8sutils.LabelsToSelector(map[string]string{"k8s-app": "kube-dns"}),
	).Returns([]corev1.Pod{}, nil)
	err := provider.EnsureMicroK8sSuitable(c.Context(), &s.fakeBroker)
	c.Assert(err, tc.ErrorMatches, `required dns addon is not enabled`)
}

type mockContext struct {
	context.Context
	testhelpers.Stub
}

func (c *mockContext) Verbosef(f string, args ...interface{}) {
	c.MethodCall(c, "Verbosef", f, args)
}

type fakeK8sClusterMetadataChecker struct {
	*testhelpers.CallMocker
	k8s.ClusterMetadataChecker
}

func (api *fakeK8sClusterMetadataChecker) GetClusterMetadata(_ context.Context, storageClass string) (result *k8s.ClusterMetadata, err error) {
	results := api.MethodCall(api, "GetClusterMetadata")
	return results[0].(*k8s.ClusterMetadata), testhelpers.TypeAssertError(results[1])
}

func (api *fakeK8sClusterMetadataChecker) CheckDefaultWorkloadStorage(cluster string, storageProvisioner *k8s.StorageProvisioner) error {
	results := api.MethodCall(api, "CheckDefaultWorkloadStorage")
	return testhelpers.TypeAssertError(results[0])
}

func (api *fakeK8sClusterMetadataChecker) EnsureStorageProvisioner(cfg k8s.StorageProvisioner) (*k8s.StorageProvisioner, bool, error) {
	results := api.MethodCall(api, "EnsureStorageProvisioner")
	return results[0].(*k8s.StorageProvisioner), false, testhelpers.TypeAssertError(results[1])
}

func (api *fakeK8sClusterMetadataChecker) ListPods(_ context.Context, namespace string, selector k8slabels.Selector) ([]corev1.Pod, error) {
	results := api.MethodCall(api, "ListPods", namespace, selector)
	return results[0].([]corev1.Pod), nil
}

func (api *fakeK8sClusterMetadataChecker) ListStorageClasses(_ context.Context, selector k8slabels.Selector) ([]storagev1.StorageClass, error) {
	results := api.MethodCall(api, "ListStorageClasses", selector)
	return results[0].([]storagev1.StorageClass), nil
}
