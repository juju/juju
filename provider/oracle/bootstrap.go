// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	oci "github.com/juju/go-oracle-cloud/api"

	"github.com/juju/errors"
)

// createInstance creates a new vm inside the oracle infrastructure
// and parses  the response into a oracleInstance
func (e *oracleEnviron) createInstance(
	c *oci.Client,
	params oci.InstanceParams,
) (*oracleInstance, error) {

	if len(params.Instances) > 1 {
		return nil, errors.NotSupportedf("launching multiple controllers")
	}

	logger.Debugf("running createInstance")

	// make the actual api request to create the instance
	resp, err := c.CreateInstance(params)
	if err != nil {
		return nil, errors.Trace(err)
	}

	instance, err := newInstance(resp.Instances[0], e)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return instance, nil
}
