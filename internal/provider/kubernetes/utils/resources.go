// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
)

// PurifyResource purifies read only fields before creating/updating the resource.
func PurifyResource(resource interface{ SetResourceVersion(string) }) {
	resource.SetResourceVersion("")
}

func NewUIDPreconditions(uid k8stypes.UID) *metav1.Preconditions {
	if uid == "" {
		return nil
	}
	return &metav1.Preconditions{UID: &uid}
}

func NewPreconditionDeleteOptions(uid k8stypes.UID) metav1.DeleteOptions {
	// TODO(caas): refactor all deleting single resource operation has this UID ensured precondition.
	return metav1.DeleteOptions{
		Preconditions:     NewUIDPreconditions(uid),
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}
}
