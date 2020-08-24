// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"k8s.io/client-go/kubernetes"
)

var logger = loggo.GetLogger("juju.kubernetes.provider.resources")

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
	err := existingRes.Get(ctx, api)
	notfound := false
	if errors.IsNotFound(err) {
		notfound = true
	} else if err != nil {
		return errors.Annotatef(err, "checking if resource %q exists or not", existingRes)
	}
	switch op.opType {
	case opApply:
		err = op.resource.Apply(ctx, api)
		if notfound {
			// delete the new resource just created.
			rollback.Delete(op.resource)
		} else {
			// apply the previously existing resource.
			rollback.Apply(existingRes)
		}
	case opDelete:
		err = op.resource.Delete(ctx, api)
		if !notfound {
			rollback.Apply(existingRes)
		}
	}
	return errors.Trace(err)
}

func (a *applier) Apply(r Resource) {
	a.ops = append(a.ops, operation{opApply, r})
}

func (a *applier) Delete(r Resource) {
	a.ops = append(a.ops, operation{opDelete, r})
}

func (a *applier) Run(ctx context.Context, client kubernetes.Interface) (err error) {
	rollback := &applier{}

	defer func() {
		if err != nil {
			if rollbackErr := rollback.Run(ctx, client); rollbackErr != nil {
				logger.Warningf("rollback failed %s", rollbackErr.Error())
			}
		}
	}()
	for _, o := range a.ops {
		if err = o.process(ctx, client, rollback); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
