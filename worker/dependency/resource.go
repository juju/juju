// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/worker"
)

// resourceGetter encapsulates a snapshot of workers and output funcs and exposes
// a getResource method that can be used as a GetResourceFunc.
type resourceGetter struct {

	// clientName is the name of the manifold for whose convenience this exists.
	clientName string

	// expired is closed when the resourceGetter should no longer be used.
	expired chan struct{}

	// workers holds the snapshot of manifold workers.
	workers map[string]worker.Worker

	// outputs holds the snapshot of manifold output funcs.
	outputs map[string]OutputFunc

	// accessLog holds the names and types of resource requests, and any error
	// encountered. It does not include requests made after expiry.
	accessLog []resourceAccess
}

// expire closes the expired channel. Calling it more than once will panic.
func (rg *resourceGetter) expire() {
	close(rg.expired)
}

// getResource is intended for use as the GetResourceFunc passed into the Start
// func of the client manifold.
func (rg *resourceGetter) getResource(resourceName string, out interface{}) error {
	logger.Tracef("%q manifold requested %q resource", rg.clientName, resourceName)
	select {
	case <-rg.expired:
		return errors.New("expired resourceGetter: cannot be used outside Start func")
	default:
		err := rg.rawAccess(resourceName, out)
		rg.accessLog = append(rg.accessLog, resourceAccess{
			name: resourceName,
			as:   fmt.Sprintf("%T", out),
			err:  err,
		})
		return err
	}
}

// rawAccess is a GetResourceFunc that neither checks enpiry nor records access.
func (rg *resourceGetter) rawAccess(resourceName string, out interface{}) error {
	input := rg.workers[resourceName]
	if input == nil {
		// No worker running (or not declared).
		return ErrMissing
	}
	if out == nil {
		// No conversion necessary.
		return nil
	}
	convert := rg.outputs[resourceName]
	if convert == nil {
		// Conversion required, no func available.
		return ErrMissing
	}
	return convert(input, out)
}

// resourceAccess describes a call made to (*resourceGetter).getResource.
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
	return map[string]interface{}{
		KeyName:  ra.name,
		KeyType:  ra.as,
		KeyError: ra.err,
	}
}

// resourceLogReport returns a convenient representation of accessLog.
func resourceLogReport(accessLog []resourceAccess) []map[string]interface{} {
	result := make([]map[string]interface{}, len(accessLog))
	for i, access := range accessLog {
		result[i] = access.report()
	}
	return result
}
