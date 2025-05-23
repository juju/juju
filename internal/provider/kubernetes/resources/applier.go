// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"github.com/juju/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.kubernetes.provider.resources")

var (
	errConflict = errors.New("resource version conflict")
)

// preferedPatchStrategy is the default patch strategy used by Juju.
const preferedPatchStrategy = types.StrategicMergePatchType

type applier struct {
	ops []operation
}

// NewApplier creates a new applier.
func NewApplier() Applier {
	return &applier{}
}

type opType int

const (
	opApply opType = iota
	opDelete
)

type operation struct {
	opType
	resource Resource
}

func (op *operation) process(ctx context.Context, api kubernetes.Interface, rollback Applier) error {
	existingRes := op.resource.Clone()
	// TODO: consider to `list` using label selectors instead of `get` by `name`.
	// Because it's not good for non namespaced resources.
	err := existingRes.Get(ctx, api)
	found := true
	if errors.Is(err, errors.NotFound) {
		found = false
	} else if err != nil {
		return errors.Annotatef(err, "checking if resource %q exists or not", existingRes)
	}
	if found {
		ver := op.resource.GetObjectMeta().GetResourceVersion()
		if ver != "" && ver != existingRes.GetObjectMeta().GetResourceVersion() {
			id := op.resource.ID()
			return errors.Annotatef(errConflict, "%s %s", id.Type, id.Name)
		}
	}
	switch op.opType {
	case opApply:
		err = op.resource.Apply(ctx, api)
		if found {
			// apply the previously existing resource.
			rollback.Apply(existingRes)
		} else {
			// delete the new resource just created.
			rollback.Delete(op.resource)
		}
	case opDelete:
		err = op.resource.Delete(ctx, api)
		if found {
			rollback.Apply(existingRes)
		}
	}
	return errors.Trace(err)
}

func (a *applier) Apply(resources ...Resource) {
	for _, r := range resources {
		a.ops = append(a.ops, operation{opApply, r})
	}
}

func (a *applier) Delete(resources ...Resource) {
	for _, r := range resources {
		a.ops = append(a.ops, operation{opDelete, r})
	}
}

func (a *applier) ApplySet(current []Resource, desired []Resource) {
	desiredMap := map[ID]bool{}
	for _, r := range desired {
		desiredMap[r.ID()] = true
	}
	for _, r := range current {
		if ok := desiredMap[r.ID()]; !ok {
			a.Delete(r)
		}
	}
	a.Apply(desired...)
}

func (a *applier) Run(ctx context.Context, client kubernetes.Interface, noRollback bool) (err error) {
	rollback := NewApplier()

	defer func() {
		if noRollback || err == nil {
			return
		}
		if rollbackErr := rollback.Run(ctx, client, true); rollbackErr != nil {
			logger.Warningf(context.TODO(), "rollback failed %s", rollbackErr.Error())
		}
	}()
	for _, op := range a.ops {
		if err = op.process(ctx, client, rollback); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
