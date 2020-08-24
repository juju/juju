// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"k8s.io/client-go/kubernetes"
)

const (
	// JujuFieldManager marks the resource changes were made by Juju.
	JujuFieldManager = "juju"
)

// Resource defines methods for manipulating a k8s resource.
type Resource interface {
	Clone() Resource
	Apply(ctx context.Context, client kubernetes.Interface) error
	Get(ctx context.Context, client kubernetes.Interface) error
	Delete(ctx context.Context, client kubernetes.Interface) error
	String() string
}

// Applier defines methods for processing a slice of resource operations.
type Applier interface {
	Apply(Resource)
	Delete(Resource)
	Run(context.Context, kubernetes.Interface) error
}
