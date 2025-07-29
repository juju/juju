// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	"github.com/juju/juju/caas/kubernetes/provider/scale"
	"github.com/juju/juju/storage"
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

func (a *app) EnsurePVC(
	filesystems []storage.KubernetesFilesystemParams,
	filesystemUnitAttachments map[string][]storage.KubernetesFilesystemUnitAttachmentParams,
) (func() error, error) {
	applier := a.newApplier()

	ss, getErr := a.getStatefulSet()
	if errors.Is(getErr, errors.NotFound) {
		// skip if statefulset not exists.
		return nil, nil
	} else if getErr != nil {
		return nil, errors.Trace(getErr)
	}

	storageUniqueID, err := a.getStorageUniqPrefix(func() (annotationGetter, error) {
		return ss, getErr
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	pvcNames, err := a.pvcNames(storageUniqueID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	pvcNameGetter := func(volName string) string {
		if n, ok := pvcNames[volName]; ok {
			logger.Debugf("using existing persistent volume claim %q (volume %q)", n, volName)
			return n
		}
		return fmt.Sprintf("%s-%s", volName, storageUniqueID)
	}
	storageClasses, err := resources.ListStorageClass(context.Background(), a.client, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageClassMap := make(map[string]resources.StorageClass)
	for _, v := range storageClasses {
		storageClassMap[v.Name] = v
	}

	pvcResources := []resources.PersistentVolumeClaim{}
	for _, fs := range filesystems {
		name := a.volumeName(fs.StorageName)
		_, pvc, _, err := a.filesystemToVolumeInfo(name, fs, storageClassMap, pvcNameGetter)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if pvc != nil {
			applyedPVCs, err := a.handleVolumeAttachment(applier, filesystemUnitAttachments, *pvc, fs.StorageName)
			if err != nil {
				return nil, err
			}
			pvcResources = append(pvcResources, applyedPVCs...)
		}
	}
	if err := applier.Run(context.Background(), a.client, false); err != nil {
		return nil, errors.Trace(err)
	}
	// CleanUp function delete applyed PVC if need roll back.
	cleanUpFunc := func() error {
		for _, pvcResource := range pvcResources {
			applier.Delete(&pvcResource)
		}
		if err := applier.Run(context.Background(), a.client, false); err != nil {
			return errors.Trace(err)
		}
		return nil
	}
	return cleanUpFunc, nil
}
