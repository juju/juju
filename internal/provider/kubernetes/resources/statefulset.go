// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	v1 "k8s.io/client-go/kubernetes/typed/apps/v1"

	"github.com/juju/juju/core/status"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

// StatefulSet extends the k8s statefulSet.
type StatefulSet struct {
	client v1.StatefulSetInterface
	appsv1.StatefulSet
}

// StatefulSetWithOrphanDelete is a wrapper around [StatefulSet] that overrides
// the [StatefulSet.Delete] method to use DeletePropagationOrphan.
type StatefulSetWithOrphanDelete struct {
	*StatefulSet
	interval time.Duration
	timeout  time.Duration
}

// NewStatefulSet creates a new statefulset resource.
func NewStatefulSet(client v1.StatefulSetInterface, namespace string, name string, in *appsv1.StatefulSet) *StatefulSet {
	if in == nil {
		in = &appsv1.StatefulSet{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &StatefulSet{client, *in}
}

func NewStatefulSetWithOrphanDelete(ss *StatefulSet) *StatefulSetWithOrphanDelete {
	return &StatefulSetWithOrphanDelete{StatefulSet: ss, interval: 1 * time.Second, timeout: 30 * time.Second}
}

// Clone returns a copy of the resource.
func (ss *StatefulSet) Clone() Resource {
	clone := *ss
	return &clone
}

// ID returns a comparable ID for the Resource.
func (ss *StatefulSet) ID() ID {
	return ID{"StatefulSet", ss.Name, ss.Namespace}
}

// Apply patches the resource change.
func (ss *StatefulSet) Apply(ctx context.Context) (err error) {
	logger.Infof("[adis][StatefulSet][Apply] sts name: %q", ss.Name)
	var result *appsv1.StatefulSet
	defer func() {
		if result != nil {
			ss.StatefulSet = *result
		}
	}()

	existing, err := ss.client.Get(ctx, ss.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		result, err = ss.client.Create(ctx, &ss.StatefulSet, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
		logger.Infof("[adis][StatefulSet][Apply] creating... sts name: %q, err: %+v", ss.Name, err)
		return errors.Trace(err)
	}
	if err != nil {
		return errors.Trace(err)
	}

	// Statefulset only allows updates to the following fields under ".Spec":
	// 'replicas', 'template', 'updateStrategy', 'persistentVolumeClaimRetentionPolicy' and 'minReadySeconds'.
	existing.SetAnnotations(ss.GetAnnotations())
	existing.Spec.Replicas = ss.Spec.Replicas
	existing.Spec.UpdateStrategy = ss.Spec.UpdateStrategy
	existing.Spec.PersistentVolumeClaimRetentionPolicy = ss.Spec.PersistentVolumeClaimRetentionPolicy
	existing.Spec.MinReadySeconds = ss.Spec.MinReadySeconds
	existing.Spec.Template = ss.Spec.Template

	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, existing)
	if err != nil {
		return errors.Trace(err)
	}
	result, err = ss.client.Patch(ctx, ss.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	logger.Infof("[adis][StatefulSet][Apply] patch... sts name: %q, err: %+v", ss.Name, err)
	if k8serrors.IsNotFound(err) {
		// This should never happen.
		return errors.NewNotFound(err, fmt.Sprintf("statefulset %q", ss.Name))
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "statefulset %q", ss.Name)
	}
	return errors.Trace(err)
}

// Get refreshes the resource.
func (ss *StatefulSet) Get(ctx context.Context) error {
	res, err := ss.client.Get(ctx, ss.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	ss.StatefulSet = *res
	return nil
}

// Delete removes the resource.
func (ss *StatefulSet) Delete(ctx context.Context) error {
	err := ss.client.Delete(ctx, ss.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	logger.Infof("[adis][StatefulSet][Delete] sts: %q, err: %+v", ss.Name, err)
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s statefulset for deletion")
	}
	return errors.Trace(err)
}

// ComputeStatus returns a juju status for the resource.
func (ss *StatefulSet) ComputeStatus(ctx context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if ss.DeletionTimestamp != nil {
		return "", status.Terminated, ss.DeletionTimestamp.Time, nil
	}
	if ss.Status.ReadyReplicas == ss.Status.Replicas {
		return "", status.Active, now, nil
	}
	return "", status.Waiting, now, nil
}

func (s StatefulSetWithOrphanDelete) Delete(ctx context.Context) error {
	err := s.client.Delete(ctx, s.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DeletePropagationOrphan(),
	})
	logger.Infof("[adis][StatefulSetWithOrphanDelete][Delete] sts: %q, err: %+v", s.Name, err)
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s statefulset for deletion")
	}
	// K8s doesn't delete the sts immediately. Block until it's deleted.
	err = wait.PollUntilContextTimeout(ctx, s.interval, s.timeout, true, func(ctx context.Context) (done bool, err error) {
		logger.Infof("[adis][StatefulSetWithOrphanDelete] blocking until sts %q is deleted", s.Name)
		getErr := s.Get(ctx)
		if errors.Is(getErr, errors.NotFound) {
			logger.Infof("[adis][StatefulSetWithOrphanDelete] sts %q is finally deleted ^_^", s.Name)
			return true, nil
		} else if getErr != nil {
			return false, getErr
		}
		return false, nil
	})
	return errors.Trace(err)
}

// ListStatefulSets returns a list of statefulsets.
func ListStatefulSets(ctx context.Context, client v1.StatefulSetInterface, namespace string, opts metav1.ListOptions) ([]StatefulSet, error) {
	var items []StatefulSet
	for {
		res, err := client.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, item := range res.Items {
			items = append(items, *NewStatefulSet(client, namespace, item.Name, &item))
		}
		if res.Continue == "" {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}
