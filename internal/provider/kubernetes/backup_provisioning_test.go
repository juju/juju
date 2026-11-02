// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"fmt"
	"net/http"
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

	"github.com/juju/juju/internal/docker/registry"
)

type backupProvisioningSuite struct{}

func TestBackupProvisioningSuite(t *testing.T) { tc.Run(t, &backupProvisioningSuite{}) }

type backupImageTransport func(*http.Request) (*http.Response, error)

func (f backupImageTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func backupSourceClient(agent, charm, agentID, charmID string) *fake.Clientset {
	class := "original-class"
	return fake.NewClientset(
		&apps.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "controller", Namespace: "controller-source"},
			Spec: apps.StatefulSetSpec{Template: core.PodTemplateSpec{Spec: core.PodSpec{Containers: []core.Container{
				{Name: apiServerContainerName, Image: "example.com/replacement:latest"},
				{Name: "charm", Image: "example.com/replacement-charm:latest"},
			}}}}},
		&core.Pod{ObjectMeta: metav1.ObjectMeta{Name: "controller-1", Namespace: "controller-source"},
			Spec: core.PodSpec{NodeName: "worker", Containers: []core.Container{
				{Name: apiServerContainerName, Image: agent}, {Name: "charm", Image: charm},
			}, Volumes: []core.Volume{{Name: storageName,
				VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "controller-storage"}},
			}}}, Status: core.PodStatus{ContainerStatuses: []core.ContainerStatus{
				{Name: apiServerContainerName, ImageID: agentID, State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
				{Name: "charm", ImageID: charmID, State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
			}}},
		&core.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker"}, Status: core.NodeStatus{NodeInfo: core.NodeSystemInfo{Architecture: "arm64"}}},
		&core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "controller-storage", Namespace: "controller-source"},
			Spec:   core.PersistentVolumeClaimSpec{StorageClassName: &class},
			Status: core.PersistentVolumeClaimStatus{Capacity: core.ResourceList{core.ResourceStorage: resource.MustParse("64Gi")}}},
		&core.Service{ObjectMeta: metav1.ObjectMeta{Name: "controller-service", Namespace: "controller-source", Annotations: map[string]string{"source": "retained"}},
			Spec: core.ServiceSpec{Type: core.ServiceTypeLoadBalancer, LoadBalancerIP: "192.0.2.1", ExternalIPs: []string{"192.0.2.2"}}},
	)
}

func (s *backupProvisioningSuite) TestRunningImagesAndActualVolume(c *tc.C) {
	originalTransport := registry.DefaultTransport
	c.Cleanup(func() { registry.DefaultTransport = originalTransport })
	registry.DefaultTransport = backupImageTransport(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected registry request")
	})
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, images := range []struct{ agent, charm, agentID, charmID string }{
		{"example.com/juju/agent:unpublished", "example.com/juju/charm:local", "", ""},
		{"example.com/juju/agent@" + digest, "example.com/juju/charm@" + digest, digest, "containerd://" + digest},
		{"example.com/juju/agent:unpublished", "example.com/juju/charm:local", "docker-pullable://example.com/other@" + digest, "runtime-specific"},
	} {
		client := backupSourceClient(images.agent, images.charm, images.agentID, images.charmID)
		broker := &kubernetesClient{clientUnlocked: client, namespace: "controller-source"}
		result, err := broker.ControllerBackupProvisioning(c.Context(), "1")
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result.AgentImage, tc.Equals, images.agent)
		c.Check(result.CharmImage, tc.Equals, images.charm)
		c.Check(result.Architecture, tc.Equals, "arm64")
		c.Check(result.StorageSizeMiB, tc.Equals, uint64(64*1024))
		c.Check(result.StorageClass, tc.Equals, "original-class")
		c.Check(result.ServiceType, tc.Equals, "loadbalancer")
		c.Check(result.LoadBalancerIP, tc.Equals, "192.0.2.1")
		c.Check(result.ExternalIPs, tc.DeepEquals, []string{"192.0.2.2"})
		c.Check(result.ServiceAnnotations["source"], tc.Equals, "retained")
	}
}

func (s *backupProvisioningSuite) TestInvalidSourceImages(c *tc.C) {
	for _, test := range []struct {
		name   string
		mutate func(*core.Pod)
	}{
		{"missing agent", func(p *core.Pod) { p.Spec.Containers = p.Spec.Containers[1:] }},
		{"missing charm", func(p *core.Pod) { p.Spec.Containers = p.Spec.Containers[:1] }},
		{"agent not running", func(p *core.Pod) { p.Status.ContainerStatuses[0].State.Running = nil }},
		{"charm not running", func(p *core.Pod) { p.Status.ContainerStatuses[1].State.Running = nil }},
		{"missing agent status", func(p *core.Pod) { p.Status.ContainerStatuses = p.Status.ContainerStatuses[1:] }},
		{"missing charm status", func(p *core.Pod) { p.Status.ContainerStatuses = p.Status.ContainerStatuses[:1] }},
		{"empty agent", func(p *core.Pod) { p.Spec.Containers[0].Image = "" }},
		{"empty charm", func(p *core.Pod) { p.Spec.Containers[1].Image = "" }},
		{"invalid agent", func(p *core.Pod) { p.Spec.Containers[0].Image = "invalid image" }},
		{"invalid charm", func(p *core.Pod) { p.Spec.Containers[1].Image = "invalid image" }},
	} {
		c.Logf("case: %s", test.name)
		client := backupSourceClient("example.com/agent:tag", "example.com/charm:tag", "", "")
		pod, err := client.CoreV1().Pods("controller-source").Get(c.Context(), "controller-1", metav1.GetOptions{})
		c.Assert(err, tc.ErrorIsNil)
		test.mutate(pod)
		_, err = client.CoreV1().Pods("controller-source").Update(c.Context(), pod, metav1.UpdateOptions{})
		c.Assert(err, tc.ErrorIsNil)
		broker := &kubernetesClient{clientUnlocked: client, namespace: "controller-source"}
		_, err = broker.ControllerBackupProvisioning(c.Context(), "1")
		c.Assert(err, tc.NotNil)
	}
}

func (s *backupProvisioningSuite) TestSourceErrors(c *tc.C) {
	client := backupSourceClient("example.com/agent:tag", "example.com/charm:tag", "", "")
	broker := &kubernetesClient{clientUnlocked: client, namespace: "controller-source"}
	_, err := broker.ControllerBackupProvisioning(c.Context(), "")
	c.Check(err, tc.ErrorMatches, "source controller ID is required")
	_, err = broker.ControllerBackupProvisioning(c.Context(), "0")
	c.Check(err, tc.ErrorMatches, ".*controller-0.*not found")
	for _, resource := range []string{"pods", "nodes", "persistentvolumeclaims", "services"} {
		client := backupSourceClient("example.com/agent:tag", "example.com/charm:tag", "", "")
		client.PrependReactor("get", resource, func(ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("failure reading %s", resource)
		})
		broker := &kubernetesClient{clientUnlocked: client, namespace: "controller-source"}
		_, err := broker.ControllerBackupProvisioning(c.Context(), "1")
		c.Check(err, tc.ErrorMatches, "failure reading "+resource)
	}
}
