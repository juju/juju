// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"testing"

	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	v4_0_4 "github.com/juju/juju/domain/export/types/v4_0_4"
	v4_0_6 "github.com/juju/juju/domain/export/types/v4_0_6"
	v4_1_0 "github.com/juju/juju/domain/export/types/v4_1_0"
)

type payloadSuite struct{}

func TestPayloadSuite(t *testing.T) {
	tc.Run(t, &payloadSuite{})
}

// TestDecodePayloadRoundTripV404 verifies that a marshalled v4_0_4 payload
// decodes back into the concrete generated type.
func (s *payloadSuite) TestDecodePayloadRoundTripV404(c *tc.C) {
	in := v4_0_4.ModelExport{
		Application: []v4_0_4.Application{{
			UUID:      "app-uuid",
			Name:      "ubuntu",
			CharmUUID: "charm-uuid",
		}},
	}
	data, err := yaml.Marshal(in)
	c.Assert(err, tc.ErrorIsNil)

	decoded, err := DecodePayload(semversion.MustParse("4.0.4"), data)
	c.Assert(err, tc.ErrorIsNil)
	out, ok := decoded.(v4_0_4.ModelExport)
	c.Assert(ok, tc.IsTrue)
	c.Check(out, tc.DeepEquals, in)
}

// TestDecodePayloadRoundTripV406 verifies that a marshalled v4_0_6 payload
// decodes back into the concrete generated type.
func (s *payloadSuite) TestDecodePayloadRoundTripV406(c *tc.C) {
	in := v4_0_6.ModelExport{
		Application: []v4_0_6.Application{{
			UUID:      "app-uuid",
			Name:      "ubuntu",
			CharmUUID: "charm-uuid",
		}},
	}
	data, err := yaml.Marshal(in)
	c.Assert(err, tc.ErrorIsNil)

	decoded, err := DecodePayload(semversion.MustParse("4.0.6"), data)
	c.Assert(err, tc.ErrorIsNil)
	out, ok := decoded.(v4_0_6.ModelExport)
	c.Assert(ok, tc.IsTrue)
	c.Check(out, tc.DeepEquals, in)
}

// TestDecodePayloadRoundTripV410 verifies that a marshalled v4_1_0 payload
// decodes back into the concrete generated type.
func (s *payloadSuite) TestDecodePayloadRoundTripV410(c *tc.C) {
	in := v4_1_0.ModelExport{
		Application: []v4_1_0.Application{{
			UUID:      "app-uuid",
			Name:      "ubuntu",
			CharmUUID: "charm-uuid",
		}},
	}
	data, err := yaml.Marshal(in)
	c.Assert(err, tc.ErrorIsNil)

	decoded, err := DecodePayload(semversion.MustParse("4.1.0"), data)
	c.Assert(err, tc.ErrorIsNil)
	out, ok := decoded.(v4_1_0.ModelExport)
	c.Assert(ok, tc.IsTrue)
	c.Check(out, tc.DeepEquals, in)
}

// TestDecodePayloadUnknownVersion verifies that an unknown payload version
// yields a clean NotSupported error.
func (s *payloadSuite) TestDecodePayloadUnknownVersion(c *tc.C) {
	_, err := DecodePayload(semversion.MustParse("4.0.5"), []byte("{}"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
	c.Assert(err, tc.ErrorMatches, `model export payload version "4.0.5": not supported`)
}

// TestDecodePayloadMalformedYAML verifies that undecodable bytes yield a
// NotValid error.
func (s *payloadSuite) TestDecodePayloadMalformedYAML(c *tc.C) {
	_, err := DecodePayload(semversion.MustParse("4.0.6"), []byte("\t: not yaml"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestDecoderRegistryCompleteness asserts that every supported export version
// has a payload decoder and a working projection view builder. Adding a new
// export version must extend payloadDecoders and ProjectionViewForPayload.
func (s *payloadSuite) TestDecoderRegistryCompleteness(c *tc.C) {
	for _, version := range ExportVersions {
		decode, ok := payloadDecoders[version]
		c.Assert(ok, tc.IsTrue, tc.Commentf("no payload decoder for export version %q", version))

		payload, err := decode([]byte("{}"))
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("decoding empty payload for version %q", version))

		_, err = ProjectionViewForPayload(payload)
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("building projection view for version %q", version))
	}
}

// TestProjectionViewForPayloadUnknownType verifies that a payload type outside
// the registry is rejected with NotSupported.
func (s *payloadSuite) TestProjectionViewForPayloadUnknownType(c *tc.C) {
	_, err := ProjectionViewForPayload(struct{}{})
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

// TestProjectionViewExtraction verifies the view projects the agent target
// version.
func (s *payloadSuite) TestProjectionViewExtraction(c *tc.C) {
	payload := v4_0_6.ModelExport{
		AgentVersion: []v4_0_6.AgentVersion{{
			TargetVersion: "4.0.6",
		}},
	}

	view, err := ProjectionViewForPayload(payload)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(view.AgentTargetVersion, tc.Equals, semversion.MustParse("4.0.6"))
}

// TestProjectionViewNoAgentVersion verifies that a payload without an
// agent_version row leaves the view's agent target version zero.
func (s *payloadSuite) TestProjectionViewNoAgentVersion(c *tc.C) {
	view, err := ProjectionViewForPayload(v4_0_6.ModelExport{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(view.AgentTargetVersion, tc.Equals, semversion.Number{})
}

// TestProjectionViewMultipleAgentVersionRows verifies that a payload with
// more than one agent_version row is rejected as malformed.
func (s *payloadSuite) TestProjectionViewMultipleAgentVersionRows(c *tc.C) {
	_, err := ProjectionViewForPayload(v4_0_6.ModelExport{
		AgentVersion: []v4_0_6.AgentVersion{
			{TargetVersion: "4.0.6"},
			{TargetVersion: "4.0.7"},
		},
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}
