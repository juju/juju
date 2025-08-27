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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
		err = retry.Call(retry.CallArgs{
			Func: func() error {
				err := op.resource.Apply(ctx)
				if errors.Is(err, errConflict) {
					_ = op.resource.Get(ctx) // refresh resource version
					return err
				}
				// this might happen if it were deleted by another apiserver.
				if k8serrors.IsNotFound(err) {
					return err
				}
				return errors.Annotatef(err, "applying resource %q", op.resource.ID().Name)
			},
			IsFatalError: func(err error) bool {
				return !k8serrors.IsConflict(err) && !errors.Is(err, errConflict)
			},
			Clock:       jujuclock.WallClock,
			Attempts:    5,
			Delay:       time.Second,
			BackoffFunc: retry.ExpBackoff(time.Second, 5*time.Second, 1.5, true),
		})

		// we do not want to rollback if the resource is not found, not applied or
		// it has been deleted by another apiserver during the retry call process.
		if err == nil {
			if found {
				// apply the previously existing resource.
				rollback.Apply(existingRes)
			} else {
				// delete the new resource just created.
				rollback.Delete(op.resource)
			}
		}
	case opDelete:
		// delete with retry
		err = retry.Call(retry.CallArgs{
			Func: func() error {
				err := op.resource.Delete(ctx)
				if k8serrors.IsConflict(err) {
					_ = op.resource.Get(ctx) // refresh resource version
					return err
				}
				return errors.Annotatef(err, "deleting resource %q", op.resource.ID().Name)
			},
			IsFatalError: func(err error) bool {
				return !k8serrors.IsConflict(err)
			},
			Clock:       jujuclock.WallClock,
			Attempts:    5,
			Delay:       time.Second,
			BackoffFunc: retry.ExpBackoff(time.Second, 5*time.Second, 1.5, true),
		})

		// do not rollback if the resource is not found or update still has conflicts after retries.
		if err == nil {
			rollback.Apply(existingRes)
		}
	}

	// ignore any uncaught not found error, we just don't do anything.
	// apply would return have returned nil error if new resource is created.
	if k8serrors.IsNotFound(err) {
		return nil
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
