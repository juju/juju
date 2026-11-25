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
	coretesting "github.com/juju/juju/internal/testing"
)

type validateSuite struct {
	coretesting.BaseSuite
}

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

func (s *validateSuite) TestValidatePayloadK8sUnitRanges(c *tc.C) {
	for _, test := range []struct {
		name      string
		units     []v4_1_0.Unit
		sequence  []v4_1_0.Sequence
		pods      []v4_1_0.K8sPod
		modelType string
		legacy    bool
		appLife   int64
		synthetic bool
		invalid   bool
	}{
		{
			name:    "sparse Alive units",
			units:   []v4_1_0.Unit{{Name: "foo/0"}, {Name: "foo/2"}},
			invalid: true,
		}, {
			name:    "duplicate ordinals",
			units:   []v4_1_0.Unit{{Name: "foo/1"}, {Name: "foo/1"}},
			invalid: true,
		}, {
			name:    "invalid name",
			units:   []v4_1_0.Unit{{Name: "foo/invalid"}},
			invalid: true,
		}, {
			name:    "wrong application",
			units:   []v4_1_0.Unit{{Name: "bar/0"}},
			invalid: true,
		}, {
			name:     "sequence ahead of Alive range",
			units:    []v4_1_0.Unit{{Name: "foo/0"}, {Name: "foo/1"}},
			sequence: []v4_1_0.Sequence{{Namespace: "application_foo", Value: 5}},
			invalid:  true,
		}, {
			name:    "departed unit ahead of Alive range",
			units:   []v4_1_0.Unit{{Name: "foo/0"}, {Name: "foo/5", LifeID: 2}},
			invalid: true,
		}, {
			name:    "pod ordinal ahead of Alive range",
			units:   []v4_1_0.Unit{{UUID: "unit", Name: "foo/0"}},
			pods:    []v4_1_0.K8sPod{{UnitUUID: "unit", ProviderID: "foo-5"}},
			invalid: true,
		}, {
			name:     "unordered non-zero Alive range with lower departing unit",
			units:    []v4_1_0.Unit{{Name: "foo/3"}, {Name: "foo/0", LifeID: 1}, {Name: "foo/2"}},
			sequence: []v4_1_0.Sequence{{Namespace: "application_foo", Value: 3}},
		}, {
			name:  "missing sequence is recovered from units",
			units: []v4_1_0.Unit{{Name: "foo/9"}},
		}, {
			name:     "zero retains ordinal history",
			sequence: []v4_1_0.Sequence{{Namespace: "application_foo", Value: 5}},
		}, {
			name:      "machine models allow sparse units",
			modelType: "iaas",
			units:     []v4_1_0.Unit{{Name: "foo/0"}, {Name: "foo/2"}},
		}, {
			name:      "machine model type overrides legacy scale rows",
			modelType: "iaas",
			legacy:    true,
			units:     []v4_1_0.Unit{{Name: "foo/0"}, {Name: "foo/2"}},
		}, {
			name:    "departing applications have no intended range",
			appLife: 1,
			units:   []v4_1_0.Unit{{Name: "foo/0"}, {Name: "foo/2"}},
		}, {
			name:      "synthetic applications allow sparse units",
			synthetic: true,
			units:     []v4_1_0.Unit{{Name: "foo/0"}, {Name: "foo/2"}},
		},
	} {
		modelType := test.modelType
		if modelType == "" {
			modelType = "caas"
		}
		payload := latest.ModelExport{
			ModelAgent:  []v4_1_0.ModelAgent{{}},
			Model:       []v4_1_0.Model{{Type: modelType}},
			Application: []v4_1_0.Application{{UUID: "app", Name: "foo", CharmUUID: "charm", LifeID: test.appLife}},
			Unit:        test.units, Sequence: test.sequence, K8sPod: test.pods,
		}
		for i := range payload.Unit {
			payload.Unit[i].ApplicationUUID = "app"
		}
		if test.synthetic {
			payload.Charm = []v4_1_0.Charm{{UUID: "charm", SourceID: 2}}
		}
		if test.legacy {
			payload.ApplicationScale = []v4_1_0.ApplicationScale{{ApplicationUUID: "app", Scale: new(int64(len(test.units)))}}
		}
		err := modelimport.ValidatePayload(payload)
		if test.invalid {
			c.Check(err, tc.ErrorIs, coreerrors.NotValid, tc.Commentf("%s", test.name))
			c.Check(err, tc.ErrorMatches, `imported application "foo".*`, tc.Commentf("%s", test.name))
		} else {
			c.Check(err, tc.ErrorIsNil, tc.Commentf("%s", test.name))
		}
	}
}

func (s *validateSuite) TestValidatePayloadRejectsSparseLegacyScale(c *tc.C) {
	scale := int64(2)
	err := modelimport.ValidatePayload(latest.ModelExport{
		ModelAgent:       []v4_1_0.ModelAgent{{}},
		Application:      []v4_1_0.Application{{UUID: "app", Name: "foo"}},
		ApplicationScale: []v4_1_0.ApplicationScale{{ApplicationUUID: "app", Scale: &scale}},
		Unit: []v4_1_0.Unit{
			{ApplicationUUID: "app", Name: "foo/0"},
			{ApplicationUUID: "app", Name: "foo/2"},
		},
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `imported application "foo".*not contiguous.*`)
}
