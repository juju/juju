// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"encoding/base64"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	apps "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/testing"
)

func (s *K8sBrokerSuite) assertMutatingWebhookConfigurations(c *gc.C, cfgs []k8sspecs.K8sMutatingWebhook, assertCalls ...any) {

	basicPodSpec := getBasicPodspec()
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			MutatingWebhookConfigurations: cfgs,
		},
	}
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, resources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec

	numUnits := int32(2)
	statefulSetArg := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"app.juju.is/uuid":               "appuuid",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"charm.juju.is/modified-version": "0",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
			PodManagementPolicy: apps.ParallelPodManagement,
			ServiceName:         "app-name-endpoints",
		},
	}

	serviceArg := *basicServiceArg
	serviceArg.Spec.Type = core.ServiceTypeClusterIP

	assertCalls = append(
		[]any{
			s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", metav1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
		},
		assertCalls...,
	)

	ociImageSecret := s.getOCIImageSecret(c, nil)
	assertCalls = append(assertCalls, []any{
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, metav1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", metav1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), &serviceArg, metav1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), &serviceArg, metav1.CreateOptions{}).
			Return(nil, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name-endpoints", metav1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicHeadlessServiceArg, metav1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicHeadlessServiceArg, metav1.CreateOptions{}).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", metav1.GetOptions{}).
			Return(statefulSetArg, nil),
		s.mockStatefulSets.EXPECT().Create(gomock.Any(), statefulSetArg, metav1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", metav1.GetOptions{}).
			Return(statefulSetArg, nil),
		s.mockStatefulSets.EXPECT().Update(gomock.Any(), statefulSetArg, metav1.UpdateOptions{}).
			Return(nil, nil),
	}...)
	gomock.InOrder(assertCalls...)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateful,
		},
		ImageDetails: resources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{"juju-controller-uuid": testing.ControllerTag.Id()},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, e string, _ map[string]interface{}) error {
		c.Logf("EnsureService error -> %q", e)
		return nil
	}, params, 2, config.ConfigAttributes{
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureMutatingWebhookConfigurationsCreateV1Beta1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "15",
	}, nil)

	webhook1Rule1 := admissionregistrationv1beta1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1beta1.RuleWithOperations{
		Operations: []admissionregistrationv1beta1.OperationType{
			admissionregistrationv1beta1.Create,
			admissionregistrationv1beta1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1beta1.Ignore
	webhook1 := admissionregistrationv1beta1.MutatingWebhook{
		Name:          "example.mutatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
			Service: &admissionregistrationv1beta1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1beta1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sMutatingWebhook{
		{
			Meta: k8sspecs.Meta{Name: "example-mutatingwebhookconfiguration"},
			Webhooks: []k8sspecs.K8sMutatingWebhookSpec{
				{
					Version:     k8sspecs.K8sWebhookV1Beta1,
					SpecV1Beta1: webhook1,
				},
			},
		},
	}

	cfg1 := &admissionregistrationv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-example-mutatingwebhookconfiguration",
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id()},
		},
		Webhooks: []admissionregistrationv1beta1.MutatingWebhook{webhook1},
	}

	s.assertMutatingWebhookConfigurations(
		c, cfgs,
		s.mockMutatingWebhookConfigurationV1Beta1.EXPECT().Create(gomock.Any(), cfg1, metav1.CreateOptions{}).Return(cfg1, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureMutatingWebhookConfigurationsCreateV1Beta1Upgrade(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "16",
	}, nil)

	webhook1Rule1 := admissionregistrationv1beta1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1beta1.RuleWithOperations{
		Operations: []admissionregistrationv1beta1.OperationType{
			admissionregistrationv1beta1.Create,
			admissionregistrationv1beta1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1beta1.Ignore
	webhook1 := admissionregistrationv1beta1.MutatingWebhook{
		Name:          "example.mutatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
			Service: &admissionregistrationv1beta1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1beta1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sMutatingWebhook{
		{
			Meta: k8sspecs.Meta{Name: "example-mutatingwebhookconfiguration"},
			Webhooks: []k8sspecs.K8sMutatingWebhookSpec{
				{
					Version:     k8sspecs.K8sWebhookV1Beta1,
					SpecV1Beta1: webhook1,
				},
			},
		},
	}

	cfg2 := func() *admissionregistrationv1.MutatingWebhookConfiguration {
		webhook2Rule1 := admissionregistrationv1.Rule{
			APIGroups:   []string{""},
			APIVersions: []string{"v1"},
			Resources:   []string{"pods"},
		}
		webhookRuleWithOperations2 := admissionregistrationv1.RuleWithOperations{
			Operations: []admissionregistrationv1.OperationType{
				admissionregistrationv1.Create,
				admissionregistrationv1.Update,
			},
		}
		webhookRuleWithOperations2.Rule = webhook2Rule1
		CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
		c.Assert(err, jc.ErrorIsNil)
		webhook2FailurePolicy := admissionregistrationv1.Ignore
		sideEffects := admissionregistrationv1.SideEffectClassNoneOnDryRun
		webhook2 := admissionregistrationv1.MutatingWebhook{
			Name:          "example.mutatingwebhookconfiguration.com",
			FailurePolicy: &webhook2FailurePolicy,
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: &admissionregistrationv1.ServiceReference{
					Name:      "apple-service",
					Namespace: "apples",
					Path:      pointer.StringPtr("/apple"),
				},
				CABundle: CABundle,
			},
			NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
				},
			},
			Rules:                   []admissionregistrationv1.RuleWithOperations{webhookRuleWithOperations2},
			AdmissionReviewVersions: []string{"v1beta1"},
			SideEffects:             &sideEffects,
		}

		return &admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-example-mutatingwebhookconfiguration",
				Namespace:   "test",
				Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
				Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id()},
			},
			Webhooks: []admissionregistrationv1.MutatingWebhook{webhook2},
		}
	}()

	s.assertMutatingWebhookConfigurations(
		c, cfgs,
		s.mockMutatingWebhookConfigurationV1.EXPECT().Create(gomock.Any(), cfg2, gomock.Any()).Return(cfg2, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureMutatingWebhookConfigurationsCreateKeepNameV1Beta1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "15",
	}, nil)

	webhook1Rule1 := admissionregistrationv1beta1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1beta1.RuleWithOperations{
		Operations: []admissionregistrationv1beta1.OperationType{
			admissionregistrationv1beta1.Create,
			admissionregistrationv1beta1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1beta1.Ignore
	webhook1 := admissionregistrationv1beta1.MutatingWebhook{
		Name:          "example.mutatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
			Service: &admissionregistrationv1beta1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1beta1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sMutatingWebhook{
		{
			Meta: k8sspecs.Meta{
				Name:        "example-mutatingwebhookconfiguration",
				Annotations: map[string]string{"model.juju.is/disable-prefix": "true"},
			},
			Webhooks: []k8sspecs.K8sMutatingWebhookSpec{
				{
					Version:     k8sspecs.K8sWebhookV1Beta1,
					SpecV1Beta1: webhook1,
				},
			},
		},
	}

	cfg1 := &admissionregistrationv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "example-mutatingwebhookconfiguration", // This name kept no change.
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id(), "model.juju.is/disable-prefix": "true"},
		},
		Webhooks: []admissionregistrationv1beta1.MutatingWebhook{webhook1},
	}

	s.assertMutatingWebhookConfigurations(
		c, cfgs,
		s.mockMutatingWebhookConfigurationV1Beta1.EXPECT().Create(gomock.Any(), cfg1, gomock.Any()).Return(cfg1, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureMutatingWebhookConfigurationsUpdateV1Beta1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "15",
	}, nil)

	webhook1Rule1 := admissionregistrationv1beta1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1beta1.RuleWithOperations{
		Operations: []admissionregistrationv1beta1.OperationType{
			admissionregistrationv1beta1.Create,
			admissionregistrationv1beta1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1beta1.Ignore
	webhook1 := admissionregistrationv1beta1.MutatingWebhook{
		Name:          "example.mutatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
			Service: &admissionregistrationv1beta1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1beta1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sMutatingWebhook{
		{
			Meta: k8sspecs.Meta{Name: "example-mutatingwebhookconfiguration"},
			Webhooks: []k8sspecs.K8sMutatingWebhookSpec{
				{
					Version:     k8sspecs.K8sWebhookV1Beta1,
					SpecV1Beta1: webhook1,
				},
			},
		},
	}

	cfg1 := &admissionregistrationv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-example-mutatingwebhookconfiguration",
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id()},
		},
		Webhooks: []admissionregistrationv1beta1.MutatingWebhook{webhook1},
	}

	s.assertMutatingWebhookConfigurations(
		c, cfgs,
		s.mockMutatingWebhookConfigurationV1Beta1.EXPECT().Create(gomock.Any(), cfg1, gomock.Any()).Return(cfg1, s.k8sAlreadyExistsError()),
		s.mockMutatingWebhookConfigurationV1Beta1.EXPECT().
			List(gomock.Any(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=app-name,model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).
			Return(&admissionregistrationv1beta1.MutatingWebhookConfigurationList{Items: []admissionregistrationv1beta1.MutatingWebhookConfiguration{*cfg1}}, nil),
		s.mockMutatingWebhookConfigurationV1Beta1.EXPECT().
			Get(gomock.Any(), "test-example-mutatingwebhookconfiguration", metav1.GetOptions{}).
			Return(cfg1, nil),
		s.mockMutatingWebhookConfigurationV1Beta1.EXPECT().Update(gomock.Any(), cfg1, gomock.Any()).Return(cfg1, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureMutatingWebhookConfigurationsCreateV1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "16",
	}, nil)

	webhook1Rule1 := admissionregistrationv1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1.RuleWithOperations{
		Operations: []admissionregistrationv1.OperationType{
			admissionregistrationv1.Create,
			admissionregistrationv1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1.Ignore
	webhook1 := admissionregistrationv1.MutatingWebhook{
		Name:          "example.mutatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1.WebhookClientConfig{
			Service: &admissionregistrationv1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sMutatingWebhook{
		{
			Meta: k8sspecs.Meta{Name: "example-mutatingwebhookconfiguration"},
			Webhooks: []k8sspecs.K8sMutatingWebhookSpec{
				{
					Version: k8sspecs.K8sWebhookV1,
					SpecV1:  webhook1,
				},
			},
		},
	}

	cfg1 := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-example-mutatingwebhookconfiguration",
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id()},
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{webhook1},
	}

	s.assertMutatingWebhookConfigurations(
		c, cfgs,
		s.mockMutatingWebhookConfigurationV1.EXPECT().Create(gomock.Any(), cfg1, gomock.Any()).Return(cfg1, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureMutatingWebhookConfigurationsCreateKeepNameV1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "16",
	}, nil)

	webhook1Rule1 := admissionregistrationv1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1.RuleWithOperations{
		Operations: []admissionregistrationv1.OperationType{
			admissionregistrationv1.Create,
			admissionregistrationv1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1.Ignore
	webhook1 := admissionregistrationv1.MutatingWebhook{
		Name:          "example.mutatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1.WebhookClientConfig{
			Service: &admissionregistrationv1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sMutatingWebhook{
		{
			Meta: k8sspecs.Meta{
				Name:        "example-mutatingwebhookconfiguration",
				Annotations: map[string]string{"model.juju.is/disable-prefix": "true"},
			},
			Webhooks: []k8sspecs.K8sMutatingWebhookSpec{
				{
					Version: k8sspecs.K8sWebhookV1,
					SpecV1:  webhook1,
				},
			},
		},
	}

	cfg1 := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "example-mutatingwebhookconfiguration", // This name kept no change.
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id(), "model.juju.is/disable-prefix": "true"},
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{webhook1},
	}

	s.assertMutatingWebhookConfigurations(
		c, cfgs,
		s.mockMutatingWebhookConfigurationV1.EXPECT().Create(gomock.Any(), cfg1, gomock.Any()).Return(cfg1, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureMutatingWebhookConfigurationsUpdateV1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "16",
	}, nil)

	webhook1Rule1 := admissionregistrationv1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1.RuleWithOperations{
		Operations: []admissionregistrationv1.OperationType{
			admissionregistrationv1.Create,
			admissionregistrationv1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1.Ignore
	webhook1 := admissionregistrationv1.MutatingWebhook{
		Name:          "example.mutatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1.WebhookClientConfig{
			Service: &admissionregistrationv1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sMutatingWebhook{
		{
			Meta: k8sspecs.Meta{Name: "example-mutatingwebhookconfiguration"},
			Webhooks: []k8sspecs.K8sMutatingWebhookSpec{
				{
					Version: k8sspecs.K8sWebhookV1,
					SpecV1:  webhook1,
				},
			},
		},
	}

	cfg1 := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-example-mutatingwebhookconfiguration",
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id()},
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{webhook1},
	}

	s.assertMutatingWebhookConfigurations(
		c, cfgs,
		s.mockMutatingWebhookConfigurationV1.EXPECT().Create(gomock.Any(), cfg1, metav1.CreateOptions{}).Return(cfg1, s.k8sAlreadyExistsError()),
		s.mockMutatingWebhookConfigurationV1.EXPECT().
			Get(gomock.Any(), "test-example-mutatingwebhookconfiguration", metav1.GetOptions{}).
			Return(cfg1, nil),
		s.mockMutatingWebhookConfigurationV1.EXPECT().Update(gomock.Any(), cfg1, metav1.UpdateOptions{}).Return(cfg1, nil),
	)
}

func (s *K8sBrokerSuite) assertValidatingWebhookConfigurations(c *gc.C, cfgs []k8sspecs.K8sValidatingWebhook, assertCalls ...any) {

	basicPodSpec := getBasicPodspec()
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			ValidatingWebhookConfigurations: cfgs,
		},
	}
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, resources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec

	numUnits := int32(2)
	statefulSetArg := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"app.juju.is/uuid":               "appuuid",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"charm.juju.is/modified-version": "0",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
			PodManagementPolicy: apps.ParallelPodManagement,
			ServiceName:         "app-name-endpoints",
		},
	}

	serviceArg := *basicServiceArg
	serviceArg.Spec.Type = core.ServiceTypeClusterIP

	assertCalls = append(
		[]any{
			s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", metav1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
		},
		assertCalls...,
	)

	ociImageSecret := s.getOCIImageSecret(c, nil)
	assertCalls = append(assertCalls, []any{
		s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, metav1.CreateOptions{}).
			Return(ociImageSecret, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name", metav1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), &serviceArg, metav1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), &serviceArg, metav1.CreateOptions{}).
			Return(nil, nil),
		s.mockServices.EXPECT().Get(gomock.Any(), "app-name-endpoints", metav1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(gomock.Any(), basicHeadlessServiceArg, metav1.UpdateOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(gomock.Any(), basicHeadlessServiceArg, metav1.CreateOptions{}).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", metav1.GetOptions{}).
			Return(statefulSetArg, nil),
		s.mockStatefulSets.EXPECT().Create(gomock.Any(), statefulSetArg, metav1.CreateOptions{}).
			Return(nil, nil),
	}...)
	gomock.InOrder(assertCalls...)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateful,
		},
		ImageDetails: resources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{"juju-controller-uuid": testing.ControllerTag.Id()},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, e string, _ map[string]interface{}) error {
		c.Logf("EnsureService error -> %q", e)
		return nil
	}, params, 2, config.ConfigAttributes{
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureValidatingWebhookConfigurationsCreateV1Beta1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "15",
	}, nil)

	webhook1Rule1 := admissionregistrationv1beta1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1beta1.RuleWithOperations{
		Operations: []admissionregistrationv1beta1.OperationType{
			admissionregistrationv1beta1.Create,
			admissionregistrationv1beta1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1beta1.Ignore
	webhook1 := admissionregistrationv1beta1.ValidatingWebhook{
		Name:          "example.validatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
			Service: &admissionregistrationv1beta1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1beta1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sValidatingWebhook{
		{
			Meta: k8sspecs.Meta{Name: "example-validatingwebhookconfiguration"},
			Webhooks: []k8sspecs.K8sValidatingWebhookSpec{
				{
					Version:     k8sspecs.K8sWebhookV1Beta1,
					SpecV1Beta1: webhook1,
				},
			},
		},
	}

	cfg1 := &admissionregistrationv1beta1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-example-validatingwebhookconfiguration",
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id()},
		},
		Webhooks: []admissionregistrationv1beta1.ValidatingWebhook{webhook1},
	}

	s.assertValidatingWebhookConfigurations(
		c, cfgs,
		s.mockValidatingWebhookConfigurationV1Beta1.EXPECT().Create(gomock.Any(), cfg1, metav1.CreateOptions{}).Return(cfg1, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureValidatingWebhookConfigurationsCreateV1Beta1Upgrade(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "16",
	}, nil)

	webhook1Rule1 := admissionregistrationv1beta1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1beta1.RuleWithOperations{
		Operations: []admissionregistrationv1beta1.OperationType{
			admissionregistrationv1beta1.Create,
			admissionregistrationv1beta1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1beta1.Ignore
	webhook1 := admissionregistrationv1beta1.ValidatingWebhook{
		Name:          "example.validatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
			Service: &admissionregistrationv1beta1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1beta1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sValidatingWebhook{
		{
			Meta: k8sspecs.Meta{Name: "example-validatingwebhookconfiguration"},
			Webhooks: []k8sspecs.K8sValidatingWebhookSpec{
				{
					Version:     k8sspecs.K8sWebhookV1Beta1,
					SpecV1Beta1: webhook1,
				},
			},
		},
	}

	webhook2 := func() admissionregistrationv1.ValidatingWebhook {
		webhook2Rule1 := admissionregistrationv1.Rule{
			APIGroups:   []string{""},
			APIVersions: []string{"v1"},
			Resources:   []string{"pods"},
		}
		webhookRuleWithOperations2 := admissionregistrationv1.RuleWithOperations{
			Operations: []admissionregistrationv1.OperationType{
				admissionregistrationv1.Create,
				admissionregistrationv1.Update,
			},
		}
		webhookRuleWithOperations2.Rule = webhook2Rule1
		CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
		c.Assert(err, jc.ErrorIsNil)
		webhook2FailurePolicy := admissionregistrationv1.Ignore
		return admissionregistrationv1.ValidatingWebhook{
			Name:          "example.validatingwebhookconfiguration.com",
			FailurePolicy: &webhook2FailurePolicy,
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: &admissionregistrationv1.ServiceReference{
					Name:      "apple-service",
					Namespace: "apples",
					Path:      pointer.StringPtr("/apple"),
				},
				CABundle: CABundle,
			},
			NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
				},
			},
			Rules:                   []admissionregistrationv1.RuleWithOperations{webhookRuleWithOperations2},
			AdmissionReviewVersions: []string{"v1beta1"},
		}
	}()

	cfg2 := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-example-validatingwebhookconfiguration",
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id()},
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{webhook2},
	}

	s.assertValidatingWebhookConfigurations(
		c, cfgs,
		s.mockValidatingWebhookConfigurationV1.EXPECT().Create(gomock.Any(), cfg2, gomock.Any()).Return(cfg2, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureValidatingWebhookConfigurationsCreateKeepNameV1Beta1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "15",
	}, nil)

	webhook1Rule1 := admissionregistrationv1beta1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1beta1.RuleWithOperations{
		Operations: []admissionregistrationv1beta1.OperationType{
			admissionregistrationv1beta1.Create,
			admissionregistrationv1beta1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1beta1.Ignore
	webhook1 := admissionregistrationv1beta1.ValidatingWebhook{
		Name:          "example.validatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
			Service: &admissionregistrationv1beta1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1beta1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sValidatingWebhook{
		{
			Meta: k8sspecs.Meta{
				Name:        "example-validatingwebhookconfiguration",
				Annotations: map[string]string{"model.juju.is/disable-prefix": "true"},
			},
			Webhooks: []k8sspecs.K8sValidatingWebhookSpec{
				{
					Version:     k8sspecs.K8sWebhookV1Beta1,
					SpecV1Beta1: webhook1,
				},
			},
		},
	}

	cfg1 := &admissionregistrationv1beta1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "example-validatingwebhookconfiguration", // This name kept no change.
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id(), "model.juju.is/disable-prefix": "true"},
		},
		Webhooks: []admissionregistrationv1beta1.ValidatingWebhook{webhook1},
	}

	s.assertValidatingWebhookConfigurations(
		c, cfgs,
		s.mockValidatingWebhookConfigurationV1Beta1.EXPECT().Create(gomock.Any(), cfg1, gomock.Any()).Return(cfg1, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureValidatingWebhookConfigurationsUpdateV1Beta1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "15",
	}, nil)

	webhook1Rule1 := admissionregistrationv1beta1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1beta1.RuleWithOperations{
		Operations: []admissionregistrationv1beta1.OperationType{
			admissionregistrationv1beta1.Create,
			admissionregistrationv1beta1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1beta1.Ignore
	webhook1 := admissionregistrationv1beta1.ValidatingWebhook{
		Name:          "example.validatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
			Service: &admissionregistrationv1beta1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1beta1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sValidatingWebhook{
		{
			Meta: k8sspecs.Meta{Name: "example-validatingwebhookconfiguration"},
			Webhooks: []k8sspecs.K8sValidatingWebhookSpec{
				{
					Version:     k8sspecs.K8sWebhookV1Beta1,
					SpecV1Beta1: webhook1,
				},
			},
		},
	}

	cfg1 := &admissionregistrationv1beta1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-example-validatingwebhookconfiguration",
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id()},
		},
		Webhooks: []admissionregistrationv1beta1.ValidatingWebhook{webhook1},
	}

	s.assertValidatingWebhookConfigurations(
		c, cfgs,
		s.mockValidatingWebhookConfigurationV1Beta1.EXPECT().Create(gomock.Any(), cfg1, gomock.Any()).Return(cfg1, s.k8sAlreadyExistsError()),
		s.mockValidatingWebhookConfigurationV1Beta1.EXPECT().
			List(gomock.Any(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=app-name,model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).
			Return(&admissionregistrationv1beta1.ValidatingWebhookConfigurationList{Items: []admissionregistrationv1beta1.ValidatingWebhookConfiguration{*cfg1}}, nil),
		s.mockValidatingWebhookConfigurationV1Beta1.EXPECT().
			Get(gomock.Any(), "test-example-validatingwebhookconfiguration", metav1.GetOptions{}).
			Return(cfg1, nil),
		s.mockValidatingWebhookConfigurationV1Beta1.EXPECT().Update(gomock.Any(), cfg1, gomock.Any()).Return(cfg1, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureValidatingWebhookConfigurationsCreateV1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "16",
	}, nil)

	webhook1Rule1 := admissionregistrationv1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1.RuleWithOperations{
		Operations: []admissionregistrationv1.OperationType{
			admissionregistrationv1.Create,
			admissionregistrationv1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1.Ignore
	webhook1 := admissionregistrationv1.ValidatingWebhook{
		Name:          "example.validatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1.WebhookClientConfig{
			Service: &admissionregistrationv1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sValidatingWebhook{
		{
			Meta: k8sspecs.Meta{Name: "example-validatingwebhookconfiguration"},
			Webhooks: []k8sspecs.K8sValidatingWebhookSpec{
				{
					Version: k8sspecs.K8sWebhookV1,
					SpecV1:  webhook1,
				},
			},
		},
	}

	cfg1 := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-example-validatingwebhookconfiguration",
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id()},
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{webhook1},
	}

	s.assertValidatingWebhookConfigurations(
		c, cfgs,
		s.mockValidatingWebhookConfigurationV1.EXPECT().Create(gomock.Any(), cfg1, gomock.Any()).Return(cfg1, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureValidatingWebhookConfigurationsCreateKeepNameV1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "16",
	}, nil)

	webhook1Rule1 := admissionregistrationv1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1.RuleWithOperations{
		Operations: []admissionregistrationv1.OperationType{
			admissionregistrationv1.Create,
			admissionregistrationv1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1.Ignore
	webhook1 := admissionregistrationv1.ValidatingWebhook{
		Name:          "example.validatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1.WebhookClientConfig{
			Service: &admissionregistrationv1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sValidatingWebhook{
		{
			Meta: k8sspecs.Meta{
				Name:        "example-validatingwebhookconfiguration",
				Annotations: map[string]string{"model.juju.is/disable-prefix": "true"},
			},
			Webhooks: []k8sspecs.K8sValidatingWebhookSpec{
				{
					Version: k8sspecs.K8sWebhookV1,
					SpecV1:  webhook1,
				},
			},
		},
	}

	cfg1 := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "example-validatingwebhookconfiguration", // This name kept no change.
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id(), "model.juju.is/disable-prefix": "true"},
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{webhook1},
	}

	s.assertValidatingWebhookConfigurations(
		c, cfgs,
		s.mockValidatingWebhookConfigurationV1.EXPECT().Create(gomock.Any(), cfg1, gomock.Any()).Return(cfg1, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureValidatingWebhookConfigurationsUpdateV1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().AnyTimes().Return(&k8sversion.Info{
		Major: "1", Minor: "16",
	}, nil)

	webhook1Rule1 := admissionregistrationv1.Rule{
		APIGroups:   []string{""},
		APIVersions: []string{"v1"},
		Resources:   []string{"pods"},
	}
	webhookRuleWithOperations1 := admissionregistrationv1.RuleWithOperations{
		Operations: []admissionregistrationv1.OperationType{
			admissionregistrationv1.Create,
			admissionregistrationv1.Update,
		},
	}
	webhookRuleWithOperations1.Rule = webhook1Rule1
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	webhook1FailurePolicy := admissionregistrationv1.Ignore
	webhook1 := admissionregistrationv1.ValidatingWebhook{
		Name:          "example.validatingwebhookconfiguration.com",
		FailurePolicy: &webhook1FailurePolicy,
		ClientConfig: admissionregistrationv1.WebhookClientConfig{
			Service: &admissionregistrationv1.ServiceReference{
				Name:      "apple-service",
				Namespace: "apples",
				Path:      pointer.StringPtr("/apple"),
			},
			CABundle: CABundle,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
			},
		},
		Rules: []admissionregistrationv1.RuleWithOperations{webhookRuleWithOperations1},
	}

	cfgs := []k8sspecs.K8sValidatingWebhook{
		{
			Meta: k8sspecs.Meta{Name: "example-validatingwebhookconfiguration"},
			Webhooks: []k8sspecs.K8sValidatingWebhookSpec{
				{
					Version: k8sspecs.K8sWebhookV1,
					SpecV1:  webhook1,
				},
			},
		},
	}

	cfg1 := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-example-validatingwebhookconfiguration",
			Namespace:   "test",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/name": "test", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			Annotations: map[string]string{"controller.juju.is/id": testing.ControllerTag.Id()},
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{webhook1},
	}

	s.assertValidatingWebhookConfigurations(
		c, cfgs,
		s.mockValidatingWebhookConfigurationV1.EXPECT().Create(gomock.Any(), cfg1, metav1.CreateOptions{}).Return(cfg1, s.k8sAlreadyExistsError()),
		s.mockValidatingWebhookConfigurationV1.EXPECT().
			List(gomock.Any(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=app-name,model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).
			Return(&admissionregistrationv1.ValidatingWebhookConfigurationList{Items: []admissionregistrationv1.ValidatingWebhookConfiguration{*cfg1}}, nil),
		s.mockValidatingWebhookConfigurationV1.EXPECT().
			Get(gomock.Any(), "test-example-validatingwebhookconfiguration", metav1.GetOptions{}).
			Return(cfg1, nil),
		s.mockValidatingWebhookConfigurationV1.EXPECT().Update(gomock.Any(), cfg1, metav1.UpdateOptions{}).Return(cfg1, nil),
	)
}
