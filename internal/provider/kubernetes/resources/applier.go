// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"k8s.io/apimachinery/pkg/types"
)

var logger = loggo.GetLogger("juju.kubernetes.provider.resources")

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

func (op *operation) process(ctx context.Context, rollback Applier) error {
	existingRes := op.resource.Clone()
	// TODO: consider to `list` using label selectors instead of `get` by `name`.
	// Because it's not good for non namespaced resources.
	err := existingRes.Get(ctx)
	found := true
	if errors.IsNotFound(err) {
		found = false
	} else if err != nil {
		return errors.Annotatef(err, "checking if resource %q exists or not", existingRes.ID())
	}

	switch op.opType {
	case opApply:
		conflictOccurred := false
		err = retry.Call(retry.CallArgs{
			Func: func() error {
				err := op.resource.Apply(ctx)
				if errors.Is(err, errConflict) {
					conflictOccurred = true
					// Refresh resource version.
					// Ignore err here because we will create resource upon
					// the next retry if the resource is not found.
					_ = op.resource.Get(ctx)
					return err
				}
				return errors.Annotatef(err, "applying resource %q", op.resource.ID().Name)
			},
			IsFatalError: func(err error) bool {
				return !errors.Is(err, errConflict)
			},
			Clock:       jujuclock.WallClock,
			Attempts:    5,
			Delay:       time.Second,
			BackoffFunc: retry.ExpBackoff(time.Second, 5*time.Second, 1.5, true),
		})

		// Do not rollback if the resource is not found, not applied or
		// it has been deleted by another apiserver
		// (leading to not found err after even if may have been initially found)
		// during the retry call process.
		if err == nil {
			// The rollback logic below reflects our current implementation; however, if a rollback
			// occurs, we cannot guarantee it behaves as intended, even in the existing code.
			// There is still an unresolved issue of potential data race here,
			// but we preserve the existing rollback behavior.
			if found {
				// Apply the previously existing resource only if there is no conflict.
				// Avoid apply rollback if there is a conflict since the end result is unpredictable.
				if !conflictOccurred {
					rollback.Apply(existingRes)
				}
			} else {
				// Delete the new resource just created.
				rollback.Delete(op.resource)
			}
		}
	case opDelete:
		err = op.resource.Delete(ctx)
		// We do not need to rollback for not found errors.
		if errors.Is(err, errors.NotFound) {
			return nil
		}

		if err != nil {
			err = errors.Annotatef(err, "deleting resource %q", op.resource.ID().Name)
		}

		// Do not rollback if the resource is not found.
		// There is still an unresolved issue of potential data race here,
		// but we preserve the existing rollback behavior.
		if err == nil {
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

func (a *applier) Run(ctx context.Context, noRollback bool) (err error) {
	rollback := NewApplier()

	defer func() {
		if noRollback || err == nil {
			return
		}
		if rollbackErr := rollback.Run(ctx, true); rollbackErr != nil {
			logger.Warningf("rollback failed %s", rollbackErr.Error())
		}
	}()
	for _, op := range a.ops {
		if err = op.process(ctx, rollback); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
