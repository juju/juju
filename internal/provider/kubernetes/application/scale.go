// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	"github.com/juju/juju/internal/provider/kubernetes/scale"
	"github.com/juju/juju/internal/storage"
)

// Scale scales the Application's unit to the value specificied. Scale must
// be >= 0. Application units will be removed or added to meet the scale
// defined.
func (a *app) Scale(scaleTo int) error {
	switch a.deploymentType {
	case caas.DeploymentStateful:
		return scale.PatchReplicasToScale(
			context.Background(),
			a.name,
			int32(scaleTo),
			scale.StatefulSetScalePatcher(a.client.AppsV1().StatefulSets(a.namespace)),
		)
	case caas.DeploymentStateless:
		return scale.PatchReplicasToScale(
			context.Background(),
			a.name,
			int32(scaleTo),
			scale.DeploymentScalePatcher(a.client.AppsV1().Deployments(a.namespace)),
		)
	default:
		return errors.NotSupportedf(
			"application %q deployment type %q cannot be scaled",
			a.name, a.deploymentType)
	}
}

// currentScale returns the current scale in use for the applications. i.e how
// many units is Kubernetes currently running for application x.
func (a *app) currentScale(ctx context.Context) (int, error) {
	switch a.deploymentType {
	case caas.DeploymentStateful:
		ss, err := a.client.AppsV1().StatefulSets(a.namespace).Get(ctx, a.name, meta.GetOptions{})
		if k8serrors.IsNotFound(err) {
			err = errors.WithType(err, errors.NotFound)
		}
		if err != nil {
			return 0, fmt.Errorf("fetching scale for application %q statefuleset: %w",
				a.name, err)
		}

		return int(*ss.Spec.Replicas), nil

	default:
		return 0, fmt.Errorf("application %q deployment type %q is not supported for fetching scale",
			a.name, a.deploymentType)
	}
}

// UnitsToRemove returns the names of units that need to be removed to reach the desired scale.
func (a *app) UnitsToRemove(ctx context.Context, desiredScale int) ([]string, error) {
	var unitsToRemove []string
	currentScale, err := a.currentScale(ctx)
	if err != nil {
		return unitsToRemove, err
	}

	numUnitsToRemove := desiredScale - currentScale
	if numUnitsToRemove >= 0 {
		return unitsToRemove, nil
	}

	for ; numUnitsToRemove != 0; numUnitsToRemove++ {
		unitsToRemove = append(unitsToRemove, fmt.Sprintf("%s/%d", a.name, currentScale+numUnitsToRemove))
	}

	return unitsToRemove, nil
}

// EnsurePVCs ensures that Persistent Volume Claims (PVCs) are created for the given
// filesystems and unit attachments. It creates PVCs based on the provided filesystem
// parameters and handles volume attachments for StatefulSet applications. Returns a
// cleanup function that can be used to delete the created PVCs if rollback is needed,
// and any error encountered during the process.
func (a *app) EnsurePVCs(
	filesystems []storage.KubernetesFilesystemParams,
	filesystemUnitAttachments map[string][]storage.KubernetesFilesystemUnitAttachmentParams,
	storageUniqueID string,
) error {
	applier := a.newApplier()

	pvcNames, err := a.pvcNames(storageUniqueID)
	if err != nil {
		return errors.Trace(err)
	}

	pvcNameGetter := a.pvcNameGetter(pvcNames, storageUniqueID)

	storageClasses, err := resources.ListStorageClass(context.Background(), a.client.StorageV1().StorageClasses(), meta.ListOptions{})
	if err != nil {
		return errors.Trace(err)
	}
	storageClassMap := make(map[string]resources.StorageClass)
	for _, v := range storageClasses {
		storageClassMap[v.Name] = v
	}

	for _, fs := range filesystems {
		name := a.volumeName(fs.StorageName)
		_, pvc, _, err := a.filesystemToVolumeInfo(name, fs, storageClassMap, pvcNameGetter)
		if err != nil {
			return errors.Trace(err)
		}
		if pvc != nil {
			err := a.handleVolumeAttachment(applier, filesystemUnitAttachments, *pvc, fs.StorageName)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	if err := applier.Run(context.Background(), false); err != nil {
		return errors.Trace(err)
	}
	return nil
}
