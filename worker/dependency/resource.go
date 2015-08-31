// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/worker"
)

type resourceAccess struct {
	name string
	as   string
	err  error
}

func (ra resourceAccess) report() map[string]interface{} {
	return map[string]interface{}{
		KeyName:  ra.name,
		KeyType:  ra.as,
		KeyError: ra.err,
	}
}

func accessReport(accesses []resourceAccess) []map[string]interface{} {
	result := make([]map[string]interface{}, len(accesses))
	for i, access := range accesses {
		result[i] = access.report()
	}
	return result
}

type resourceGetter struct {
	clientName string

	expired chan struct{}

	workers map[string]worker.Worker

	outputs map[string]OutputFunc

	accesses []resourceAccess
}

func (rg *resourceGetter) expire() {
	close(rg.expired)
}

func (rg *resourceGetter) getResource(resourceName string, out interface{}) error {
	select {
	case <-rg.expired:
		return errors.New("expired resourceGetter: cannot be used outside Start func")
	default:
		err := rg.rawAccess(resourceName, out)
		rg.accesses = append(rg.accesses, resourceAccess{
			name: resourceName,
			as:   fmt.Sprintf("%T", out),
			err:  err,
		})
		return err
	}
}

func (rg *resourceGetter) rawAccess(resourceName string, out interface{}) error {
	logger.Debugf("%q manifold requested %q resource", rg.clientName, resourceName)
	input := rg.workers[resourceName]
	if input == nil {
		// No worker running (or not declared).
		return ErrMissing
	}
	convert := rg.outputs[resourceName]
	if convert == nil {
		// No conversion func available...
		if out != nil {
			// ...and the caller wants a resource.
			return ErrMissing
		}
		// ...but it's ok, because the caller depends on existence only.
		return nil
	}
	return convert(input, out)
}
