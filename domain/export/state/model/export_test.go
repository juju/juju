// Copyright 2026 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type exportStateSuiteV4_0_3 struct {
	schematesting.ModelSuite
}

func TestExportStateSuiteV4_0_3(t *testing.T) {
	tc.Run(t, &exportStateSuiteV4_0_3{})
}

func (s *exportStateSuiteV4_0_3) TestExportRuns(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, err := st.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}
