// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"k8s.io/client-go/kubernetes"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/resources_mock.go github.com/juju/juju/caas/kubernetes/provider/resources Resource,Applier

const (
	// JujuFieldManager marks the resource changes were made by Juju.
	JujuFieldManager = "juju"
)

// Resource defines methods for manipulating a k8s resource.
type Resource interface {
	// Clone returns a copy of the resource.
	Clone() Resource
	// Apply patches the resource change.
	Apply(ctx context.Context, client kubernetes.Interface) error
	// Get refreshes the resource.
	Get(ctx context.Context, client kubernetes.Interface) error
	// Delete removes the resource.
	Delete(ctx context.Context, client kubernetes.Interface) error
	// String returns a string format containing the name and type of the resource.
	String() string
}

// Applier defines methods for processing a slice of resource operations.
type Applier interface {
	// Apply adds an apply operation to the applier.
	Apply(Resource)
	// Delete adds an delete operation to the applier.
	Delete(Resource)
	// Run processes the slice of the operations.
	Run(ctx context.Context, client kubernetes.Interface, noRollback bool) error
}
