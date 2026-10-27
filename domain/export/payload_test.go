// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"testing"

	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	ctrlv4_1_0 "github.com/juju/juju/domain/export/types/controller/v4_1_0"
	v4_0_12 "github.com/juju/juju/domain/export/types/v4_0_12"
	v4_1_0 "github.com/juju/juju/domain/export/types/v4_1_0"
)

type payloadSuite struct{}

func TestPayloadSuite(t *testing.T) {
	tc.Run(t, &payloadSuite{})
}

// TestDecodePayloadRoundTripV406 verifies that a marshalled v4_0_12 payload
// decodes back into the concrete generated type.
func (s *payloadSuite) TestDecodePayloadRoundTripV406(c *tc.C) {
	in := v4_0_12.ModelExport{
		Application: []v4_0_12.Application{{
			UUID:      "app-uuid",
			Name:      "ubuntu",
			CharmUUID: "charm-uuid",
		}},
	}
	data, err := yaml.Marshal(in)
	c.Assert(err, tc.ErrorIsNil)

	decoded, err := DecodePayload(semversion.MustParse("4.0.12"), data)
	c.Assert(err, tc.ErrorIsNil)
	out, ok := decoded.(v4_0_12.ModelExport)
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
// yields a NotSupported error naming the controller to upgrade, rather than
// only the version that was rejected.
func (s *payloadSuite) TestDecodePayloadUnknownVersion(c *tc.C) {
	_, err := DecodePayload(semversion.MustParse("4.0.5"), []byte("{}"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
	c.Check(err, tc.ErrorMatches,
		`source payload version "4.0.5" predates the 4.0 export format this controller imports \("4.0.12"\); upgrade the source controller in place to the latest 4.0 release, then retry the migration.*`)
}

// TestDecodePayloadMissingDecoder verifies the fallback error when a supported
// payload version has no registered decoder.
func (s *payloadSuite) TestDecodePayloadMissingDecoder(c *tc.C) {
	originalDecoders := payloadDecoders
	payloadDecoders = map[semversion.Number]PayloadDecodeFunc{}
	c.Cleanup(func() {
		payloadDecoders = originalDecoders
	})

	_, err := DecodePayload(semversion.MustParse("4.0.12"), []byte("{}"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
	c.Check(err, tc.ErrorMatches,
		`model export payload version "4.0.12" has no decoder.*`)
}

// TestDecodePayloadMalformedYAML verifies that undecodable bytes yield a
// NotValid error.
func (s *payloadSuite) TestDecodePayloadMalformedYAML(c *tc.C) {
	_, err := DecodePayload(semversion.MustParse("4.0.12"), []byte("\t: not yaml"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestDecoderRegistryCompleteness asserts that every supported export version
// has a payload decoder. Adding a new export version must extend payloadDecoders.
func (s *payloadSuite) TestDecoderRegistryCompleteness(c *tc.C) {
	for _, version := range ExportVersions {
		decode, ok := payloadDecoders[version]
		c.Assert(ok, tc.IsTrue, tc.Commentf("no payload decoder for export version %q", version))

		_, err := decode([]byte("{}"))
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("decoding empty payload for version %q", version))
	}
}

// TestDecodeControllerPayloadRoundTrip verifies that a marshalled
// v4_1_0 controller payload decodes back into the concrete generated type.
func (s *payloadSuite) TestDecodeControllerPayloadRoundTrip(c *tc.C) {
	in := ctrlv4_1_0.ControllerExport{
		Controller: []ctrlv4_1_0.Controller{{
			UUID:          "controller-uuid",
			ModelUUID:     "model-uuid",
			TargetVersion: "4.1.0",
		}},
	}
	data, err := yaml.Marshal(in)
	c.Assert(err, tc.ErrorIsNil)

	decoded, err := DecodeControllerPayload(semversion.MustParse("4.1.0"), data)
	c.Assert(err, tc.ErrorIsNil)
	out, ok := decoded.(ctrlv4_1_0.ControllerExport)
	c.Assert(ok, tc.IsTrue)
	c.Check(out, tc.DeepEquals, in)
}

// TestDecodeControllerPayloadUnknownVersion verifies that an unknown
// controller export version is reported with NotSupported.
func (s *payloadSuite) TestDecodeControllerPayloadUnknownVersion(c *tc.C) {
	_, err := DecodeControllerPayload(semversion.MustParse("3.0.0"), []byte("{}"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

// TestControllerDecoderRegistryCompleteness asserts that every supported
// controller export version has a decoder.
func (s *payloadSuite) TestControllerDecoderRegistryCompleteness(c *tc.C) {
	for _, version := range ControllerExportVersions {
		decode, ok := controllerPayloadDecoders[version]
		c.Assert(ok, tc.IsTrue, tc.Commentf("no controller payload decoder for export version %q", version))

		_, err := decode([]byte("{}"))
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("decoding empty controller payload for version %q", version))
	}
}

// TestProjectionViewExtraction verifies agent tables take precedence over
// competing model config in the transformed payload.
func (s *payloadSuite) TestProjectionViewExtraction(c *tc.C) {
	streamID := int64(1)
	payload := v4_1_0.ModelExport{
		AgentVersion: []v4_1_0.AgentVersion{{
			StreamID:      streamID,
			TargetVersion: "4.1.0",
		}},
		AgentStream: []v4_1_0.AgentStream{
			{ID: new(int64(0)), Name: "released"},
			{ID: &streamID, Name: "proposed"},
		},
		ModelConfig: []v4_1_0.ModelConfig{
			{Key: "agent-stream", Value: "devel"},
			{Key: "agent-version", Value: "99.0.0"},
		},
	}

	view, err := ProjectionViewForPayload(payload)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(view.AgentTargetVersion, tc.Equals, semversion.MustParse("4.1.0"))
	c.Check(view.AgentStream, tc.Equals, "proposed")
}

func (s *payloadSuite) TestProjectionViewStreamWithoutModelConfig(c *tc.C) {
	streamID := int64(2)
	view, err := ProjectionViewForPayload(v4_1_0.ModelExport{
		AgentVersion: []v4_1_0.AgentVersion{{
			StreamID:      streamID,
			TargetVersion: "4.1.0",
		}},
		AgentStream: []v4_1_0.AgentStream{{ID: &streamID, Name: "testing"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(view.AgentStream, tc.Equals, "testing")
}

func (s *payloadSuite) TestProjectionViewInvalidAgentStream(c *tc.C) {
	for _, streams := range [][]v4_1_0.AgentStream{
		nil,
		{{Name: "proposed"}},
		{{ID: new(int64(0)), Name: "released"}},
		{{ID: new(int64(1))}},
	} {
		_, err := ProjectionViewForPayload(v4_1_0.ModelExport{
			AgentVersion: []v4_1_0.AgentVersion{{
				StreamID:      1,
				TargetVersion: "4.1.0",
			}},
			AgentStream: streams,
			ModelConfig: []v4_1_0.ModelConfig{{Key: "agent-stream", Value: "released"}},
		})
		c.Check(err, tc.ErrorIs, coreerrors.NotValid, tc.Commentf("streams: %#v", streams))
	}
}

// TestProjectionViewNoAgentVersion verifies that a payload without an
// agent_version row leaves the view's agent target version zero.
func (s *payloadSuite) TestProjectionViewNoAgentVersion(c *tc.C) {
	view, err := ProjectionViewForPayload(v4_1_0.ModelExport{
		ModelConfig: []v4_1_0.ModelConfig{{Key: "agent-stream", Value: "devel"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(view.AgentTargetVersion, tc.Equals, semversion.Number{})
	c.Check(view.AgentStream, tc.Equals, "")
}

// TestProjectionViewMultipleAgentVersionRows verifies that a payload with
// more than one agent_version row is rejected as malformed.
func (s *payloadSuite) TestProjectionViewMultipleAgentVersionRows(c *tc.C) {
	_, err := ProjectionViewForPayload(v4_1_0.ModelExport{
		AgentVersion: []v4_1_0.AgentVersion{
			{TargetVersion: "4.1.0"},
			{TargetVersion: "4.0.7"},
		},
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}
