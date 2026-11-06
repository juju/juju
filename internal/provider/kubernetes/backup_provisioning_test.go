// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"fmt"
	"strings"
	"testing"

	"github.com/juju/tc"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
)

type backupProvisioningSuite struct{}

func TestBackupProvisioningSuite(t *testing.T) { tc.Run(t, &backupProvisioningSuite{}) }

func backupSourceClient(agent, charm string) *fake.Clientset {
	class := "original-class"
	return fake.NewClientset(
		&apps.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: constants.JujuControllerStackName, Namespace: "controller-source"},
			Spec: apps.StatefulSetSpec{Template: core.PodTemplateSpec{Spec: core.PodSpec{Containers: []core.Container{
				{Name: apiServerContainerName, Image: "example.com/replacement:latest"},
				{Name: constants.ApplicationCharmContainer, Image: "example.com/replacement-charm:latest"},
			}}}}},
		&core.Pod{ObjectMeta: metav1.ObjectMeta{Name: getBootstrapResourceName(constants.JujuControllerStackName, "1"), Namespace: "controller-source"},
			Spec: core.PodSpec{NodeName: "worker", Containers: []core.Container{
				{Name: apiServerContainerName, Image: agent}, {Name: constants.ApplicationCharmContainer, Image: charm},
			}, Volumes: []core.Volume{{Name: storageName,
				VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "controller-storage"}},
			}}}, Status: core.PodStatus{ContainerStatuses: []core.ContainerStatus{
				{Name: apiServerContainerName, State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
				{Name: constants.ApplicationCharmContainer, State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
			}}},
		&core.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker"}, Status: core.NodeStatus{NodeInfo: core.NodeSystemInfo{Architecture: "arm64"}}},
		&core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "controller-storage", Namespace: "controller-source"},
			Spec:   core.PersistentVolumeClaimSpec{StorageClassName: &class},
			Status: core.PersistentVolumeClaimStatus{Capacity: core.ResourceList{core.ResourceStorage: resource.MustParse("64Gi")}}},
		&core.Service{ObjectMeta: metav1.ObjectMeta{Name: constants.ControllerServiceName, Namespace: "controller-source", Annotations: map[string]string{"source": "retained"}},
			Spec: core.ServiceSpec{Type: core.ServiceTypeLoadBalancer, LoadBalancerIP: "192.0.2.1", ExternalIPs: []string{"192.0.2.2"}}},
	)
}

func (s *backupProvisioningSuite) TestRunningImagesAndActualVolume(c *tc.C) {
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, images := range []struct{ agent, charm string }{
		{"example.com/juju/agent:unpublished", "example.com/juju/charm:local"},
		{"example.com/juju/agent@" + digest, "example.com/juju/charm@" + digest},
	} {
		client := backupSourceClient(images.agent, images.charm)
		broker := &kubernetesClient{clientUnlocked: client, namespace: "controller-source"}
		result, err := broker.ControllerBackupProvisioning(c.Context(), "1")
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result.AgentImage, tc.Equals, images.agent)
		c.Check(result.CharmImage, tc.Equals, images.charm)
		c.Check(result.Architecture, tc.Equals, "arm64")
		c.Check(result.StorageSizeMiB, tc.Equals, uint64(64*1024))
		c.Check(result.StorageClass, tc.Equals, "original-class")
		c.Check(result.ServiceType, tc.Equals, "loadbalancer")
		c.Check(result.ExternalName, tc.Equals, "")
		c.Check(result.LoadBalancerIP, tc.Equals, "192.0.2.1")
		c.Check(result.ExternalIPs, tc.DeepEquals, []string{"192.0.2.2"})
		c.Check(result.ServiceAnnotations["source"], tc.Equals, "retained")
	}
}

// TestCharmContainerNotRunning verifies that a restarting charm container
// does not prevent recording its source image.
func (s *backupProvisioningSuite) TestCharmContainerNotRunning(c *tc.C) {
	client := backupSourceClient("example.com/agent:tag", "example.com/charm:tag")
	pod := getBackupPod(c, client)
	pod.Status.ContainerStatuses = pod.Status.ContainerStatuses[:1]
	updateBackupPod(c, client, pod)

	broker := &kubernetesClient{clientUnlocked: client, namespace: "controller-source"}
	result, err := broker.ControllerBackupProvisioning(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.AgentImage, tc.Equals, "example.com/agent:tag")
	c.Check(result.CharmImage, tc.Equals, "example.com/charm:tag")
}

func (s *backupProvisioningSuite) TestInvalidSourceImages(c *tc.C) {
	for _, test := range []struct {
		name   string
		mutate func(*core.Pod)
	}{
		{"missing agent", func(p *core.Pod) { p.Spec.Containers = p.Spec.Containers[1:] }},
		{"missing charm", func(p *core.Pod) { p.Spec.Containers = p.Spec.Containers[:1] }},
		{"agent not running", func(p *core.Pod) { p.Status.ContainerStatuses[0].State.Running = nil }},
		{"missing agent status", func(p *core.Pod) { p.Status.ContainerStatuses = p.Status.ContainerStatuses[1:] }},
		{"empty agent", func(p *core.Pod) { p.Spec.Containers[0].Image = "" }},
		{"empty charm", func(p *core.Pod) { p.Spec.Containers[1].Image = "" }},
		{"invalid agent", func(p *core.Pod) { p.Spec.Containers[0].Image = "invalid image" }},
		{"invalid charm", func(p *core.Pod) { p.Spec.Containers[1].Image = "invalid image" }},
	} {
		c.Logf("case: %s", test.name)
		client := backupSourceClient("example.com/agent:tag", "example.com/charm:tag")
		pod, err := client.CoreV1().Pods("controller-source").Get(
			c.Context(), getBootstrapResourceName(constants.JujuControllerStackName, "1"), metav1.GetOptions{})
		c.Assert(err, tc.ErrorIsNil)
		test.mutate(pod)
		_, err = client.CoreV1().Pods("controller-source").Update(c.Context(), pod, metav1.UpdateOptions{})
		c.Assert(err, tc.ErrorIsNil)
		broker := &kubernetesClient{clientUnlocked: client, namespace: "controller-source"}
		_, err = broker.ControllerBackupProvisioning(c.Context(), "1")
		c.Check(err, tc.NotNil)
	}
}

func (s *backupProvisioningSuite) TestSourceErrors(c *tc.C) {
	client := backupSourceClient("example.com/agent:tag", "example.com/charm:tag")
	broker := &kubernetesClient{clientUnlocked: client, namespace: "controller-source"}
	_, err := broker.ControllerBackupProvisioning(c.Context(), "")
	c.Check(err, tc.ErrorMatches, "source controller ID is required")
	_, err = broker.ControllerBackupProvisioning(c.Context(), "0")
	c.Check(err, tc.ErrorMatches, ".*controller-0.*not found")
	for _, resource := range []string{"pods", "nodes", "persistentvolumeclaims", "services"} {
		client := backupSourceClient("example.com/agent:tag", "example.com/charm:tag")
		client.PrependReactor("get", resource, func(ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("failure reading %s", resource)
		})
		broker := &kubernetesClient{clientUnlocked: client, namespace: "controller-source"}
		_, err := broker.ControllerBackupProvisioning(c.Context(), "1")
		c.Check(err, tc.ErrorMatches, "failure reading "+resource)
	}
}

func (s *backupProvisioningSuite) TestStorageAndService(c *tc.C) {
	tests := []struct {
		name             string
		mutate           func(*tc.C, *fake.Clientset)
		wantStorageMiB   uint64
		wantServiceType  string
		wantExternalName string
		wantErr          string
	}{
		{
			name: "request capacity fallback rounds up",
			mutate: func(c *tc.C, client *fake.Clientset) {
				claim := getBackupClaim(c, client)
				claim.Status.Capacity = nil
				claim.Spec.Resources.Requests = core.ResourceList{
					core.ResourceStorage: resource.MustParse("1500Ki"),
				}
				updateBackupClaim(c, client, claim)
			},
			wantStorageMiB:  2,
			wantServiceType: string(caas.ServiceLoadBalancer),
		},
		{
			name: "missing storage class",
			mutate: func(c *tc.C, client *fake.Clientset) {
				claim := getBackupClaim(c, client)
				claim.Spec.StorageClassName = nil
				updateBackupClaim(c, client, claim)
			},
			wantErr: "controller volume has no storage class",
		},
		{
			name: "zero capacity",
			mutate: func(c *tc.C, client *fake.Clientset) {
				claim := getBackupClaim(c, client)
				claim.Status.Capacity = nil
				claim.Spec.Resources.Requests = nil
				updateBackupClaim(c, client, claim)
			},
			wantErr: "controller volume has no capacity",
		},
		{
			name: "missing storage volume",
			mutate: func(c *tc.C, client *fake.Clientset) {
				pod := getBackupPod(c, client)
				pod.Spec.Volumes = nil
				updateBackupPod(c, client, pod)
			},
			wantErr: "source controller data volume is missing",
		},
		{
			name: "missing PVC",
			mutate: func(c *tc.C, client *fake.Clientset) {
				err := client.CoreV1().PersistentVolumeClaims("controller-source").Delete(
					c.Context(), "controller-storage", metav1.DeleteOptions{})
				c.Assert(err, tc.ErrorIsNil)
			},
			wantErr: ".*controller-storage.*not found.*",
		},
		{
			name: "unscheduled pod",
			mutate: func(c *tc.C, client *fake.Clientset) {
				pod := getBackupPod(c, client)
				pod.Spec.NodeName = ""
				updateBackupPod(c, client, pod)
			},
			wantErr: ".*not found.*",
		},
		{
			name: "cluster IP service",
			mutate: func(c *tc.C, client *fake.Clientset) {
				service := getBackupService(c, client)
				service.Spec.Type = core.ServiceTypeClusterIP
				updateBackupService(c, client, service)
			},
			wantStorageMiB:  64 * 1024,
			wantServiceType: string(caas.ServiceCluster),
		},
		{
			name: "external name service",
			mutate: func(c *tc.C, client *fake.Clientset) {
				service := getBackupService(c, client)
				service.Spec.Type = core.ServiceTypeExternalName
				service.Spec.ExternalName = "example.com"
				updateBackupService(c, client, service)
			},
			wantStorageMiB:   64 * 1024,
			wantServiceType:  string(caas.ServiceExternal),
			wantExternalName: "example.com",
		},
		{
			name: "unsupported service type",
			mutate: func(c *tc.C, client *fake.Clientset) {
				service := getBackupService(c, client)
				service.Spec.Type = core.ServiceTypeNodePort
				updateBackupService(c, client, service)
			},
			wantErr: `unsupported source controller service type "NodePort"`,
		},
	}
	for _, test := range tests {
		c.Logf("case: %s", test.name)
		client := backupSourceClient("example.com/agent:tag", "example.com/charm:tag")
		test.mutate(c, client)
		broker := &kubernetesClient{clientUnlocked: client, namespace: "controller-source"}
		result, err := broker.ControllerBackupProvisioning(c.Context(), "1")
		if test.wantErr != "" {
			c.Check(err, tc.ErrorMatches, test.wantErr)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result.StorageSizeMiB, tc.Equals, test.wantStorageMiB)
		c.Check(result.ServiceType, tc.Equals, test.wantServiceType)
		c.Check(result.ExternalName, tc.Equals, test.wantExternalName)
	}
}

func getBackupPod(c *tc.C, client *fake.Clientset) *core.Pod {
	pod, err := client.CoreV1().Pods("controller-source").Get(
		c.Context(), getBootstrapResourceName(constants.JujuControllerStackName, "1"), metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	return pod
}

func updateBackupPod(c *tc.C, client *fake.Clientset, pod *core.Pod) {
	_, err := client.CoreV1().Pods("controller-source").Update(c.Context(), pod, metav1.UpdateOptions{})
	c.Assert(err, tc.ErrorIsNil)
}

func getBackupClaim(c *tc.C, client *fake.Clientset) *core.PersistentVolumeClaim {
	claim, err := client.CoreV1().PersistentVolumeClaims("controller-source").Get(
		c.Context(), "controller-storage", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	return claim
}

func updateBackupClaim(c *tc.C, client *fake.Clientset, claim *core.PersistentVolumeClaim) {
	_, err := client.CoreV1().PersistentVolumeClaims("controller-source").Update(
		c.Context(), claim, metav1.UpdateOptions{})
	c.Assert(err, tc.ErrorIsNil)
}

func getBackupService(c *tc.C, client *fake.Clientset) *core.Service {
	service, err := client.CoreV1().Services("controller-source").Get(
		c.Context(), constants.ControllerServiceName, metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	return service
}

func updateBackupService(c *tc.C, client *fake.Clientset, service *core.Service) {
	_, err := client.CoreV1().Services("controller-source").Update(
		c.Context(), service, metav1.UpdateOptions{})
	c.Assert(err, tc.ErrorIsNil)
}
