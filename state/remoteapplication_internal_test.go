// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	stateerrors "github.com/juju/juju/state/errors"
)

type remoteApplicationOpsSuite struct{}

var _ = gc.Suite(&remoteApplicationOpsSuite{})

// populatedRemoteApplicationDoc returns a document with every field
// set to a non-zero value.
func populatedRemoteApplicationDoc() *remoteApplicationDoc {
	return &remoteApplicationDoc{
		DocID:                "model-uuid:remote-wordpress",
		Name:                 "remote-wordpress",
		OfferUUID:            "offer-uuid",
		URL:                  "me/model.wordpress",
		SourceControllerUUID: "controller-uuid",
		SourceModelUUID:      "source-model-uuid",
		Endpoints: []remoteEndpointDoc{{
			Name:      "db",
			Role:      charm.RoleRequirer,
			Interface: "mysql",
			Limit:     1,
			Scope:     charm.ScopeGlobal,
		}},
		Spaces: []remoteSpaceDoc{{
			CloudType:  "ec2",
			Name:       "space",
			ProviderId: "space-id",
			ProviderAttributes: attributeMap{
				"foo": "bar",
			},
			Subnets: []remoteSubnetDoc{{
				CIDR:              "10.0.0.0/24",
				ProviderId:        "subnet-id",
				VLANTag:           42,
				AvailabilityZones: []string{"az-1"},
				ProviderSpaceId:   "space-id",
				ProviderNetworkId: "network-id",
			}},
		}},
		Bindings:        map[string]string{"db": "space"},
		Life:            Alive,
		RelationCount:   2,
		TxnRevno:        99,
		IsConsumerProxy: true,
		Version:         3,
		Macaroon:        "macaroon-json",
	}
}

// TestRemoteApplicationDocUpdateFieldsGolden pins the exact $set/$unset
// the swap writes for the two boundary shapes of the document. The
// fields are derived by reflection over the bson tags, so this is the
// unit that fails loudly when a new field is added without a deliberate
// omitempty decision (a missing omitempty would $set a zero value over
// the old document's data; an unjustified omitempty would $unset a
// legitimate zero value).
func (s *remoteApplicationOpsSuite) TestRemoteApplicationDocUpdateFieldsGolden(c *gc.C) {
	full := populatedRemoteApplicationDoc()
	c.Assert(remoteApplicationDocUpdateFields(full), gc.DeepEquals, bson.D{
		{"$set", bson.D{
			{"name", full.Name},
			{"offer-uuid", full.OfferUUID},
			{"url", full.URL},
			{"source-controller-uuid", full.SourceControllerUUID},
			{"source-model-uuid", full.SourceModelUUID},
			{"endpoints", full.Endpoints},
			{"spaces", full.Spaces},
			{"bindings", full.Bindings},
			{"life", full.Life},
			{"relationcount", full.RelationCount},
			{"is-consumer-proxy", full.IsConsumerProxy},
			{"version", full.Version},
			{"macaroon", full.Macaroon},
		}},
	})

	empty := &remoteApplicationDoc{}
	c.Assert(remoteApplicationDocUpdateFields(empty), gc.DeepEquals, bson.D{
		{"$set", bson.D{
			{"name", ""},
			{"offer-uuid", ""},
			{"source-controller-uuid", ""},
			{"source-model-uuid", ""},
			{"endpoints", []remoteEndpointDoc(nil)},
			{"spaces", []remoteSpaceDoc(nil)},
			{"bindings", map[string]string(nil)},
			{"life", Life(0)},
			{"relationcount", 0},
			{"is-consumer-proxy", false},
			{"version", 0},
		}},
		{"$unset", bson.D{
			{"url", 1},
			{"macaroon", 1},
		}},
	})
}

// TestReplaceFailedErrorPreservesCause verifies the error wrapping used
// when a replacement triggered from inside the AddRemoteApplication
// transaction build fails: the distinct replacement error type is
// matched while the original cause remains reachable.
func (s *remoteApplicationOpsSuite) TestReplaceFailedErrorPreservesCause(c *gc.C) {
	cause := errors.WithType(errors.Errorf(
		"saas application %q is being removed; retry the consume once the removal completes", "remote-wordpress"),
		errors.AlreadyExists)
	wrapped := errors.WithType(
		errors.Annotatef(cause, "replacing existing saas application %q", "remote-wordpress"),
		stateerrors.RemoteApplicationReplaceFailedError)
	c.Assert(wrapped, gc.ErrorMatches,
		`replacing existing saas application "remote-wordpress": saas application "remote-wordpress" is being removed; retry the consume once the removal completes`)
	c.Check(errors.Is(wrapped, stateerrors.RemoteApplicationReplaceFailedError), jc.IsTrue)
	c.Check(errors.Is(wrapped, errors.AlreadyExists), jc.IsTrue)

	corrupt := errors.WithType(
		errors.Annotatef(
			stateerrors.NewRelationCountCorruptError("remote-wordpress", 2, 1),
			"replacing existing saas application %q", "remote-wordpress"),
		stateerrors.RemoteApplicationReplaceFailedError)
	c.Check(errors.Is(corrupt, stateerrors.RemoteApplicationReplaceFailedError), jc.IsTrue)
	c.Check(errors.Is(corrupt, stateerrors.RelationCountCorruptError), jc.IsTrue)
}
