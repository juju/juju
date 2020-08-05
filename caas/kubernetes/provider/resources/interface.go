// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8sresources

import (
	"context"

	"k8s.io/client-go/kubernetes"
)

const (
	JujuFieldManager = "juju"
)

type Resource interface {
	Clone() Resource
	Apply(ctx context.Context, client kubernetes.Interface) error
	Get(ctx context.Context, client kubernetes.Interface) error
	Delete(ctx context.Context, client kubernetes.Interface) error
}
