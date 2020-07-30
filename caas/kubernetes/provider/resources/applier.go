// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"k8s.io/client-go/kubernetes"
)

type Applier struct {
	ops []operation
}

type operation struct {
	op       op
	resource Resource
}

type op int

const (
	opApply  op = iota
	opDelete op = iota
)

func (a *Applier) Apply(r Resource) {
	a.ops = append(a.ops, operation{opApply, r})
}

func (a *Applier) Delete(r Resource) {
	a.ops = append(a.ops, operation{opDelete, r})
}

func (a *Applier) Run(ctx context.Context, client kubernetes.Interface) error {
	rollback := &Applier{}
	for _, o := range a.ops {
		rsc := o.resource.Clone()
		err := rsc.Get(ctx, client)
		notFound := false
		if errors.IsNotFound(err) {
			notFound = true
		} else if err != nil {
			return err
		}
		switch o.op {
		case opApply:
			if notFound {
				rollback.Delete(rsc)
			} else {
				rollback.Apply(rsc)
			}
		case opDelete:
			if !notFound {
				rollback.Apply(rsc)
			}
		}
	}
	n, err := a.execute(ctx, client, len(a.ops))
	if err != nil {
		_, rerr := rollback.execute(ctx, client, n)
		if rerr != nil {
			return errors.NewNotProvisioned(err, fmt.Sprintf("rollback failed %s", rerr.Error()))
		}
		return errors.Trace(err)
	}
	return nil
}

func (a *Applier) execute(ctx context.Context, client kubernetes.Interface, n int) (int, error) {
	for i, o := range a.ops {
		if i > n {
			return n, nil
		}
		var err error
		switch o.op {
		case opApply:
			err = o.resource.Apply(ctx, client)
		case opDelete:
			err = o.resource.Delete(ctx, client)
		}
		if err != nil {
			return i, errors.Trace(err)
		}
	}
	return len(a.ops), nil
}
