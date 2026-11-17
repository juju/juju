// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport_test

import (
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/domain/modelimport"
)

type validateSuite struct{}

func TestValidateSuite(t *testing.T) {
	tc.Run(t, &validateSuite{})
}

func (s *validateSuite) TestValidatePayload(c *tc.C) {
	passwordHash := "hash"
	err := modelimport.ValidatePayload(latest.ModelExport{
		ModelAgent: []v4_1_0.ModelAgent{{PasswordHash: &passwordHash}},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *validateSuite) TestValidatePayloadMissingModelAgent(c *tc.C) {
	err := modelimport.ValidatePayload(latest.ModelExport{})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, "model export payload has 0 model_agent rows, expected 1.*")
}

// TestValidatePayloadAllowsEmptyModelAgentPassword asserts that a payload
// carrying no model_agent password (the normal state for a non-CAAS model)
// passes validation rather than being rejected.
func (s *validateSuite) TestValidatePayloadAllowsEmptyModelAgentPassword(c *tc.C) {
	err := modelimport.ValidatePayload(latest.ModelExport{
		ModelAgent: []v4_1_0.ModelAgent{{}},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *validateSuite) TestValidatePayloadRejectsUnmaterialisedScale(c *tc.C) {
	scale := int64(2)
	err := modelimport.ValidatePayload(latest.ModelExport{
		ModelAgent:       []v4_1_0.ModelAgent{{}},
		Application:      []v4_1_0.Application{{UUID: "app", Name: "foo"}},
		ApplicationScale: []v4_1_0.ApplicationScale{{ApplicationUUID: "app", Scale: &scale}},
		Unit:             []v4_1_0.Unit{{ApplicationUUID: "app", Name: "foo/0", LifeID: 0}},
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, "imported application .* scale request is not materialised.*")
}

func (s *validateSuite) TestValidatePayloadCountsOnlyAliveUnits(c *tc.C) {
	scale := int64(1)
	err := modelimport.ValidatePayload(latest.ModelExport{
		ModelAgent:       []v4_1_0.ModelAgent{{}},
		Application:      []v4_1_0.Application{{UUID: "app", Name: "foo"}},
		ApplicationScale: []v4_1_0.ApplicationScale{{ApplicationUUID: "app", Scale: &scale}},
		Unit: []v4_1_0.Unit{
			{ApplicationUUID: "app", Name: "foo/1", LifeID: 0},
			{ApplicationUUID: "app", Name: "foo/0", LifeID: 1},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}
