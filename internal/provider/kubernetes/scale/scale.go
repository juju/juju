// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scale

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/juju/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	apps "k8s.io/client-go/kubernetes/typed/apps/v1"
)

// ScalePatcher provides a generic interface for patching replicas count on
// common Kubernetes objects. The Kubernetes objects must support patching of
// spec.replicas to work with this interface
type ScalePatcher interface {
	// Patch patches the supplied object name with a supplied patch operation.
	// The returned scale is provided if the operation completes successfully.
	Patch(context.Context, string, types.PatchType, []byte, meta.PatchOptions, ...string) (int32, error)
}

type scalePatch struct {
	Spec scalePatchSpec `json:"spec"`
}

type scalePatchSpec struct {
	Replicas int32 `json:"replicas"`
}

// ScalePatcherFunc is a convenience func to implement the ScalePatcher interface
type ScalePatcherFunc func(context.Context, string, types.PatchType, []byte, meta.PatchOptions, ...string) (int32, error)

// DeploymentScalePatcher returns a ScalePatcher suitable for use with
// Deployments
func DeploymentScalePatcher(deploy apps.DeploymentInterface) ScalePatcher {
	return ScalePatcherFunc(func(
		c context.Context,
		n string,
		p types.PatchType,
		d []byte,
		o meta.PatchOptions,
		s ...string,
	) (int32, error) {
		deployment, err := deploy.Patch(c, n, p, d, o, s...)
		if k8serrors.IsNotFound(err) {
			return 0, errors.NotFoundf("deployment %q", n)
		} else if err != nil {
			return 0, errors.Annotatef(err, "scale patching deployment %q", n)
		}
		return *deployment.Spec.Replicas, nil
	})
}

// Patch see ScalePatcher.Patch
func (p ScalePatcherFunc) Patch(
	ctx context.Context,
	name string,
	patchType types.PatchType,
	data []byte,
	opts meta.PatchOptions,
	subresources ...string,
) (int32, error) {
	return p(ctx, name, patchType, data, opts, subresources...)
}

// PatchReplicasToScale patches the provided object name with the expected scale.
// If the operation fails an error is returned
func PatchReplicasToScale(
	ctx context.Context,
	name string,
	scale int32,
	patcher ScalePatcher) error {
	if scale < 0 {
		return errors.NewNotValid(nil, "scale cannot be < 0")
	}

	patch, err := json.Marshal(scalePatch{
		Spec: scalePatchSpec{
			Replicas: scale,
		},
	})
	if err != nil {
		return fmt.Errorf("building json patch for for scale on %q", name)
	}

	setScale, err := patcher.Patch(
		ctx,
		name,
		types.StrategicMergePatchType,
		patch,
		meta.PatchOptions{},
	)
	if err != nil {
		return errors.Annotatef(err, "setting scale to %d for %q", scale, name)
	}

	if setScale != scale {
		return fmt.Errorf("patched scale is %d expected %d", setScale, scale)
	}

	return nil
}

// StatefulSetScalePatcher returns a ScalePatcher suitable for use with
// Statefulsets.
func StatefulSetScalePatcher(stateSet apps.StatefulSetInterface) ScalePatcher {
	return ScalePatcherFunc(func(
		c context.Context,
		n string,
		p types.PatchType,
		d []byte,
		o meta.PatchOptions,
		s ...string,
	) (int32, error) {
		ss, err := stateSet.Patch(c, n, p, d, o, s...)
		if k8serrors.IsNotFound(err) {
			return 0, errors.NotFoundf("statefulset %q", n)
		} else if err != nil {
			return 0, errors.Annotatef(err, "scale patching statefulset %q", n)
		}
		return *ss.Spec.Replicas, nil
	})
}
