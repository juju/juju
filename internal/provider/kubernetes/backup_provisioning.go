// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/backups"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/errors"
)

// ControllerBackupProvisioning records the selected source pod's images and storage.
func (k *kubernetesClient) ControllerBackupProvisioning(ctx context.Context, controllerID string) (*backups.KubernetesProvisioning, error) {
	if controllerID == "" {
		return nil, errors.New("source controller ID is required")
	}
	pod, err := k.client().CoreV1().Pods(k.namespace).Get(ctx, "controller-"+controllerID, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	node, err := k.client().CoreV1().Nodes().Get(ctx, pod.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	result := &backups.KubernetesProvisioning{Architecture: node.Status.NodeInfo.Architecture}
	for _, container := range pod.Spec.Containers {
		if container.Name != apiServerContainerName && container.Name != "charm" {
			continue
		}
		running := false
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == container.Name && status.State.Running != nil {
				running = true
				break
			}
		}
		if !running {
			continue
		}
		if err := docker.ValidateDockerRegistryPath(container.Image); err != nil {
			return nil, errors.Errorf("source container %q: %w", container.Name, err)
		}
		if container.Name == apiServerContainerName {
			result.AgentImage = container.Image
		} else {
			result.CharmImage = container.Image
		}
	}
	if result.AgentImage == "" || result.CharmImage == "" || result.Architecture == "" {
		return nil, errors.New("source controller has no running agent, charm or platform identity")
	}
	for _, volume := range pod.Spec.Volumes {
		if volume.Name != storageName || volume.PersistentVolumeClaim == nil {
			continue
		}
		claim, err := k.client().CoreV1().PersistentVolumeClaims(k.namespace).Get(ctx, volume.PersistentVolumeClaim.ClaimName, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		if claim.Spec.StorageClassName == nil {
			return nil, errors.New("controller volume has no storage class")
		}
		result.StorageClass = *claim.Spec.StorageClassName
		capacity := claim.Status.Capacity[core.ResourceStorage]
		if capacity.IsZero() {
			capacity = claim.Spec.Resources.Requests[core.ResourceStorage]
		}
		if capacity.Sign() <= 0 {
			return nil, errors.New("controller volume has no capacity")
		}
		result.StorageSizeMiB = uint64((capacity.Value() + (1 << 20) - 1) >> 20)
	}
	if result.StorageSizeMiB == 0 {
		return nil, errors.New("source controller data volume is missing")
	}
	service, err := k.client().CoreV1().Services(k.namespace).Get(ctx, "controller-service", metav1.GetOptions{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	switch service.Spec.Type {
	case core.ServiceTypeClusterIP:
		result.ServiceType = string(caas.ServiceCluster)
	case core.ServiceTypeLoadBalancer:
		result.ServiceType = string(caas.ServiceLoadBalancer)
	case core.ServiceTypeExternalName:
		result.ServiceType = string(caas.ServiceExternal)
	default:
		return nil, errors.Errorf("unsupported source controller service type %q", service.Spec.Type)
	}
	result.ExternalName, result.ExternalIPs = service.Spec.ExternalName, service.Spec.ExternalIPs
	result.LoadBalancerIP, result.ServiceAnnotations = service.Spec.LoadBalancerIP, service.Annotations
	return result, nil
}
