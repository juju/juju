// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/k8s"
	"github.com/juju/juju/internal/testing"
)

type brokerSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&brokerSuite{})

func (s *brokerSuite) TestDeploymentTypeValidation(c *gc.C) {

	validTypes := []k8s.WorkloadType{
		k8s.WorkloadTypeStatefulSet,
		k8s.WorkloadTypeDeployment,
		k8s.WorkloadTypeDaemonSet,
		k8s.WorkloadType(""), // TODO(caas): change deployment to mandatory.
	}
	for _, t := range validTypes {
		c.Check(t.Validate(), jc.ErrorIsNil)
	}

	c.Assert(k8s.WorkloadType("bad type").Validate(), jc.ErrorIs, errors.NotSupported)
}
