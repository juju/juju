// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"strings"

	"github.com/juju/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
)

// Claim represents an assertion over a generic Kubernetes object to assert
// ownership. These are used in Juju for cluster scoped resources to assert that
// that Juju is not going to take ownership of an object that was not created by
// itself.
type Claim interface {
	// Assert defines the assertion to run. Returns true if a claim is asserted
	// over the provided object or if an error occurred where a claim can not be
	// made.
	Assert(obj interface{}) (bool, error)
}

// ClaimFn is a helper type for making Claim types out of functions. See Claim
type ClaimFn func(obj interface{}) (bool, error)

var (
	// ClaimJujuOwnership asserts that the Kubernetes object has labels that
	// in line with Juju management "ownership".
	ClaimJujuOwnership = ClaimAggregateOr(
		ClaimFn(claimIsManagedByJuju),
		ClaimFn(claimHasJujuLabel),
	)
)

func (c ClaimFn) Assert(obj interface{}) (bool, error) {
	return c(obj)
}

// ClaimAggregateOr runs multiple claims looking for the first true condition.
// If no claims are provided or no claim returns true false is returned. The
// first claim to error stops execution.
func ClaimAggregateOr(claims ...Claim) Claim {
	return ClaimFn(func(obj interface{}) (bool, error) {
		for _, claim := range claims {
			if r, err := claim.Assert(obj); err != nil {
				return r, err
			} else if r {
				return r, err
			}
		}
		return false, nil
	})
}

// claimHasAJujuLabel is a throw everything against the wall and see what sticks
// assertion. It iterates all labels of the object trying to find a key that
// has the lowercase word "juju". We use this because our labeling at one stage
// is a bit hit and miss and no consistency to fall back on.
// TODO: Remove in Juju 3.0
func claimHasJujuLabel(obj interface{}) (bool, error) {
	if obj == nil {
		return false, errors.NewNotValid(nil, "obj for claim cannot be nil")
	}

	metaObj, err := meta.Accessor(obj)
	if err != nil {
		return false, errors.Annotate(err, "asserting Kubernetes object has Juju labels")
	}
	for k := range metaObj.GetLabels() {
		if strings.Contains(k, "juju") {
			return true, nil
		}
	}
	return false, nil
}

// claimIsManagedByJuju is a check to assert that the Kubernetes object provided
// is managed by Juju by having the label key and value of
// app.kubernetes.io/managed-by: juju.
func claimIsManagedByJuju(obj interface{}) (bool, error) {
	if obj == nil {
		return false, errors.NewNotValid(nil, "obj for claim cannot be nil")
	}

	metaObj, err := meta.Accessor(obj)
	if err != nil {
		return false, errors.Annotate(err, "asserting Kubernetes object has managed by juju label")
	}

	val, has := metaObj.GetLabels()[constants.LabelKubernetesAppManaged]
	if !has {
		return false, nil
	}
	return val == "juju", nil
}

// RunClaims runs the provided claims until the first true condition is found or
// the first error occurs. If no claims are provided then true is returned.
func RunClaims(claims ...Claim) Claim {
	return ClaimFn(func(obj interface{}) (bool, error) {
		for _, claim := range claims {
			if r, err := claim.Assert(obj); err != nil {
				return r, err
			} else if r {
				return r, err
			}
		}
		return true, nil
	})
}
