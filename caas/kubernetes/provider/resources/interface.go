// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/core/status"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/resources_mock.go github.com/juju/juju/caas/kubernetes/provider/resources Resource,Applier

const (
	// JujuFieldManager marks the resource changes were made by Juju.
	JujuFieldManager = "juju"
)

// Resource defines methods for manipulating a k8s resource.
type Resource interface {
	metav1.ObjectMetaAccessor
	// Clone returns a copy of the resource.
	Clone() Resource
	// Apply patches the resource change.
	Apply(ctx context.Context) error
	// Get refreshes the resource.
	Get(ctx context.Context) error
	// Delete removes the resource.
	Delete(ctx context.Context) error
	// String returns a string format containing the name and type of the resource.
	String() string
	// ComputeStatus returns a juju status for the resource.
	ComputeStatus(ctx context.Context, now time.Time) (string, status.Status, time.Time, error)
	// ID returns a comparable ID for the Resource
	ID() ID
}

// Applier defines methods for processing a slice of resource operations.
type Applier interface {
	// Apply adds apply operations to the applier.
	Apply(...Resource)
	// Delete adds delete operations to the applier.
	Delete(...Resource)
	// ApplySet deletes Resources in the current slice that don't exist in the
	// desired slice. All items in the desired slice are applied.
	ApplySet(current []Resource, desired []Resource)
	// Run processes the slice of the operations.
	Run(ctx context.Context, noRollback bool) error
}

// ID represents a compareable identifier for Resources.
type ID struct {
	Type      string
	Name      string
	Namespace string
}
