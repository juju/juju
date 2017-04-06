// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	oci "github.com/juju/go-oracle-cloud/api"

	"github.com/juju/errors"
)

// createInstance creates a new instance inside the oracle infrastructure
func (e *oracleEnviron) createInstance(params oci.InstanceParams) (*oracleInstance, error) {
	if len(params.Instances) > 1 {
		return nil, errors.NotSupportedf("launching multiple instances")
	}

	logger.Debugf("running createInstance")
	resp, err := e.client.CreateInstance(params)
	if err != nil {
		return nil, errors.Trace(err)
	}

	instance, err := newInstance(resp.Instances[0], e)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return instance, nil
}
