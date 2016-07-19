// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/worker"
)

// context encapsulates a snapshot of workers and output funcs and implements Context.
type context struct {

	// clientName is the name of the manifold for whose convenience this exists.
	clientName string

	// abort is closed when the worker being started is no longer required.
	abort <-chan struct{}

	// expired is closed when the context should no longer be used.
	expired chan struct{}

	// workers holds the snapshot of manifold workers.
	workers map[string]worker.Worker

	// outputs holds the snapshot of manifold output funcs.
	outputs map[string]OutputFunc

	// accessLog holds the names and types of resource requests, and any error
	// encountered. It does not include requests made after expiry.
	accessLog []resourceAccess
}

// Abort is part of the Context interface.
func (ctx *context) Abort() <-chan struct{} {
	return ctx.abort
}

// Get is part of the Context interface.
func (ctx *context) Get(resourceName string, out interface{}) error {
	logger.Tracef("%q manifold requested %q resource", ctx.clientName, resourceName)
	select {
	case <-ctx.expired:
		return errors.New("expired context: cannot be used outside Start func")
	default:
		err := ctx.rawAccess(resourceName, out)
		ctx.accessLog = append(ctx.accessLog, resourceAccess{
			name: resourceName,
			as:   fmt.Sprintf("%T", out),
			err:  err,
		})
		return err
	}
}

// expire closes the expired channel. Calling it more than once will panic.
func (ctx *context) expire() {
	close(ctx.expired)
}

// rawAccess is a GetResourceFunc that neither checks enpiry nor records access.
func (ctx *context) rawAccess(resourceName string, out interface{}) error {
	input := ctx.workers[resourceName]
	if input == nil {
		// No worker running (or not declared).
		return ErrMissing
	}
	if out == nil {
		// No conversion necessary.
		return nil
	}
	convert := ctx.outputs[resourceName]
	if convert == nil {
		// Conversion required, no func available.
		return ErrMissing
	}
	return convert(input, out)
}

// resourceAccess describes a call made to (*context).Get.
type resourceAccess struct {

	// name is the name of the resource requested.
	name string

	// as is the string representation of the type of the out param.
	as string

	// err is any error returned from rawAccess.
	err error
}

// report returns a convenient representation of ra.
func (ra resourceAccess) report() map[string]interface{} {
	report := map[string]interface{}{
		KeyName: ra.name,
		KeyType: ra.as,
	}
	if ra.err != nil {
		report[KeyError] = ra.err.Error()
	}
	return report
}

// resourceLogReport returns a convenient representation of accessLog.
func resourceLogReport(accessLog []resourceAccess) []map[string]interface{} {
	result := make([]map[string]interface{}, len(accessLog))
	for i, access := range accessLog {
		result[i] = access.report()
	}
	return result
}
