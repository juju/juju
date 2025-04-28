// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// addRelationSuite is a test suite dedicated to check functionalities
// related to adding relation.
// It extends baseRelationSuite to leverage common setup and utility methods
// for relation-related testing and provides more builder dedicated for this
// specific context.
type addRelationSuite struct {
	baseRelationSuite

	// charmByApp maps application IDs to their associated charm IDs for quick
	// lookup during tests.
	charmByApp map[coreapplication.ID]corecharm.ID
}

var _ = gc.Suite(&addRelationSuite{})

func (s *addRelationSuite) SetUpTest(c *gc.C) {
	s.baseRelationSuite.SetUpTest(c)
	s.charmByApp = make(map[coreapplication.ID]corecharm.ID)
}

func (s *addRelationSuite) TestAddRelation(c *gc.C) {
	// Arrange
	relProvider := charm.Relation{
		Name:  "prov",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	}
	relRequirer := charm.Relation{
		Name:  "req",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	}
	appUUID1 := s.addApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	epUUID1 := s.addApplicationEndpointFromRelation(c, appUUID1, relProvider)
	epUUID2 := s.addApplicationEndpointFromRelation(c, appUUID2, relRequirer)
	epUUID3 := s.addApplicationEndpointFromRelation(c, appUUID2, relProvider)
	epUUID4 := s.addApplicationEndpointFromRelation(c, appUUID1, relRequirer)

	// Act
	ep1, ep2, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "req",
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error while inserting the first relation: %s",
		errors.ErrorStack(err)))
	ep3, ep4, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "req",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "prov",
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error while inserting the second relation: %s",
		errors.ErrorStack(err)))

	// Assert
	c.Check(ep1, gc.Equals, relation.Endpoint{
		ApplicationName: "application-1",
		Relation:        relProvider,
	})
	c.Check(ep2, gc.Equals, relation.Endpoint{
		ApplicationName: "application-2",
		Relation:        relRequirer,
	})
	c.Check(ep3, gc.Equals, relation.Endpoint{
		ApplicationName: "application-1",
		Relation:        relRequirer,
	})
	c.Check(ep4, gc.Equals, relation.Endpoint{
		ApplicationName: "application-2",
		Relation:        relProvider,
	})
	epUUIDsByRelID := s.fetchAllEndpointUUIDsByRelationIDs(c)
	c.Check(epUUIDsByRelID, gc.HasLen, 2)
	c.Check(epUUIDsByRelID[0], jc.SameContents, []corerelation.EndpointUUID{epUUID1, epUUID2},
		gc.Commentf("full map: %v", epUUIDsByRelID))
	c.Check(epUUIDsByRelID[1], jc.SameContents, []corerelation.EndpointUUID{epUUID3, epUUID4},
		gc.Commentf("full map: %v", epUUIDsByRelID))

	// check all relation have a status
	statuses := s.fetchAllRelationStatusesOrderByRelationIDs(c)
	c.Check(statuses, jc.DeepEquals, []corestatus.Status{corestatus.Joining, corestatus.Joining},
		gc.Commentf("all relations should have the same starting status: %q", corestatus.Joining))

}

func (s *addRelationSuite) TestAddRelationSubordinate(c *gc.C) {
	// Arrange
	relProvider := charm.Relation{
		Name:  "prov",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeContainer,
	}
	relRequirer := charm.Relation{
		Name:  "req",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	}
	channel := corebase.Channel{
		Track: "20.04",
		Risk:  "stable",
	}
	appUUID1 := s.addSubordinateApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	s.addApplicationPlatform(c, appUUID1, channel)
	s.addApplicationPlatform(c, appUUID2, channel)
	epUUID1 := s.addApplicationEndpointFromRelation(c, appUUID1, relProvider)
	epUUID2 := s.addApplicationEndpointFromRelation(c, appUUID2, relRequirer)

	// Act
	ep1, ep2, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "req",
	})

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ep1, gc.Equals, relation.Endpoint{
		ApplicationName: "application-1",
		Relation:        relProvider,
	})
	c.Check(ep2, gc.Equals, relation.Endpoint{
		ApplicationName: "application-2",
		Relation:        relRequirer,
	})
	epUUIDsByRelID := s.fetchAllEndpointUUIDsByRelationIDs(c)
	c.Check(epUUIDsByRelID, gc.HasLen, 1)
	c.Check(epUUIDsByRelID[0], jc.SameContents, []corerelation.EndpointUUID{epUUID1, epUUID2},
		gc.Commentf("full map: %v", epUUIDsByRelID))

	// check all relation have a status
	statuses := s.fetchAllRelationStatusesOrderByRelationIDs(c)
	c.Check(statuses, jc.DeepEquals, []corestatus.Status{corestatus.Joining},
		gc.Commentf("all relations should have the same starting status: %q", corestatus.Joining))
}

func (s *addRelationSuite) TestAddRelationSubordinateNotCompatible(c *gc.C) {
	// Arrange
	relProvider := charm.Relation{
		Name:  "prov",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeContainer,
	}
	relRequirer := charm.Relation{
		Name:  "req",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	}
	channel1 := corebase.Channel{
		Track: "20.04",
		Risk:  "stable",
	}
	channel2 := corebase.Channel{
		Track: "22.04",
		Risk:  "stable",
	}
	appUUID1 := s.addSubordinateApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	s.addApplicationPlatform(c, appUUID1, channel1)
	s.addApplicationPlatform(c, appUUID2, channel2)
	s.addApplicationEndpointFromRelation(c, appUUID1, relProvider)
	s.addApplicationEndpointFromRelation(c, appUUID2, relRequirer)

	// Act
	_, _, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "req",
	})

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.CompatibleEndpointsNotFound)
}

func (s *addRelationSuite) TestAddRelationErrorInfersEndpoint(c *gc.C) {
	// Act
	_, _, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
	})

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.RelationEndpointNotFound)
}

func (s *addRelationSuite) TestAddRelationErrorAlreadyExists(c *gc.C) {
	// Arrange
	relProvider := charm.Relation{
		Name:  "prov",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	}
	relRequirer := charm.Relation{
		Name:  "req",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	}
	appUUID1 := s.addApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	s.addApplicationEndpointFromRelation(c, appUUID1, relProvider)
	s.addApplicationEndpointFromRelation(c, appUUID2, relRequirer)

	// Act
	_, _, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "req",
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error while inserting the first relation: %s",
		errors.ErrorStack(err)))
	_, _, err = s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "req",
	})

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.RelationAlreadyExists)
}

func (s *addRelationSuite) TestAddRelationErrorCandidateIsPeer(c *gc.C) {
	// Arrange
	relPeer := charm.Relation{
		Name:  "peer",
		Role:  charm.RolePeer,
		Scope: charm.ScopeGlobal,
	}
	appUUID1 := s.addApplication(c, "application")
	s.addApplicationEndpointFromRelation(c, appUUID1, relPeer)

	// Act
	_, _, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application",
		EndpointName:    "peer",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application",
		EndpointName:    "peer",
	})

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.CompatibleEndpointsNotFound)
}

func (s *addRelationSuite) TestAddRelationErrorNotAliveFirstApp(c *gc.C) {
	// Arrange
	relProvider := charm.Relation{
		Name:  "prov",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	}
	relRequirer := charm.Relation{
		Name:  "req",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	}
	appUUID1 := s.addApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	s.addApplicationEndpointFromRelation(c, appUUID1, relProvider)
	s.addApplicationEndpointFromRelation(c, appUUID2, relRequirer)
	s.setLife(c, "application", appUUID1.String(), life.Dying)

	// Act
	_, _, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "req",
	})

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationNotAlive, gc.Commentf("(Assert) %s",
		errors.ErrorStack(err)))
}

func (s *addRelationSuite) TestAddRelationErrorNotAliveSecond(c *gc.C) {
	// Arrange
	relProvider := charm.Relation{
		Name:  "prov",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	}
	relRequirer := charm.Relation{
		Name:  "req",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	}
	appUUID1 := s.addApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	s.addApplicationEndpointFromRelation(c, appUUID1, relProvider)
	s.addApplicationEndpointFromRelation(c, appUUID2, relRequirer)
	s.setLife(c, "application", appUUID2.String(), life.Dying)

	// Act
	_, _, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "req",
	})

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationNotAlive, gc.Commentf("(Assert) %s",
		errors.ErrorStack(err)))
}

func (s *addRelationSuite) TestAddRelationErrorProviderCapacityExceeded(c *gc.C) {
	// Arrange
	relProvider := charm.Relation{
		Name:  "prov",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
		Limit: 1,
	}
	relRequirer := charm.Relation{
		Name:  "req",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	}
	appUUID1 := s.addApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	appUUID3 := s.addApplication(c, "application-3")
	s.addApplicationEndpointFromRelation(c, appUUID1, relProvider)
	s.addApplicationEndpointFromRelation(c, appUUID2, relRequirer)
	s.addApplicationEndpointFromRelation(c, appUUID3, relRequirer)

	// Act
	_, _, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "req",
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error while inserting the first relation: %s",
		errors.ErrorStack(err)))
	_, _, err = s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-3",
		EndpointName:    "req",
	})

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.EndpointQuotaLimitExceeded)
}

func (s *addRelationSuite) TestAddRelationErrorRequirerCapacityExceeded(c *gc.C) {
	// Arrange
	relProvider := charm.Relation{
		Name:  "prov",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	}
	relRequirer := charm.Relation{
		Name:  "req",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
		Limit: 1,
	}
	appUUID1 := s.addApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	appUUID3 := s.addApplication(c, "application-3")
	s.addApplicationEndpointFromRelation(c, appUUID1, relProvider)
	s.addApplicationEndpointFromRelation(c, appUUID2, relProvider)
	s.addApplicationEndpointFromRelation(c, appUUID3, relRequirer)

	// Act
	_, _, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-3",
		EndpointName:    "req",
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error while inserting the first relation: %s",
		errors.ErrorStack(err)))
	_, _, err = s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-3",
		EndpointName:    "req",
	})

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.EndpointQuotaLimitExceeded)
}

func (s *addRelationSuite) TestAddRelationWithID(c *gc.C) {
	// Arrange
	relProvider := charm.Relation{
		Name:  "prov",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	}
	relRequirer := charm.Relation{
		Name:  "req",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	}
	appUUID1 := s.addApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	_ = s.addApplicationEndpointFromRelation(c, appUUID1, relProvider)
	_ = s.addApplicationEndpointFromRelation(c, appUUID2, relRequirer)
	_ = s.addApplicationEndpointFromRelation(c, appUUID2, relProvider)
	_ = s.addApplicationEndpointFromRelation(c, appUUID1, relRequirer)
	expectedRelID := uint64(42)

	// Act
	obtainedRelUUID, err := s.state.SetRelationWithID(context.Background(), corerelation.EndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "req",
	}, corerelation.EndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "prov",
	}, expectedRelID)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	foundRelUUID := s.fetchRelationUUIDByRelationID(c, expectedRelID)
	c.Assert(obtainedRelUUID, gc.Equals, foundRelUUID)
}

func (s *addRelationSuite) TestInferEndpoints(c *gc.C) {
	// Arrange:
	db, err := s.state.DB()
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) cannot get the DB: %s", errors.ErrorStack(err)))

	appUUID1 := s.addApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	appUUID3 := s.addApplication(c, "application-3")
	appSubUUID := s.addSubordinateApplication(c, "application-sub")

	// Create endpoints on applications:
	uuids := map[string]corerelation.EndpointUUID{
		// - Application 1: (all are providers)
		//   - interface: unique / name: whatever
		//   - interface: duplicated / name: pickme
		//   - interface: duplicated / name: filler
		"application-1:whatever": s.addApplicationEndpoint(c, appUUID1, "whatever", charm.RoleProvider, "unique"),
		"application-1:pickme": s.addApplicationEndpoint(c, appUUID1, "pickme", charm.RoleProvider,
			"duplicated"),
		"application-1:filler": s.addApplicationEndpoint(c, appUUID1, "filler", charm.RoleProvider,
			"duplicated"),
		// - Application 2: (all are requirers)
		//   - interface: unique / name: whatever
		//   - interface: duplicated / name: pickme
		//   - interface: duplicated / name: filler
		"application-2:whatever": s.addApplicationEndpoint(c, appUUID2, "whatever", charm.RoleRequirer, "unique"),
		"application-2:pickme": s.addApplicationEndpoint(c, appUUID2, "pickme", charm.RoleRequirer,
			"duplicated"),
		"application-2:filler": s.addApplicationEndpoint(c, appUUID2, "filler", charm.RoleRequirer,
			"duplicated"),
		// - Application 3: (all are requirers)
		//   - interface: unique / name: whatever
		"application-3:whatever": s.addApplicationEndpoint(c, appUUID3, "whatever", charm.RoleRequirer, "unique"),
		// - Application Sub: provider on Container scope
		"application-sub:whatever": s.addApplicationEndpointFromRelation(c, appSubUUID, charm.Relation{
			Name:      "whatever",
			Role:      charm.RoleProvider,
			Interface: "unique",
			Scope:     charm.ScopeContainer,
		}),
	}

	cases := []struct {
		description          string
		input1, input2       string
		expected1, expected2 string
	}{
		{
			description: "fully qualified",
			input1:      "application-1:pickme",
			input2:      "application-2:pickme",
			expected1:   "application-1:pickme",
			expected2:   "application-2:pickme",
		}, {
			description: "first identifier not fully qualified",
			input1:      "application-1",
			input2:      "application-2:whatever",
			expected1:   "application-1:whatever",
			expected2:   "application-2:whatever",
		}, {
			description: "second identifier not fully qualified",
			input1:      "application-1:whatever",
			input2:      "application-2",
			expected1:   "application-1:whatever",
			expected2:   "application-2:whatever",
		}, {
			description: "both identifier not fully qualified",
			input1:      "application-1",
			input2:      "application-3",
			expected1:   "application-1:whatever",
			expected2:   "application-3:whatever",
		}, {
			description: "both identifier not fully qualified, but one is subordinate",
			input1:      "application-sub",
			input2:      "application-3",
			expected1:   "application-sub:whatever",
			expected2:   "application-3:whatever",
		},
	}

	for i, tc := range cases {
		identifier1 := s.newEndpointIdentifier(c, tc.input1)
		identifier2 := s.newEndpointIdentifier(c, tc.input2)

		// Act
		var uuid1, uuid2 corerelation.EndpointUUID
		err := db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
			ep1, ep2, err := s.state.inferEndpoints(ctx, tx, identifier1, identifier2)
			uuid1 = ep1.EndpointUUID
			uuid2 = ep2.EndpointUUID
			return err
		})

		// Assert
		c.Logf("test %d of %d: %s", i+1, len(cases), tc.description)
		if c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) %s: unexpected error: %s", tc.description,
			errors.ErrorStack(err))) {
			c.Check(uuid1, gc.Equals, uuids[tc.expected1], gc.Commentf("(Assert) %s", tc.description))
			c.Check(uuid2, gc.Equals, uuids[tc.expected2], gc.Commentf("(Assert) %s", tc.description))
		}
	}
}

func (s *addRelationSuite) TestInferEndpointsError(c *gc.C) {
	// Arrange:
	db, err := s.state.DB()
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) cannot get the DB: %s", errors.ErrorStack(err)))

	// Create endpoints on applications:
	appUUID1 := s.addApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	appUUID3 := s.addApplication(c, "application-3")

	// - Application 1: name == role
	//   - interface: test / name: provider
	//   - interface: test / name: requirer
	//   - interface: test / name: provider-container / scope container
	s.addApplicationEndpoint(c, appUUID1, "provider", charm.RoleProvider, "test")
	s.addApplicationEndpoint(c, appUUID1, "requirer", charm.RoleRequirer, "test")
	s.addApplicationEndpointFromRelation(c, appUUID1, charm.Relation{
		Name:      "provider-container",
		Role:      charm.RoleProvider,
		Interface: "test",
		Scope:     charm.ScopeContainer,
	})

	// - Application 2:  name == role
	//   - interface: test / name: provider
	//   - interface: test / name: requirer
	//   - interface: test / name: peer
	//   - interface: other / name: provider
	s.addApplicationEndpoint(c, appUUID2, "provider", charm.RoleProvider, "test")
	s.addApplicationEndpoint(c, appUUID2, "requirer", charm.RoleRequirer, "test")
	s.addApplicationEndpoint(c, appUUID2, "peer", charm.RolePeer, "test")
	s.addApplicationEndpoint(c, appUUID2, "other-provider", charm.RoleProvider, "other")

	// - Application 3: different interface than other app
	//   - interface: other / name: first-requirer
	//   - interface: other / name: second-requirer
	s.addApplicationEndpoint(c, appUUID3, "first-requirer", charm.RoleRequirer, "other")
	s.addApplicationEndpoint(c, appUUID3, "second-requirer", charm.RoleRequirer, "other")

	cases := []struct {
		description    string
		input1, input2 string
		expectedError  error
	}{
		{
			description:   "provider with provider",
			input1:        "application-1:provider",
			input2:        "application-2:provider",
			expectedError: relationerrors.CompatibleEndpointsNotFound,
		},
		{
			description:   "provider with peer",
			input1:        "application-1:provider",
			input2:        "application-2:peer",
			expectedError: relationerrors.CompatibleEndpointsNotFound,
		},
		{
			description:   "requirer with requirer",
			input1:        "application-1:requirer",
			input2:        "application-2:requirer",
			expectedError: relationerrors.CompatibleEndpointsNotFound,
		},
		{
			description:   "requirer with peer",
			input1:        "application-1:requirer",
			input2:        "application-2:peer",
			expectedError: relationerrors.CompatibleEndpointsNotFound,
		},
		{
			description:   "unknown endpoints application-1",
			input1:        "application-1:oupsy",
			input2:        "application-2:peer",
			expectedError: relationerrors.RelationEndpointNotFound,
		},
		{
			description:   "unknown endpoints application-2",
			input1:        "application-1:provider",
			input2:        "application-2:oupsy",
			expectedError: relationerrors.RelationEndpointNotFound,
		},
		{
			description:   "no matches (no common interface)",
			input1:        "application-1",
			input2:        "application-3",
			expectedError: relationerrors.CompatibleEndpointsNotFound,
		},
		{
			description:   "ambiguous on interface 'other'",
			input1:        "application-2",
			input2:        "application-3",
			expectedError: relationerrors.AmbiguousRelation,
		},
		{
			description:   "possible match, but with one endpoint on container scope",
			input1:        "application-1:provider-container",
			input2:        "application-2:requirer",
			expectedError: relationerrors.CompatibleEndpointsNotFound,
		},
	}

	for i, tc := range cases {
		identifier1 := s.newEndpointIdentifier(c, tc.input1)
		identifier2 := s.newEndpointIdentifier(c, tc.input2)

		// Act
		err := db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
			_, _, err = s.state.inferEndpoints(ctx, tx, identifier1, identifier2)
			return err
		})

		// Assert
		c.Logf("test %d of %d: %s", i+1, len(cases), tc.description)
		c.Check(err, jc.ErrorIs, tc.expectedError, gc.Commentf("(Assert) %s", tc.description))
	}
}

// addApplication creates and adds a new application with the specified name and
// returns its unique identifier.
// It creates a specific charm for this application.
func (s *addRelationSuite) addApplication(
	c *gc.C,
	applicationName string,
) coreapplication.ID {
	charmUUID := s.addCharm(c)
	s.addCharmMetadata(c, charmUUID, false)
	appUUID := s.baseRelationSuite.addApplication(c, charmUUID, applicationName)
	s.charmByApp[appUUID] = charmUUID
	return appUUID
}

// addApplicationPlatform inserts a new application platform into the database
// using the provided application UUID and channel.
// Os is defaulted to ubuntu and architecture to AMD64 (db zero-values)
func (s *addRelationSuite) addApplicationPlatform(
	c *gc.C,
	appUUID coreapplication.ID,
	channel corebase.Channel,
) {
	s.query(c, `
INSERT INTO application_platform (application_uuid, os_id, channel, architecture_id)
VALUES (?, 0, ?, 0)`, appUUID, channel.String())
}

// addSubordinateApplication creates and adds a new subordinate application
// with the specified name and returns its unique identifier.
// It creates a specific charm for this application.
func (s *addRelationSuite) addSubordinateApplication(
	c *gc.C,
	applicationName string,
) coreapplication.ID {
	charmUUID := s.addCharm(c)
	s.addCharmMetadata(c, charmUUID, true)
	appUUID := s.baseRelationSuite.addApplication(c, charmUUID, applicationName)
	s.charmByApp[appUUID] = charmUUID
	return appUUID
}

// addApplicationEndpoint adds a new application endpoint with the specified
// attributes and returns its unique identifier.
func (s *addRelationSuite) addApplicationEndpoint(
	c *gc.C,
	appUUID coreapplication.ID,
	name string,
	role charm.RelationRole,
	relInterface string) corerelation.EndpointUUID {

	return s.addApplicationEndpointFromRelation(c, appUUID, charm.Relation{
		Name:      name,
		Role:      role,
		Interface: relInterface,
		Scope:     charm.ScopeGlobal,
	})
}

// addApplicationEndpointFromRelation creates and associates a new application
// endpoint based on the provided relation.
func (s *addRelationSuite) addApplicationEndpointFromRelation(c *gc.C,
	appUUID coreapplication.ID,
	relation charm.Relation) corerelation.EndpointUUID {

	// Generate and get required UUIDs
	charmUUID := s.charmByApp[appUUID]
	// todo(gfouillet) introduce proper generation for this uuid
	charmRelationUUID := uuid.MustNewUUID()
	relationEndpointUUID := corerelationtesting.GenEndpointUUID(c)

	// Add relation to charm
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, name, interface, capacity, role_id,  scope_id)
SELECT ?, ?, ?, ?, ?, crr.id, crs.id
FROM charm_relation_scope crs
JOIN charm_relation_role crr ON crr.name = ?
WHERE crs.name = ?
`, charmRelationUUID.String(), charmUUID.String(), relation.Name,
		relation.Interface, relation.Limit, relation.Role, relation.Scope)

	// application endpoint
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?,?,?,?)
`, relationEndpointUUID.String(), appUUID.String(), charmRelationUUID.String(), network.AlphaSpaceId)

	return relationEndpointUUID
}

type relationSuite struct {
	baseRelationSuite

	fakeCharmUUID1                corecharm.ID
	fakeCharmUUID2                corecharm.ID
	fakeApplicationUUID1          coreapplication.ID
	fakeApplicationUUID2          coreapplication.ID
	fakeApplicationName1          string
	fakeApplicationName2          string
	fakeCharmRelationProvidesUUID string
}

var _ = gc.Suite(&relationSuite{})

func (s *relationSuite) SetUpTest(c *gc.C) {
	s.baseRelationSuite.SetUpTest(c)

	s.fakeApplicationName1 = "fake-application-1"
	s.fakeApplicationName2 = "fake-application-2"

	// Populate DB with one application and charm.
	s.fakeCharmUUID1 = s.addCharm(c)
	s.fakeCharmUUID2 = s.addCharm(c)
	s.fakeCharmRelationProvidesUUID = s.addCharmRelationWithDefaults(c, s.fakeCharmUUID1)
	s.fakeApplicationUUID1 = s.addApplication(c, s.fakeCharmUUID1, s.fakeApplicationName1)
	s.fakeApplicationUUID2 = s.addApplication(c, s.fakeCharmUUID2, s.fakeApplicationName2)
}

func (s *relationSuite) TestGetRelationID(c *gc.C) {
	// Arrange.
	corerelationtesting.GenRelationUUID(c)
	relationID := 1
	relationUUID := s.addRelationWithID(c, relationID)

	// Act.
	id, err := s.state.GetRelationID(context.Background(), relationUUID)

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, gc.Equals, relationID)
}

func (s *relationSuite) TestGetRelationIDNotFound(c *gc.C) {
	// Act.
	_, err := s.state.GetRelationID(context.Background(), "fake-relation-uuid")

	// Assert.
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetRelationUUIDByID(c *gc.C) {
	// Arrange.
	relationID := 1
	relationUUID := s.addRelationWithID(c, relationID)

	// Act.
	uuid, err := s.state.GetRelationUUIDByID(context.Background(), relationID)

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, relationUUID)
}

func (s *relationSuite) TestGetRelationUUIDByIDNotFound(c *gc.C) {
	// Act.
	_, err := s.state.GetRelationUUIDByID(context.Background(), 1)

	// Assert.
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

// TestGetRelationEndpointUUID validates that the correct relation endpoint UUID
// is retrieved for given application and relation ids.
func (s *relationSuite) TestGetRelationEndpointUUID(c *gc.C) {
	// Arrange: create relation endpoint.
	relationUUID := s.addRelation(c)
	applicationEndpointUUID := s.addApplicationEndpoint(c, s.fakeApplicationUUID1,
		s.fakeCharmRelationProvidesUUID)
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID)

	// Act: get the relation endpoint UUID.
	uuid, err := s.state.GetRelationEndpointUUID(context.Background(), relation.GetRelationEndpointUUIDArgs{
		ApplicationID: s.fakeApplicationUUID1,
		RelationUUID:  relationUUID,
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error: %v", errors.ErrorStack(err)))

	// Assert: check the right relation has been fetched.
	c.Check(uuid, gc.Equals, corerelation.EndpointUUID(relationEndpointUUID),
		gc.Commentf("(Assert) wrong relation endpoint uuid"))
}

// TestGetRelationEndpointUUIDRelationNotFound verifies that attempting to retrieve
// a relation endpoint UUID for a nonexistent relation returns RelationNotFound.
func (s *relationSuite) TestGetRelationEndpointUUIDRelationNotFound(c *gc.C) {
	// Arrange: nothing to do, no relations.

	// Act: get a relation.
	_, err := s.state.GetRelationEndpointUUID(context.Background(), relation.GetRelationEndpointUUIDArgs{
		ApplicationID: s.fakeApplicationUUID1,
		RelationUUID:  "not-found-relation-uuid",
	})

	// Assert: check that RelationNotFound is returned.
	c.Check(err, jc.ErrorIs, relationerrors.RelationNotFound, gc.Commentf("(Assert) wrong error: %v", errors.ErrorStack(err)))
}

// TestGetRelationEndpointUUIDApplicationNotFound verifies that attempting to
// fetch a relation endpoint UUID with a non-existent application ID returns
// the ApplicationNotFound error.
func (s *relationSuite) TestGetRelationEndpointUUIDApplicationNotFound(c *gc.C) {
	// Arrange: nothing to do, will fail on application fetch anyway.

	// Act: get a relation.
	_, err := s.state.GetRelationEndpointUUID(context.Background(), relation.GetRelationEndpointUUIDArgs{
		ApplicationID: "not-found-application-uuid ",
		RelationUUID:  "not-used-uuid",
	})

	// Assert: check that ApplicationNotFound is returned.
	c.Check(err, jc.ErrorIs, relationerrors.ApplicationNotFound, gc.Commentf("(Assert) wrong error: %v", errors.ErrorStack(err)))
}

// TestGetRelationEndpointUUIDRelationEndPointNotFound verifies that attempting
// to fetch a relation endpoint UUID for an existing relation without a
// corresponding endpoint returns the RelationEndpointNotFound error.
func (s *relationSuite) TestGetRelationEndpointUUIDRelationEndPointNotFound(c *gc.C) {
	// Arrange: add a relation, but no relation endpoint between apps and relation.
	relationUUID := s.addRelation(c)
	s.addApplicationEndpoint(c, s.fakeApplicationUUID1, s.fakeCharmRelationProvidesUUID)

	// Act: get a relation.
	_, err := s.state.GetRelationEndpointUUID(context.Background(), relation.GetRelationEndpointUUIDArgs{
		ApplicationID: s.fakeApplicationUUID1,
		RelationUUID:  relationUUID,
	})

	// Assert: check that ApplicationNotFound is returned.
	c.Check(err, jc.ErrorIs, relationerrors.RelationEndpointNotFound, gc.Commentf("(Assert) wrong error: %v", errors.ErrorStack(err)))
}

func (s *relationSuite) TestGetRelationEndpoints(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.

	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	// Act: Get relation endpoints.
	endpoints, err := s.state.GetRelationEndpoints(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endpoints, gc.HasLen, 2)
	c.Check(endpoints, jc.SameContents, []relation.Endpoint{
		endpoint1,
		endpoint2,
	})
}

func (s *relationSuite) TestGetRelationEndpointsPeer(c *gc.C) {
	// Arrange: Add a single endpoint and relation over it.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name",
			Role:      charm.RolePeer,
			Interface: "self",
			Optional:  true,
			Limit:     1,
			Scope:     charm.ScopeGlobal,
		},
	}

	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Act: Get relation endpoints.
	endpoints, err := s.state.GetRelationEndpoints(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endpoints, gc.HasLen, 1)
	c.Check(endpoints[0], gc.DeepEquals, endpoint1)
}

// TestGetRelationEndpointsTooManyEndpoints checks that GetRelationEndpoints
// errors when it finds more than 2 endpoints in the database. This should never
// happen and indicates that the database has become corrupted.
func (s *relationSuite) TestGetRelationEndpointsTooManyEndpoints(c *gc.C) {
	// Arrange: Add three endpoints and a relation on them (shouldn't be
	// possible outside of tests!).
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint3 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-3",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     11,
			Scope:     charm.ScopeGlobal,
		},
	}

	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	charmRelationUUID3 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint3.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	applicationEndpointUUID3 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID3)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID3)

	// Act: Get relation endpoints.
	_, err := s.state.GetRelationEndpoints(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, gc.ErrorMatches, "internal error: expected 1 or 2 endpoints in relation, got 3")
}

func (s *relationSuite) TestGetRelationEndpointsRelationNotFound(c *gc.C) {
	// Arrange: Create relationUUID.
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act: Get relation endpoints.
	_, err := s.state.GetRelationEndpoints(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetApplicationEndpoints(c *gc.C) {
	// Arrange: Add two endpoints to the same application.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint2.Relation)
	s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID2)

	// Act: Get relation endpoints.
	endpoints, err := s.state.GetApplicationEndpoints(context.Background(), s.fakeApplicationUUID1)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endpoints, jc.SameContents, []relation.Endpoint{endpoint1, endpoint2})
}

func (s *relationSuite) TestGetApplicationEndpointsEmptySlice(c *gc.C) {
	// Act: Get relation endpoints.
	endpoints, err := s.state.GetApplicationEndpoints(context.Background(), s.fakeApplicationUUID1)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endpoints, gc.HasLen, 0)
}

func (s *relationSuite) TestGetRegularRelationUUIDByEndpointIdentifiers(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	endpoint2 := relation.Endpoint{

		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	expectedRelationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, expectedRelationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, expectedRelationUUID, applicationEndpointUUID2)

	// Act: Get relation UUID from endpoints.
	uuid, err := s.state.GetRegularRelationUUIDByEndpointIdentifiers(
		context.Background(),
		corerelation.EndpointIdentifier{
			ApplicationName: endpoint1.ApplicationName,
			EndpointName:    endpoint1.Name,
		},
		corerelation.EndpointIdentifier{
			ApplicationName: endpoint2.ApplicationName,
			EndpointName:    endpoint2.Name,
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) unexpected error: %s", errors.ErrorStack(err)))
	c.Assert(uuid, gc.Equals, expectedRelationUUID)
}

// TestGetRegularRelationUUIDByEndpointIdentifiersRelationNotFoundPeerRelation
// checks that the function returns not found if only one of the endpoints
// exists (i.e. it is a peer relation).
func (s *relationSuite) TestGetRegularRelationUUIDByEndpointIdentifiersRelationNotFoundPeerRelation(c *gc.C) {
	// Arrange: Add an endpoint and a peer relation on it.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	expectedRelationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, expectedRelationUUID, applicationEndpointUUID1)

	// Act: Try and get relation UUID from endpoints.
	_, err := s.state.GetRegularRelationUUIDByEndpointIdentifiers(
		context.Background(),
		corerelation.EndpointIdentifier{
			ApplicationName: endpoint1.ApplicationName,
			EndpointName:    endpoint1.Name,
		},
		corerelation.EndpointIdentifier{
			ApplicationName: "fake-application-2",
			EndpointName:    "fake-endpoint-name-2",
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetRegularRelationUUIDByEndpointIdentifiersRelationNotFound(c *gc.C) {
	// Act: Try and get relation UUID from endpoints.
	_, err := s.state.GetRegularRelationUUIDByEndpointIdentifiers(
		context.Background(),
		corerelation.EndpointIdentifier{
			ApplicationName: "fake-application-1",
			EndpointName:    "fake-endpoint-name-1",
		},
		corerelation.EndpointIdentifier{
			ApplicationName: "fake-application-2",
			EndpointName:    "fake-endpoint-name-2",
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetPeerRelationUUIDByEndpointIdentifiers(c *gc.C) {
	// Arrange: Add an endpoint and a peer relation on it.

	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	expectedRelationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, expectedRelationUUID, applicationEndpointUUID1)

	// Act: Get relation UUID from endpoint.
	_, err := s.state.GetPeerRelationUUIDByEndpointIdentifiers(
		context.Background(),
		corerelation.EndpointIdentifier{
			ApplicationName: endpoint1.ApplicationName,
			EndpointName:    endpoint1.Name,
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

// TestGetPeerRelationUUIDByEndpointIdentifiersRelationNotFoundRegularRelation
// checks that the function returns not found if the endpoint is part of a
// regular relation, not a peer relation.
func (s *relationSuite) TestGetPeerRelationUUIDByEndpointIdentifiersRelationNotFoundRegularRelation(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.

	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	expectedRelationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, expectedRelationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, expectedRelationUUID, applicationEndpointUUID2)

	// Act: Try and get relation UUID from endpoint.
	_, err := s.state.GetPeerRelationUUIDByEndpointIdentifiers(
		context.Background(),
		corerelation.EndpointIdentifier{
			ApplicationName: endpoint1.ApplicationName,
			EndpointName:    endpoint1.Name,
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetPeerRelationUUIDByEndpointIdentifiersNotFound(c *gc.C) {
	// Act: Try and get relation UUID from endpoint.
	_, err := s.state.GetPeerRelationUUIDByEndpointIdentifiers(
		context.Background(),
		corerelation.EndpointIdentifier{
			ApplicationName: "fake-application-1",
			EndpointName:    "fake-endpoint-name-1",
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetRelationsStatusForUnit(c *gc.C) {
	// Arrange: Add a relation with two endpoints.

	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:  "fake-endpoint-name-1",
			Role:  charm.RoleProvider,
			Scope: charm.ScopeGlobal,
		},
	}

	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:  "fake-endpoint-name-2",
			Role:  charm.RoleRequirer,
			Scope: charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	// Arrange: Add a unit.
	unitUUID := s.addUnit(c, "unit-name", s.fakeApplicationUUID1, s.fakeCharmUUID1)

	// Arrange: Add unit to relation and set relation status.
	s.addRelationUnit(c, unitUUID, relationEndpointUUID1)
	s.setRelationStatus(c, relationUUID, corestatus.Suspended, time.Now())

	expectedResults := []relation.RelationUnitStatusResult{{
		Endpoints: []relation.Endpoint{endpoint1, endpoint2},
		InScope:   true,
		Suspended: true,
	}}

	// Act: Get relation status for unit.
	results, err := s.state.GetRelationsStatusForUnit(context.Background(), unitUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert): %v",
		errors.ErrorStack(err)))
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].InScope, gc.Equals, expectedResults[0].InScope)
	c.Check(results[0].Suspended, gc.Equals, expectedResults[0].Suspended)
	c.Check(results[0].Endpoints, jc.SameContents, expectedResults[0].Endpoints)
}

// TestGetRelationsStatusForUnit checks that GetRelationStatusesForUnit works
// well with peer relations.
func (s *relationSuite) TestGetRelationsStatusForUnitPeer(c *gc.C) {
	// Arrange: Add two peer relations with one endpoint each.

	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:  "fake-endpoint-name-1",
			Role:  charm.RolePeer,
			Scope: charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID1 := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID1, applicationEndpointUUID1)

	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:  "fake-endpoint-name-2",
			Role:  charm.RolePeer,
			Scope: charm.ScopeGlobal,
		},
	}
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint2.Relation)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID2)
	relationUUID2 := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID2, applicationEndpointUUID2)

	// Arrange: Add a unit.
	unitUUID := s.addUnit(c, "unit-name", s.fakeApplicationUUID1, s.fakeCharmUUID1)

	// Arrange: Add unit to both the relation and set their status.
	s.addRelationUnit(c, unitUUID, relationEndpointUUID1)
	s.setRelationStatus(c, relationUUID1, corestatus.Joined, time.Now())
	s.setRelationStatus(c, relationUUID2, corestatus.Suspended, time.Now())

	expectedResults := []relation.RelationUnitStatusResult{{
		Endpoints: []relation.Endpoint{endpoint1},
		InScope:   true,
		Suspended: false,
	}, {
		Endpoints: []relation.Endpoint{endpoint2},
		InScope:   false,
		Suspended: true,
	}}

	// Act: Get relation status for unit.
	results, err := s.state.GetRelationsStatusForUnit(context.Background(), unitUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert): %v",
		errors.ErrorStack(err)))
	c.Assert(results, jc.SameContents, expectedResults)
}

// TestGetRelationStatusesForUnitEmptyResult checks that an empty slice is
// returned when a unit is in no relations.
func (s *relationSuite) TestGetRelationsStatusForUnitEmptyResult(c *gc.C) {
	// Act: Get relation endpoints.
	results, err := s.state.GetRelationsStatusForUnit(context.Background(), "fake-unit-uuid")

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("%v", errors.ErrorStack(err)))
	c.Check(results, gc.HasLen, 0)
}

func (s *relationSuite) TestGetRelationDetails(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.
	relationID := 7

	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelationWithLifeAndID(c, corelife.Dying, relationID)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	expectedDetails := relation.RelationDetailsResult{
		Life:      corelife.Dying,
		UUID:      relationUUID,
		ID:        relationID,
		Endpoints: []relation.Endpoint{endpoint1, endpoint2},
	}

	// Act: Get relation details.
	details, err := s.state.GetRelationDetails(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Life, gc.Equals, expectedDetails.Life)
	c.Assert(details.UUID, gc.Equals, expectedDetails.UUID)
	c.Assert(details.ID, gc.Equals, expectedDetails.ID)
	c.Assert(details.Endpoints, jc.SameContents, expectedDetails.Endpoints)
}

func (s *relationSuite) TestGetRelationDetailsNotFound(c *gc.C) {
	// Act: Get relation details.
	_, err := s.state.GetRelationDetails(context.Background(), "unknown-relation-uuid")

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetRelationUnitEndpointName(c *gc.C) {
	// Arrange
	unitName := coreunittesting.GenNewName(c, "app1/0")
	endpoint := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:  "fake-endpoint-name-1",
			Role:  charm.RolePeer,
			Scope: charm.ScopeGlobal,
		},
	}
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, "app1")
	unitUUID := s.addUnit(c, unitName, appUUID, charmUUID)
	charmRelationUUID := s.addCharmRelation(c, charmUUID, endpoint.Relation)
	applicationEndpointUUID := s.addApplicationEndpoint(c, appUUID, charmRelationUUID)
	relationUUID := s.addRelation(c)
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID)

	// Act
	name, err := s.state.GetRelationUnitEndpointName(context.Background(), relationUnitUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, endpoint.Name)
}

func (s *relationSuite) TestGetRelationUnitEndpointNameNotFound(c *gc.C) {
	// Act
	_, err := s.state.GetRelationUnitEndpointName(context.Background(), "unknown-relation-uuid")

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUnitNotFound)
}

func (s *relationSuite) TestGetRelationUnit(c *gc.C) {
	// Arrange: one relation unit
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, "my-app")
	unitUUID := s.addUnit(c, "my-app/0", appUUID, charmUUID)
	relUUID := s.addRelation(c)
	charmRelationUUID := s.addCharmRelation(c, charmUUID, charm.Relation{})
	applicationEndpointUUID := s.addApplicationEndpoint(c, appUUID, charmRelationUUID)
	relEndpointUUID := s.addRelationEndpoint(c, relUUID, applicationEndpointUUID)
	relUnitUUID := s.addRelationUnit(c, unitUUID, relEndpointUUID)

	// Act
	uuid, err := s.state.GetRelationUnit(context.Background(), relUUID, "my-app/0")

	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))
	c.Assert(uuid, gc.Equals, relUnitUUID)
}

func (s *relationSuite) TestGetRelationUnitNotFound(c *gc.C) {
	// Act
	_, err := s.state.GetRelationUnit(context.Background(), "unknown-relation-uuid", "some-unit-name")

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUnitNotFound)
}

func (s *relationSuite) TestGetAllRelationDetails(c *gc.C) {
	// Arrange: Add three endpoints and two relations on them.
	relationID1 := 7
	relationID2 := 8

	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
		},
	}

	endpoint3 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-3",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
		},
	}

	// This is a lot of code to build two relation:
	// - application-1:endpoint-1 application-2:endpoint-2
	// - application-1:endpoint-1 application-2:endpoint-3
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	charmRelationUUID3 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint3.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	applicationEndpointUUID3 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID3)
	relationUUID1 := s.addRelationWithLifeAndID(c, corelife.Dying, relationID1)
	relationUUID2 := s.addRelationWithLifeAndID(c, corelife.Alive, relationID2)
	s.addRelationEndpoint(c, relationUUID1, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID1, applicationEndpointUUID2)
	s.addRelationEndpoint(c, relationUUID2, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID2, applicationEndpointUUID3)

	expectedDetails := map[int]relation.RelationDetailsResult{
		relationID1: {
			Life:      corelife.Dying,
			UUID:      relationUUID1,
			ID:        relationID1,
			Endpoints: []relation.Endpoint{endpoint1, endpoint2},
		},
		relationID2: {
			Life:      corelife.Alive,
			UUID:      relationUUID2,
			ID:        relationID2,
			Endpoints: []relation.Endpoint{endpoint1, endpoint3},
		},
	}

	// Act: Get relation details.
	details, err := s.state.GetAllRelationDetails(context.Background())

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.HasLen, 2)
	detailsByRelationID := make(map[int]relation.RelationDetailsResult)
	for _, detail := range details {
		detailsByRelationID[detail.ID] = detail
	}
	// First relation
	c.Check(detailsByRelationID[relationID1].Life, gc.Equals, expectedDetails[relationID1].Life)
	c.Check(detailsByRelationID[relationID1].UUID, gc.Equals, expectedDetails[relationID1].UUID)
	c.Check(detailsByRelationID[relationID1].ID, gc.Equals, expectedDetails[relationID1].ID)
	c.Check(detailsByRelationID[relationID1].Endpoints, jc.SameContents, expectedDetails[relationID1].Endpoints)
	// Second relation
	c.Check(detailsByRelationID[relationID2].Life, gc.Equals, expectedDetails[relationID2].Life)
	c.Check(detailsByRelationID[relationID2].UUID, gc.Equals, expectedDetails[relationID2].UUID)
	c.Check(detailsByRelationID[relationID2].ID, gc.Equals, expectedDetails[relationID2].ID)
	c.Check(detailsByRelationID[relationID2].Endpoints, jc.SameContents, expectedDetails[relationID2].Endpoints)
}

func (s *relationSuite) TestGetAllRelationDetailsNone(c *gc.C) {
	// Act: Get relation details.
	result, err := s.state.GetAllRelationDetails(context.Background())

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 0)
}

func (s *relationSuite) TestGetApplicationRelations(c *gc.C) {
	// Arrange: one application with few relations (2 app endpoint, 3 relations)
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, "my-app")
	relationUUID1 := s.addRelationWithID(c, 1)
	relationUUID2 := s.addRelationWithID(c, 2)
	relationUUID3 := s.addRelationWithID(c, 3)
	charmRelationUUID1 := s.addCharmRelation(c, charmUUID, charm.Relation{
		Name:  "fake-endpoint-name-1",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal})
	charmRelationUUID2 := s.addCharmRelation(c, charmUUID, charm.Relation{
		Name:  "fake-endpoint-name-2",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal})
	appEndpointUUID1 := s.addApplicationEndpoint(c, appUUID, charmRelationUUID1)
	appEndpointUUID2 := s.addApplicationEndpoint(c, appUUID, charmRelationUUID2)
	s.addRelationEndpoint(c, relationUUID1, appEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID2, appEndpointUUID2)
	s.addRelationEndpoint(c, relationUUID3, appEndpointUUID1)

	// Act
	relations, err := s.state.GetApplicationRelations(context.Background(), appUUID)

	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))
	c.Assert(relations, jc.SameContents, []corerelation.UUID{
		relationUUID1, // not ordered
		relationUUID2,
		relationUUID3,
	})
}

func (s *relationSuite) TestGetApplicationRelationsApplicationNotFound(c *gc.C) {
	// Act
	notAnAppUUID := coreapplicationtesting.GenApplicationUUID(c)
	_, err := s.state.GetApplicationRelations(context.Background(), notAnAppUUID)

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationNotFound)
}

func (s *relationSuite) TestGetApplicationRelationsApplicationNoRelation(c *gc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, "app1")

	// Act
	relations, err := s.state.GetApplicationRelations(context.Background(), appUUID)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relations, gc.HasLen, 0)
}

func (s *relationSuite) TestEnterScope(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.addCharmMetadata(c, s.fakeCharmUUID1, false)
	s.addCharmMetadata(c, s.fakeCharmUUID2, false)

	// Arrange: Add two endpoints
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	// Arrange: Add unit to application in the relation.
	unitName := coreunittesting.GenNewName(c, "app1/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	settings := map[string]string{"ingress-address": "x.x.x.x"}

	// Act: Enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName, settings)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	relationUnitUUID := s.getRelationUnitInScope(c, relationUUID, unitUUID)
	c.Check(relationUUID.Validate(), jc.ErrorIsNil)

	obtainedSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Check(obtainedSettings, jc.DeepEquals, settings)

	obtainedHash := s.getRelationUnitSettingsHash(c, relationUnitUUID)
	c.Assert(obtainedHash, gc.Not(gc.Equals), "")
}

// TestEnterScopeIdempotent checks that no error is returned if the unit is
// already in scope.
func (s *relationSuite) TestEnterScopeIdempotent(c *gc.C) {
	// Populate charm metadata with subordinate data.
	s.addCharmMetadata(c, s.fakeCharmUUID1, false)
	s.addCharmMetadata(c, s.fakeCharmUUID2, false)

	// Add two endpoints and a relation on them.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	// Add unit to application in the relation.
	unitName := coreunittesting.GenNewName(c, "app1/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	settings := map[string]string{"ingress-address": "x.x.x.x"}

	// Add relation unit for the unit
	s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName, settings)
	c.Assert(err, jc.ErrorIsNil)

	relationUnitUUID := s.getRelationUnitInScope(c, relationUUID, unitUUID)
	c.Check(relationUUID.Validate(), jc.ErrorIsNil)

	obtainedSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Check(obtainedSettings, jc.DeepEquals, settings)

	obtainedHash := s.getRelationUnitSettingsHash(c, relationUnitUUID)
	c.Assert(obtainedHash, gc.Not(gc.Equals), "")

	// Change the settings.
	newSettings := map[string]string{"ingress-address": "y.y.y.y"}

	// EnterScope a second time, with change settings.
	err = s.state.EnterScope(context.Background(), relationUUID, unitName, newSettings)
	c.Assert(err, jc.ErrorIsNil)

	// Check the same relation unit uuid is found and the settings have
	// changed.
	newRelationUnitUUID := s.getRelationUnitInScope(c, relationUUID, unitUUID)
	if c.Check(newRelationUnitUUID.Validate(), jc.ErrorIsNil) {
		c.Check(newRelationUnitUUID.String(), gc.Equals, relationUnitUUID.String())
	}

	newObtainedSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Check(newObtainedSettings, jc.DeepEquals, newSettings)

	newObtainedHash := s.getRelationUnitSettingsHash(c, relationUnitUUID)
	c.Assert(newObtainedHash, gc.Not(gc.Equals), obtainedHash)
}

// TestEnterScopeSubordinate checks that a subordinate unit can enter scope to
// with its principle application.
func (s *relationSuite) TestEnterScopeSubordinate(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.addCharmMetadata(c, s.fakeCharmUUID1, true)
	s.addCharmMetadata(c, s.fakeCharmUUID2, false)

	// Arrange: Add container scoped endpoints on charm 1 and charm 2.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleRequirer,
			Interface: "ntp",
			Scope:     charm.ScopeContainer,
		},
	}
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "ntp",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)

	// Arrange: Add a unit to application 1 and application 2, and make the unit
	// of application 1 a subordinate to the unit of application 2.
	unitName1 := coreunittesting.GenNewName(c, "app1/0")
	unitUUID1 := s.addUnit(c, unitName1, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	unitName2 := coreunittesting.GenNewName(c, "app2/0")
	unitUUID2 := s.addUnit(c, unitName2, s.fakeApplicationUUID2, s.fakeCharmUUID2)
	s.setUnitSubordinate(c, unitUUID1, unitUUID2)

	// Add a relation between application 1 and application 2.
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	// Act: Try and enter scope with the unit 1, which is a subordinate to an
	// application not in the relation.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName1, map[string]string{})

	// Assert:
	c.Assert(err, jc.ErrorIsNil)

	// Assert: relation unit is in scope:
	relationUnitUUID := s.getRelationUnitInScope(c, relationUUID, unitUUID1)
	c.Check(relationUnitUUID.Validate(), jc.ErrorIsNil)
}

// TestEnterScopePotentialRelationUnitNotValidSubordinate checks the right error
// is returned if the unit is a subordinate of an application that is not in the
// relation.
func (s *relationSuite) TestEnterScopePotentialRelationUnitNotValidSubordinate(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.addCharmMetadata(c, s.fakeCharmUUID1, true)
	s.addCharmMetadata(c, s.fakeCharmUUID2, false)

	// Arrange: Add container scoped endpoints on charm 1 and charm 2.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleRequirer,
			Interface: "ntp",
			Scope:     charm.ScopeContainer,
		},
	}
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "ntp",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)

	// Arrange: Add a unit to application 1 and application 2,
	unitName1 := coreunittesting.GenNewName(c, "app1/0")
	unitUUID1 := s.addUnit(c, unitName1, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	unitName2 := coreunittesting.GenNewName(c, "app2/0")
	unitUUID2 := s.addUnit(c, unitName2, s.fakeApplicationUUID2, s.fakeCharmUUID2)

	// Arrange: Make the unit of application 1 a subordinate to the unit of
	// application 2.
	s.setUnitSubordinate(c, unitUUID1, unitUUID2)

	// Arrange: Add application 3 which is an instance of charm 2, so also
	// a principle,
	applicationName3 := "application-name-3"
	applicationUUID3 := s.addApplication(c, s.fakeCharmUUID2, applicationName3)

	// Arrange: Enter application 3 into a relation with the subordinate
	// application (application 1).
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, applicationUUID3, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	// Act: Try and enter scope with the unit 1 of application 1, which is a
	// subordinate to an application not in the relation (application 2).
	err := s.state.EnterScope(context.Background(), relationUUID, unitName1, map[string]string{})

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.PotentialRelationUnitNotValid)
}

// TestEnterScopePotentialRelationUnitNotValid checks that the correct error
// is returned when the unit specified is not a unit of the application in the
// relation.
func (s *relationSuite) TestEnterScopePotentialRelationUnitNotValid(c *gc.C) {
	// Arrange: Add a peer relation on application 1.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RolePeer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add unit to application not in the relation.
	unitName := coreunittesting.GenNewName(c, "app2/0")
	s.addUnit(c, unitName, s.fakeApplicationUUID2, s.fakeCharmUUID2)

	// Act: Enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName, map[string]string{})

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.UnitNotInRelation)
}

func (s *relationSuite) TestEnterScopeRelationNotAlive(c *gc.C) {
	// Arrange: Add two endpoints and a relation
	endpoint1 := relation.Endpoint{
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelationWithLifeAndID(c, corelife.Dying, 17)

	// Arrange: Add unit to application in the relation.
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)

	// Act: Enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName, map[string]string{})

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotAlive)
}

func (s *relationSuite) TestEnterScopeUnitNotAlive(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.
	endpoint1 := relation.Endpoint{
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)

	// Arrange: Add unit to application in the relation.
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.addUnitWithLife(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1, corelife.Dead)

	// Act: Enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName, map[string]string{})

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.UnitNotAlive)
}

func (s *relationSuite) TestEnterScopeRelationNotFound(c *gc.C) {
	// Arrange: Add unit to application in the relation.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)

	// Act: Try and enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName, map[string]string{})

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestEnterScopeUnitNotFound(c *gc.C) {
	relationUUID := corerelationtesting.GenRelationUUID(c)
	// Act: Try and enter scope.
	err := s.state.EnterScope(
		context.Background(),
		relationUUID,
		coreunittesting.GenNewName(c, "app1/0"),
		map[string]string{},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.UnitNotFound)
}

func (s *relationSuite) TestLeaveScope(c *gc.C) {
	// Arrange: Add two endpoints.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)

	// Arrange: Add a relation.
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	// Arrange: Add a unit.
	unitName := coreunittesting.GenNewName(c, "app1/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)

	// Arrange: Add a relation unit.
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Arrange: Add some relation unit settings.
	s.addRelationUnitSetting(c, relationUnitUUID, "test-key", "test-value")
	s.addRelationUnitSettingsHash(c, relationUnitUUID, "hash")

	// Act: Leave scope with the first unit.
	err := s.state.LeaveScope(context.Background(), relationUnitUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	// Assert: check the unit relation has been deleted. This can only be
	// deleted if the unit settings have also been deleted, so no need to check
	// them separately.
	c.Assert(s.doesRelationUnitExist(c, relationUnitUUID), jc.IsFalse)
}

func (s *relationSuite) TestLeaveScopeRelationUnitNotFound(c *gc.C) {
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)

	// Act: Leave scope with the first unit.
	err := s.state.LeaveScope(context.Background(), relationUnitUUID)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUnitNotFound)
}

func (s *relationSuite) TestGetMapperDataForWatchLifeSuspendedStatus(c *gc.C) {
	// Arrange: add a relation with a single endpoint which is suspended
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)
	s.setRelationStatus(c, relationUUID, corestatus.Suspended, time.Now())

	// Act:
	result, err := s.state.GetMapperDataForWatchLifeSuspendedStatus(
		context.Background(),
		relationUUID,
		s.fakeApplicationUUID1,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Life, jc.DeepEquals, corelife.Alive)
	c.Check(result.Suspended, jc.IsTrue)
	c.Check(result.EndpointIdentifiers, jc.SameContents, []corerelation.EndpointIdentifier{
		endpoint1.EndpointIdentifier(),
		endpoint2.EndpointIdentifier(),
	})
}

func (s *relationSuite) TestGetMapperDataForWatchLifeSuspendedStatusWrongApp(c *gc.C) {
	// Arrange: add a relation with a single endpoint. Make the
	// call to GetMapperDataForWatchLifeSuspendedStatus with a different
	// application.
	relationUUID := s.addRelation(c)
	applicationEndpointUUID := s.addApplicationEndpoint(c, s.fakeApplicationUUID1,
		s.fakeCharmRelationProvidesUUID)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID)

	// Act:
	_, err := s.state.GetMapperDataForWatchLifeSuspendedStatus(
		context.Background(),
		relationUUID,
		coreapplicationtesting.GenApplicationUUID(c),
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationNotFoundForRelation)
}

func (s *relationSuite) TestGetOtherRelatedEndpointApplicationData(c *gc.C) {
	// Arrange:
	endpoint1 := relation.Endpoint{
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
		},
	}
	s.addCharmMetadata(c, s.fakeCharmUUID1, true)
	s.addCharmMetadata(c, s.fakeCharmUUID2, false)
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	// Act:
	result, err := s.state.GetOtherRelatedEndpointApplicationData(
		context.Background(),
		relationUUID,
		s.fakeApplicationUUID1,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, relation.OtherApplicationForWatcher{
		ApplicationID: s.fakeApplicationUUID2,
		Subordinate:   false,
	})
}

func (s *relationSuite) TestGetRelationEndpointScope(c *gc.C) {
	// Arrange:
	endpoint1 := relation.Endpoint{
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeContainer,
		},
	}

	s.addCharmMetadata(c, s.fakeCharmUUID1, false)
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Act:
	obtainedScope, err := s.state.GetRelationEndpointScope(context.Background(),
		relationUUID, s.fakeApplicationUUID1)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedScope, gc.Equals, charm.ScopeContainer)
}

func (s *relationSuite) TestGetRelationEndpointScopeRelationNotFound(c *gc.C) {
	// Arrange:
	applicationUUID := coreapplicationtesting.GenApplicationUUID(c)
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act:
	_, err := s.state.GetRelationEndpointScope(context.Background(),
		relationUUID, applicationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetRelationApplicationSettings(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add application settings.
	expectedSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	for k, v := range expectedSettings {
		s.addRelationApplicationSetting(c, relationEndpointUUID1, k, v)
	}

	// Act:
	settings, err := s.state.GetRelationApplicationSettings(
		context.Background(),
		relationUUID,
		s.fakeApplicationUUID1,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, expectedSettings)
}

func (s *relationSuite) TestGetRelationApplicationSettingsEmptyList(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Act:
	settings, err := s.state.GetRelationApplicationSettings(
		context.Background(),
		relationUUID,
		s.fakeApplicationUUID1,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.HasLen, 0)
	c.Assert(settings, gc.NotNil)
}

func (s *relationSuite) TestGetRelationApplicationSettingsRelationNotFound(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act:
	_, err := s.state.GetRelationApplicationSettings(context.Background(),
		relationUUID, s.fakeApplicationUUID1)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetRelationApplicationSettingsApplicationNotFoundForRelation(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	relationUUID := s.addRelation(c)

	// Act:
	_, err := s.state.GetRelationApplicationSettings(context.Background(),
		relationUUID, s.fakeApplicationUUID1)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationNotFoundForRelation)
}

func (s *relationSuite) TestGetRelationUnitChanges(c *gc.C) {

	// Arrange
	// - 1 application with no settings hash => will return a version of 0
	// - 1 application with settings hash => will return a non nil hash
	// - 1 unit with no settings hash => will return a version of 0
	// - 1 unit with settings hash => will return a non nil hash
	// - 1 unit requested but not found => will be added to departed
	charmUUID := s.addCharm(c)
	charmRelationUUID := s.addCharmRelationWithDefaults(c, charmUUID)
	noSettingAppUUID := s.addApplication(c, charmUUID, "noSetting")
	withSettingAppUUID := s.addApplication(c, charmUUID, "withSetting")
	noSettingAppEndpointUUID := s.addApplicationEndpoint(c, noSettingAppUUID, charmRelationUUID)
	withSettingAppEndpointUUID := s.addApplicationEndpoint(c, withSettingAppUUID, charmRelationUUID)
	relationUUID := s.addRelation(c)
	departedUnitUUID := s.addUnit(c, "noSetting/1", noSettingAppUUID, charmUUID)
	noSettingUnitUUID := s.addUnit(c, "noSetting/0", noSettingAppUUID, charmUUID)
	withSettingUnitUUID := s.addUnit(c, "withSetting/0", withSettingAppUUID, charmUUID)
	noSettingRelationEndpointUUID := s.addRelationEndpoint(c, relationUUID, noSettingAppEndpointUUID)
	withSettingRelationEndpointUUID := s.addRelationEndpoint(c, relationUUID, withSettingAppEndpointUUID)
	s.addRelationUnit(c, noSettingUnitUUID, noSettingRelationEndpointUUID)
	relUnitUUID := s.addRelationUnit(c, withSettingUnitUUID, withSettingRelationEndpointUUID)
	s.addRelationUnitSettingsHash(c, relUnitUUID, "42")
	s.addRelationApplicationSettingsHash(c, withSettingRelationEndpointUUID, "84")

	db, err := s.state.DB()
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) cannot get the DB: %s", errors.ErrorStack(err)))

	// Act
	var changes relation.RelationUnitsChange
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		changes, err = s.state.GetRelationUnitChanges(ctx,
			[]coreunit.UUID{noSettingUnitUUID, withSettingUnitUUID, departedUnitUUID},
			[]coreapplication.ID{noSettingAppUUID, withSettingAppUUID},
		)
		return err
	})

	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) unexpected error: %s", errors.ErrorStack(err)))
	c.Assert(changes.Changed, jc.DeepEquals, map[coreunit.Name]int64{
		"noSetting/0":   0,
		"withSetting/0": hashToInt("42"),
	})
	c.Assert(changes.AppChanged, jc.DeepEquals, map[string]int64{
		"noSetting":   0,
		"withSetting": hashToInt("84"),
	})
	c.Assert(changes.Departed, jc.SameContents, []coreunit.Name{"noSetting/1"})
}

func (s *relationSuite) TestGetRelationUnitChangesEmptyArgs(c *gc.C) {

	// Arrange
	db, err := s.state.DB()
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) cannot get the DB: %s", errors.ErrorStack(err)))

	// Act
	var changes relation.RelationUnitsChange
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		changes, err = s.state.GetRelationUnitChanges(ctx, nil, nil)
		return err
	})

	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) unexpected error: %s", errors.ErrorStack(err)))
	c.Check(changes, gc.DeepEquals, relation.RelationUnitsChange{
		Changed:    map[coreunit.Name]int64{},
		AppChanged: map[string]int64{},
		Departed:   []coreunit.Name{},
	})
}

func (s *relationSuite) TestSetRelationApplicationSettings(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Declare settings and add initial settings.
	initialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	settingsUpdate := map[string]string{
		"key2": "value22",
		"key3": "",
	}
	expectedSettings := map[string]string{
		"key1": "value1",
		"key2": "value22",
	}
	for k, v := range initialSettings {
		s.addRelationApplicationSetting(c, relationEndpointUUID1, k, v)
	}

	// Act:
	err := s.state.SetRelationApplicationSettings(
		context.Background(),
		relationUUID,
		s.fakeApplicationUUID1,
		settingsUpdate,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Assert(foundSettings, gc.DeepEquals, expectedSettings)
}

func (s *relationSuite) TestSetRelationApplicationSettingsNothingToSet(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Declare settings and add initial settings.
	initialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	settingsUpdate := map[string]string{
		"key2": "",
		"key3": "",
	}
	expectedSettings := map[string]string{
		"key1": "value1",
	}
	for k, v := range initialSettings {
		s.addRelationApplicationSetting(c, relationEndpointUUID1, k, v)
	}

	// Act:
	err := s.state.SetRelationApplicationSettings(
		context.Background(),
		relationUUID,
		s.fakeApplicationUUID1,
		settingsUpdate,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Assert(foundSettings, gc.DeepEquals, expectedSettings)
}

func (s *relationSuite) TestSetRelationApplicationSettingsNothingToUnSet(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Declare settings and add initial settings.
	initialSettings := map[string]string{
		"key1": "value1",
	}
	settingsUpdate := map[string]string{
		"key2": "value2",
		"key3": "value3",
	}
	expectedSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	for k, v := range initialSettings {
		s.addRelationApplicationSetting(c, relationEndpointUUID1, k, v)
	}

	// Act:
	err := s.state.SetRelationApplicationSettings(
		context.Background(),
		relationUUID,
		s.fakeApplicationUUID1,
		settingsUpdate,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Assert(foundSettings, gc.DeepEquals, expectedSettings)
}

func (s *relationSuite) TestSetRelationApplicationSettingsNilMap(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Act:
	err := s.state.SetRelationApplicationSettings(
		context.Background(),
		relationUUID,
		s.fakeApplicationUUID1,
		nil,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Assert(foundSettings, gc.HasLen, 0)
}

// TestSetRelationApplicationSettingsCheckHash checks that the settings hash is
// updated when the settings are updated.
func (s *relationSuite) TestSetRelationApplicationSettingsHashUpdated(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add some initial settings, this will also set the hash.
	initialSettings := map[string]string{
		"key1": "value1",
	}
	err := s.state.SetRelationApplicationSettings(
		context.Background(),
		relationUUID,
		s.fakeApplicationUUID1,
		initialSettings,
	)
	c.Assert(err, jc.ErrorIsNil)

	initialHash := s.getRelationApplicationSettingsHash(c, relationEndpointUUID1)

	// Act:
	err = s.state.SetRelationApplicationSettings(
		context.Background(),
		relationUUID,
		s.fakeApplicationUUID1,
		map[string]string{
			"key1": "value2",
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	// Assert: Check the hash has changed.
	foundHash := s.getRelationApplicationSettingsHash(c, relationEndpointUUID1)
	c.Assert(initialHash, gc.Not(gc.Equals), foundHash)
}

// TestSetRelationApplicationSettingsHashConstant checks that the settings hash
// is stays the same if the update does not actually change the settings.
func (s *relationSuite) TestSetRelationApplicationSettingsHashConstant(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add some initial settings, this will also set the hash.
	settings := map[string]string{
		"key1": "value1",
	}
	err := s.state.SetRelationApplicationSettings(
		context.Background(),
		relationUUID,
		s.fakeApplicationUUID1,
		settings,
	)
	c.Assert(err, jc.ErrorIsNil)

	initialHash := s.getRelationApplicationSettingsHash(c, relationEndpointUUID1)

	// Act:
	err = s.state.SetRelationApplicationSettings(
		context.Background(),
		relationUUID,
		s.fakeApplicationUUID1,
		settings,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	// Assert: Check the hash has changed.
	foundHash := s.getRelationApplicationSettingsHash(c, relationEndpointUUID1)
	c.Assert(initialHash, gc.Equals, foundHash)
}

func (s *relationSuite) TestSetRelationApplicationSettingsApplicationNotFoundInRelation(c *gc.C) {
	// Arrange: Add relation.
	relationUUID := s.addRelation(c)

	// Act:
	err := s.state.SetRelationApplicationSettings(
		context.Background(),
		relationUUID,
		s.fakeApplicationUUID1,
		nil,
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationNotFoundForRelation)
}

func (s *relationSuite) TestSetRelationApplicationSettingsRelationNotFound(c *gc.C) {
	// Act:
	err := s.state.SetRelationApplicationSettings(
		context.Background(),
		"bad-uuid",
		s.fakeApplicationUUID1,
		nil,
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetPrincipalSubordinateApplicationIDs(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	subordinateCharm := s.fakeCharmUUID1
	subordinateAppUUID := s.fakeApplicationUUID1
	principalCharm := s.fakeCharmUUID2
	principalAppUUID := s.fakeApplicationUUID2
	s.addCharmMetadata(c, subordinateCharm, true)
	s.addCharmMetadata(c, principalCharm, false)

	// Arrange: create principal and subordinate units, then link
	subordinateUnitName := coreunittesting.GenNewName(c, "app1/0")
	subordinateUnitUUID := s.addUnit(c, subordinateUnitName, subordinateAppUUID, subordinateCharm)
	principalUnitName := coreunittesting.GenNewName(c, "app2/0")
	principalUnitUUID := s.addUnit(c, principalUnitName, principalAppUUID, principalCharm)
	s.setUnitSubordinate(c, subordinateUnitUUID, principalUnitUUID)

	// Act
	obtainedPrincipal, obtainedSubordinate, err := s.state.GetPrincipalSubordinateApplicationIDs(
		context.Background(), subordinateUnitUUID)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedPrincipal, gc.Equals, principalAppUUID)
	c.Check(obtainedSubordinate, gc.Equals, subordinateAppUUID)
}

func (s *relationSuite) TestGetPrincipalSubordinateApplicationIDsPrincipalOnly(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	principalCharm := s.fakeCharmUUID1
	principalAppUUID := s.fakeApplicationUUID2
	s.addCharmMetadata(c, principalCharm, false)

	// Arrange: create principal unit
	principalUnitName := coreunittesting.GenNewName(c, "app2/0")
	principalUnitUUID := s.addUnit(c, principalUnitName, principalAppUUID, principalCharm)

	// Act
	obtainedPrincipal, obtainedSubordinate, err := s.state.GetPrincipalSubordinateApplicationIDs(
		context.Background(), principalUnitUUID)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedPrincipal, gc.Equals, principalAppUUID)
	c.Check(obtainedSubordinate.String(), gc.Equals, "")
}

func (s *relationSuite) TestGetRelationUnitSettings(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Arrange: Add application settings.
	expectedSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	for k, v := range expectedSettings {
		s.addRelationUnitSetting(c, relationUnitUUID, k, v)
	}

	// Act:
	settings, err := s.state.GetRelationUnitSettings(
		context.Background(),
		relationUnitUUID,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, expectedSettings)
}

func (s *relationSuite) TestGetRelationUnitSettingsEmptyList(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Act:
	settings, err := s.state.GetRelationUnitSettings(
		context.Background(),
		relationUnitUUID,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.HasLen, 0)
	c.Assert(settings, gc.NotNil)
}

func (s *relationSuite) TestGetRelationUnitSettingsRelationUnitNotFound(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)

	// Act:
	_, err := s.state.GetRelationUnitSettings(
		context.Background(),
		relationUnitUUID,
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUnitNotFound)
}

func (s *relationSuite) TestSetRelationUnitSettings(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Arrange: Declare settings and add initial settings.
	initialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	settingsUpdate := map[string]string{
		"key2": "value22",
		"key3": "",
	}
	expectedSettings := map[string]string{
		"key1": "value1",
		"key2": "value22",
	}
	for k, v := range initialSettings {
		s.addRelationUnitSetting(c, relationUnitUUID, k, v)
	}

	// Act:
	err := s.state.SetRelationUnitSettings(
		context.Background(),
		relationUnitUUID,
		settingsUpdate,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Assert(foundSettings, gc.DeepEquals, expectedSettings)
}

func (s *relationSuite) TestSetRelationUnitSettingsNothingToSet(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Arrange: Declare settings and add initial settings.
	initialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	settingsUpdate := map[string]string{
		"key3": "",
	}
	expectedSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	for k, v := range initialSettings {
		s.addRelationUnitSetting(c, relationUnitUUID, k, v)
	}

	// Act:
	err := s.state.SetRelationUnitSettings(
		context.Background(),
		relationUnitUUID,
		settingsUpdate,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Assert(foundSettings, gc.DeepEquals, expectedSettings)
}

func (s *relationSuite) TestSetRelationUnitSettingsNothingToUnset(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Arrange: Declare settings and add initial settings.
	initialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	settingsUpdate := map[string]string{
		"key2": "value22",
		"key3": "value3bis",
	}
	expectedSettings := map[string]string{
		"key1": "value1",
		"key2": "value22",
		"key3": "value3bis",
	}
	for k, v := range initialSettings {
		s.addRelationUnitSetting(c, relationUnitUUID, k, v)
	}

	// Act:
	err := s.state.SetRelationUnitSettings(
		context.Background(),
		relationUnitUUID,
		settingsUpdate,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Assert(foundSettings, gc.DeepEquals, expectedSettings)
}

func (s *relationSuite) TestSetRelationUnitSettingsNilMap(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Act:
	err := s.state.SetRelationUnitSettings(
		context.Background(),
		relationUnitUUID,
		nil,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Assert(foundSettings, gc.HasLen, 0)
}

// TestSetRelationUnitSettingsCheckHash checks that the settings hash is
// updated when the settings are updated.
func (s *relationSuite) TestSetRelationUnitSettingsHashUpdated(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Arrange: Add some initial settings, this will also set the hash.
	initialSettings := map[string]string{
		"key1": "value1",
	}
	err := s.state.SetRelationUnitSettings(
		context.Background(),
		relationUnitUUID,
		initialSettings,
	)
	c.Assert(err, jc.ErrorIsNil)

	initialHash := s.getRelationUnitSettingsHash(c, relationUnitUUID)

	// Act:
	err = s.state.SetRelationUnitSettings(
		context.Background(),
		relationUnitUUID,
		map[string]string{
			"key1": "value2",
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	// Assert: Check the hash has changed.
	foundHash := s.getRelationUnitSettingsHash(c, relationUnitUUID)
	c.Assert(initialHash, gc.Not(gc.Equals), foundHash)
}

// TestSetRelationUnitSettingsHashConstant checks that the settings hash
// is stays the same if the update does not actually change the settings.
func (s *relationSuite) TestSetRelationUnitSettingsHashConstant(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Arrange: Add some initial settings, this will also set the hash.
	settings := map[string]string{
		"key1": "value1",
	}
	err := s.state.SetRelationUnitSettings(
		context.Background(),
		relationUnitUUID,
		settings,
	)
	c.Assert(err, jc.ErrorIsNil)

	initialHash := s.getRelationUnitSettingsHash(c, relationUnitUUID)

	// Act:
	err = s.state.SetRelationUnitSettings(
		context.Background(),
		relationUnitUUID,
		settings,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	// Assert: Check the hash has changed.
	foundHash := s.getRelationUnitSettingsHash(c, relationUnitUUID)
	c.Assert(initialHash, gc.Equals, foundHash)
}

func (s *relationSuite) TestSetRelationUnitSettingsRelationUnitNotFound(c *gc.C) {
	// Act:
	err := s.state.SetRelationUnitSettings(
		context.Background(),
		"bad-uuid",
		nil,
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUnitNotFound)
}

func (s *relationSuite) TestSetRelationApplicationAndUnitSettings(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Declare settings and add initial settings.
	appInitialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	appSettingsUpdate := map[string]string{
		"key2": "value22",
		"key3": "",
	}
	appExpectedSettings := map[string]string{
		"key1": "value1",
		"key2": "value22",
	}
	for k, v := range appInitialSettings {
		s.addRelationApplicationSetting(c, relationEndpointUUID1, k, v)
	}

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	unitInitialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	unitSettingsUpdate := map[string]string{
		"key2": "value22",
		"key3": "",
	}
	unitExpectedSettings := map[string]string{
		"key1": "value1",
		"key2": "value22",
	}
	for k, v := range unitInitialSettings {
		s.addRelationUnitSetting(c, relationUnitUUID, k, v)
	}

	// Act:
	err := s.state.SetRelationApplicationAndUnitSettings(
		context.Background(),
		relationUnitUUID,
		appSettingsUpdate,
		unitSettingsUpdate,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	foundAppSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Assert(foundAppSettings, gc.DeepEquals, appExpectedSettings)
	foundUnitSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Assert(foundUnitSettings, gc.DeepEquals, unitExpectedSettings)
}

func (s *relationSuite) TestSetRelationApplicationAndUnitSettingsNilMap(c *gc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Act:
	err := s.state.SetRelationApplicationAndUnitSettings(
		context.Background(),
		relationUnitUUID,
		nil,
		nil,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Assert(foundSettings, gc.HasLen, 0)
	foundSettings = s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Assert(foundSettings, gc.HasLen, 0)
}

func (s *relationSuite) TestSetRelationApplicationAndUnitSettingsRelationUnitNotFound(c *gc.C) {
	// Act:
	err := s.state.SetRelationApplicationAndUnitSettings(
		context.Background(),
		"bad-uuid",
		nil,
		nil,
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUnitNotFound)
}

// TestApplicationRelationsInfo tests getting ApplicationRelationsInfo for
// an application related to 2 other applications.
func (s *relationSuite) TestApplicationRelationsInfo(c *gc.C) {
	// Arrange: add application endpoints for the 2 default applications.
	appEndpoint1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, s.fakeCharmRelationProvidesUUID)
	charm2RelationUUID := s.addCharmRelationWithDefaults(c, s.fakeCharmUUID2)
	appEndpoint2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charm2RelationUUID)

	// Add a third application with 2 units, this is the one tested.
	charm3 := s.addCharm(c)
	app3 := s.addApplication(c, charm3, "three")
	charm3RelationUUID := s.addCharmRelation(c, charm3, charm.Relation{
		Name:  "relation",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	})
	appEndpoint3 := s.addApplicationEndpoint(c, app3, charm3RelationUUID)
	unit1 := s.addUnit(c, "three/0", app3, charm3)
	unit2 := s.addUnit(c, "three/1", app3, charm3)

	// Relate applications 1 and 3. Both of application 3's units
	// are in scope and have settings.
	relID1 := 3
	relUUID1 := s.addRelationWithID(c, relID1)
	_ = s.addRelationEndpoint(c, relUUID1, appEndpoint1)
	relEndpoint13 := s.addRelationEndpoint(c, relUUID1, appEndpoint3)
	rel1unit1 := s.addRelationUnit(c, unit1, relEndpoint13)
	rel1unit2 := s.addRelationUnit(c, unit2, relEndpoint13)
	s.addRelationUnitSetting(c, rel1unit1, "foo", "bar")
	s.addRelationUnitSetting(c, rel1unit2, "foo", "baz")
	rel13Data := relation.EndpointRelationData{
		RelationID:      3,
		Endpoint:        "relation",
		RelatedEndpoint: "fake-provides",
		ApplicationData: map[string]interface{}{},
		UnitRelationData: map[string]relation.RelationData{
			"three/0": {
				InScope:  true,
				UnitData: map[string]interface{}{"foo": "bar"},
			},
			"three/1": {
				InScope:  true,
				UnitData: map[string]interface{}{"foo": "baz"},
			},
		},
	}

	// Relate applications 2 and 3. Application 3 has settings.
	relID2 := 4
	relUUID2 := s.addRelationWithID(c, relID2)
	_ = s.addRelationEndpoint(c, relUUID2, appEndpoint2)
	relEndpoint23 := s.addRelationEndpoint(c, relUUID2, appEndpoint3)
	s.addRelationApplicationSetting(c, relEndpoint23, "one", "two")
	rel23Data := relation.EndpointRelationData{
		RelationID:      4,
		Endpoint:        "relation",
		RelatedEndpoint: "fake-provides",
		ApplicationData: map[string]interface{}{"one": "two"},
		UnitRelationData: map[string]relation.RelationData{
			"three/0": {InScope: false},
			"three/1": {InScope: false},
		},
	}

	expectedData := []relation.EndpointRelationData{
		rel13Data,
		rel23Data,
	}

	// Act:
	results, err := s.state.ApplicationRelationsInfo(context.Background(), app3)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.HasLen, 2)
	c.Assert(results, jc.SameContents, expectedData)
}

// TestApplicationRelationsInfo tests getting ApplicationRelationsInfo for
// an application with a peer relation.
func (s *relationSuite) TestApplicationRelationsInfoPeerRelation(c *gc.C) {
	// Arrange: add a third application with 2 units, this is the one tested.
	charm3 := s.addCharm(c)
	app3 := s.addApplication(c, charm3, "three")
	charm3RelationUUID := s.addCharmRelation(c, charm3, charm.Relation{
		Name:  "peer-relation",
		Role:  charm.RolePeer,
		Scope: charm.ScopeGlobal,
	})
	appEndpoint3 := s.addApplicationEndpoint(c, app3, charm3RelationUUID)
	unit1 := s.addUnit(c, "three/0", app3, charm3)
	unit2 := s.addUnit(c, "three/1", app3, charm3)

	// Relate applications 1 and 3. Both of application 3's units
	// are in scope and have settings.
	relID3 := 3
	relUUID3 := s.addRelationWithID(c, relID3)
	relEndpoint3 := s.addRelationEndpoint(c, relUUID3, appEndpoint3)
	_ = s.addRelationUnit(c, unit1, relEndpoint3)
	rel1unit2 := s.addRelationUnit(c, unit2, relEndpoint3)
	s.addRelationUnitSetting(c, rel1unit2, "foo", "baz")
	rel3Data := relation.EndpointRelationData{
		RelationID:      3,
		Endpoint:        "peer-relation",
		RelatedEndpoint: "peer-relation",
		ApplicationData: map[string]interface{}{},
		UnitRelationData: map[string]relation.RelationData{
			"three/0": {
				InScope:  true,
				UnitData: map[string]interface{}{},
			},
			"three/1": {
				InScope:  true,
				UnitData: map[string]interface{}{"foo": "baz"},
			},
		},
	}

	expectedData := []relation.EndpointRelationData{
		rel3Data,
	}

	// Act:
	results, err := s.state.ApplicationRelationsInfo(context.Background(), app3)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.SameContents, expectedData)
}

func (s *relationSuite) TestApplicationRelationsInfoNoApp(c *gc.C) {
	// Arrange:
	appID := coreapplicationtesting.GenApplicationUUID(c)

	// Act:
	_, err := s.state.ApplicationRelationsInfo(context.Background(), appID)

	// Assert: fail if the application does not exist.
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationNotFound)
}

func (s *relationSuite) TestApplicationRelationsInfoNoRelations(c *gc.C) {
	// Act:
	_, err := s.state.ApplicationRelationsInfo(context.Background(), s.fakeApplicationUUID1)

	// Assert: do not fail if an application has no relations.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationSuite) TestCreateSubordinateParams(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.addCharmMetadata(c, s.fakeCharmUUID1, false)
	s.addCharmMetadata(c, s.fakeCharmUUID2, true)

	principalApplicationID := s.fakeApplicationUUID1
	subordinateApplicationID := s.fakeApplicationUUID2

	// Arrange: add container scoped relation.
	relationUUID, principalRelationEndpointUUID, _ := s.addContainerScopedRelation(c, principalApplicationID, subordinateApplicationID)

	// Arrange: Add unit to the principal application.
	unitName := coreunittesting.GenNewName(c, "app1/0")
	principalUnitUUID := s.addUnit(c, unitName, principalApplicationID, s.fakeCharmUUID1)

	// Arrange: enter the principal unit into scope.
	s.addRelationUnit(c, principalUnitUUID, principalRelationEndpointUUID)

	// Act:
	subAppID, err := s.state.NeedsSubordinateUnit(context.Background(), relationUUID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subAppID, gc.NotNil)
	c.Check(*subAppID, gc.Equals, subordinateApplicationID)
}

// TestCreateSubordinateParamsGlobalScopedRelation checks that no parameters are
// returned if the relation is globally scoped.
func (s *relationSuite) TestCreateSubordinateParamsGlobalScopedRelation(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.addCharmMetadata(c, s.fakeCharmUUID1, false)
	s.addCharmMetadata(c, s.fakeCharmUUID2, true)

	principalApplicationID := s.fakeApplicationUUID1
	subordinateApplicationID := s.fakeApplicationUUID2

	// Arrange: add container scoped relation.
	relationUUID, principalRelationEndpointUUID, _ := s.addGlobalScopedRelation(c, principalApplicationID, subordinateApplicationID)

	// Arrange: Add unit to the principal application.
	unitName := coreunittesting.GenNewName(c, "app1/0")
	principalUnitUUID := s.addUnit(c, unitName, principalApplicationID, s.fakeCharmUUID1)

	// Arrange: enter the principal unit into scope.
	s.addRelationUnit(c, principalUnitUUID, principalRelationEndpointUUID)

	// Act:
	subAppID, err := s.state.NeedsSubordinateUnit(context.Background(), relationUUID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subAppID, gc.IsNil)
}

// TestCreateSubordinateParamsPeerRelation checks that no parameters are
// returned for a peer relation.
func (s *relationSuite) TestCreateSubordinateParamsPeerRelation(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.addCharmMetadata(c, s.fakeCharmUUID2, true)

	subordinateApplicationID := s.fakeApplicationUUID2

	// Arrange: add container scoped relation.
	relationUUID, relEndpointUUID := s.addPeerRelation(c, s.fakeCharmUUID2, subordinateApplicationID)

	// Arrange: Add unit to the principal application.
	unitName := coreunittesting.GenNewName(c, "app1/0")
	principalUnitUUID := s.addUnit(c, unitName, subordinateApplicationID, s.fakeCharmUUID1)

	// Arrange: enter the principal unit into scope.
	s.addRelationUnit(c, principalUnitUUID, relEndpointUUID)

	// Act:
	subAppID, err := s.state.NeedsSubordinateUnit(context.Background(), relationUUID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subAppID, gc.IsNil)
}

// TestCreateSubordinateParamsAppNotSubordinate checks that no parameters are
// returned if the related app is not a subordinate.
func (s *relationSuite) TestCreateSubordinateParamsAppNotSubordinate(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.addCharmMetadata(c, s.fakeCharmUUID1, false)
	s.addCharmMetadata(c, s.fakeCharmUUID2, false)

	// Arrange: add container scoped relation.
	relationUUID, principalRelationEndpointUUID, _ := s.addContainerScopedRelation(c, s.fakeApplicationUUID1, s.fakeApplicationUUID2)

	// Arrange: Add unit to the principal application.
	unitName := coreunittesting.GenNewName(c, "app1/0")
	principalUnitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)

	// Arrange: enter the principal unit into scope.
	s.addRelationUnit(c, principalUnitUUID, principalRelationEndpointUUID)

	// Act:
	subAppID, err := s.state.NeedsSubordinateUnit(context.Background(), relationUUID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subAppID, gc.IsNil)
}

// TestCreateSubordinateParamsSubordinateAlreadyExists checks that no parameters
// are returned if a subordinate unit already exists
func (s *relationSuite) TestCreateSubordinateParamsSubordinateAlreadyExists(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.addCharmMetadata(c, s.fakeCharmUUID1, false)
	s.addCharmMetadata(c, s.fakeCharmUUID2, true)

	principalApplicationID := s.fakeApplicationUUID1
	subordinateApplicationID := s.fakeApplicationUUID2

	// Arrange: add container scoped relation.
	relationUUID, principalRelationEndpointUUID, _ := s.addContainerScopedRelation(c, principalApplicationID, subordinateApplicationID)

	// Arrange: Add unit to the principal application and enter into scope.
	principalUnitName := coreunittesting.GenNewName(c, "app1/0")
	principalUnitUUID := s.addUnit(c, principalUnitName, principalApplicationID, s.fakeCharmUUID1)
	s.addRelationUnit(c, principalUnitUUID, principalRelationEndpointUUID)

	// Arrange: Add unit to the subordinate application and set its principal unit.
	subordinateUnitName := coreunittesting.GenNewName(c, "app2/0")
	subordinateUnitUUID := s.addUnit(c, subordinateUnitName, subordinateApplicationID, s.fakeCharmUUID2)
	s.addUnitPrincipal(c, principalUnitUUID, subordinateUnitUUID)

	// Act:
	subAppID, err := s.state.NeedsSubordinateUnit(context.Background(), relationUUID, principalUnitName)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subAppID, gc.IsNil)
}

func (s *relationSuite) TestCreateSubordinateParamsRelationNotAlive(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.addCharmMetadata(c, s.fakeCharmUUID1, false)
	s.addCharmMetadata(c, s.fakeCharmUUID2, true)

	principalApplicationID := s.fakeApplicationUUID1
	subordinateApplicationID := s.fakeApplicationUUID2

	// Arrange: add container scoped relation.
	relationUUID, principalRelationEndpointUUID, _ := s.addContainerScopedRelation(c, principalApplicationID, subordinateApplicationID)

	// Arrange: Add unit to the principal application.
	unitName := coreunittesting.GenNewName(c, "app1/0")
	principalUnitUUID := s.addUnit(c, unitName, principalApplicationID, s.fakeCharmUUID1)

	// Arrange: enter the principal unit into scope.
	s.addRelationUnit(c, principalUnitUUID, principalRelationEndpointUUID)

	// Arrange: set relation to dying.
	s.setLife(c, "relation", relationUUID.String(), life.Dying)

	// Act:
	_, err := s.state.NeedsSubordinateUnit(context.Background(), relationUUID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotAlive)
}

func (s *relationSuite) TestCreateSubordinateParamsRelationUnitNotFound(c *gc.C) {
	// Arrange:
	relationUUID := s.addRelation(c)

	// Act:
	_, err := s.state.NeedsSubordinateUnit(context.Background(), relationUUID, "")

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUnitNotFound)
}

func (s *relationSuite) TestCreateSubordinateParamsUnitNotAlive(c *gc.C) {
	// Arrange: Add unit to application in the relation.
	unitName := coreunittesting.GenNewName(c, "app1/0")
	unitUUID := s.addUnitWithLife(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1, corelife.Dying)

	// Arrange: add container scoped relation.
	relationUUID, relEndpointUUID, _ := s.addContainerScopedRelation(c, s.fakeApplicationUUID1, s.fakeApplicationUUID2)

	// Arrange: enter the principal unit into scope.
	s.addRelationUnit(c, unitUUID, relEndpointUUID)

	// Act:
	_, err := s.state.NeedsSubordinateUnit(context.Background(), relationUUID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.UnitNotAlive)
}

func (s *relationSuite) TestGetGoalStateRelationDataForApplication(c *gc.C) {
	// Arrange: add application endpoints for the 2 default applications.
	appEndpoint1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, s.fakeCharmRelationProvidesUUID)
	charm2RelationUUID := s.addCharmRelationWithDefaults(c, s.fakeCharmUUID2)
	appEndpoint2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charm2RelationUUID)

	// Add a third application with 2 units, this is the one tested.
	charm3 := s.addCharm(c)
	appName3 := "three"
	app3 := s.addApplication(c, charm3, appName3)
	relationName3 := "relation"
	charm3RelationUUID := s.addCharmRelation(c, charm3, charm.Relation{
		Name:  relationName3,
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	})
	appEndpoint3 := s.addApplicationEndpoint(c, app3, charm3RelationUUID)

	testTime := time.Now().UTC()

	// Relate applications 1 and 3.
	relID1 := 3
	relUUID1 := s.addRelationWithID(c, relID1)
	s.setRelationStatus(c, relUUID1, corestatus.Joining, testTime)
	_ = s.addRelationEndpoint(c, relUUID1, appEndpoint1)
	_ = s.addRelationEndpoint(c, relUUID1, appEndpoint3)

	// Relate applications 2 and 3.
	relID2 := 4
	relUUID2 := s.addRelationWithID(c, relID2)
	s.setRelationStatus(c, relUUID2, corestatus.Joined, testTime)
	_ = s.addRelationEndpoint(c, relUUID2, appEndpoint2)
	_ = s.addRelationEndpoint(c, relUUID2, appEndpoint3)

	expected := []relation.GoalStateRelationData{
		{
			EndpointIdentifiers: []corerelation.EndpointIdentifier{
				{
					ApplicationName: appName3,
					EndpointName:    relationName3,
					Role:            charm.RoleRequirer,
				}, {
					ApplicationName: s.fakeApplicationName1,
					EndpointName:    "fake-provides",
					Role:            charm.RoleProvider,
				},
			},
			Since:  &testTime,
			Status: corestatus.Joining,
		}, {
			EndpointIdentifiers: []corerelation.EndpointIdentifier{
				{
					ApplicationName: appName3,
					EndpointName:    relationName3,
					Role:            charm.RoleRequirer,
				}, {
					ApplicationName: s.fakeApplicationName2,
					EndpointName:    "fake-provides",
					Role:            charm.RoleProvider,
				},
			},
			Since:  &testTime,
			Status: corestatus.Joined,
		},
	}

	// Act
	obtained, err := s.state.GetGoalStateRelationDataForApplication(context.Background(), app3)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.HasLen, 2)
	c.Assert(obtained, jc.SameContents, expected)
}

func (s *relationSuite) TestGetGoalStateRelationDataForApplicationNoRows(c *gc.C) {
	// Act
	_, err := s.state.GetGoalStateRelationDataForApplication(context.Background(), s.fakeApplicationUUID1)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationSuite) TestGetApplicationIDByName(c *gc.C) {
	obtainedID, err := s.state.GetApplicationIDByName(context.Background(), s.fakeApplicationName1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedID, gc.Equals, s.fakeApplicationUUID1)
}

func (s *relationSuite) TestGetApplicationIDByNameNotFound(c *gc.C) {
	_, err := s.state.GetApplicationIDByName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *relationSuite) TestDeleteImportedRelations(c *gc.C) {
	// Arrange: Add a peer relation with one endpoint.
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Declare settings and add initial settings.
	appInitialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	for k, v := range appInitialSettings {
		s.addRelationApplicationSetting(c, relationEndpointUUID1, k, v)
	}

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	unitInitialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	for k, v := range unitInitialSettings {
		s.addRelationUnitSetting(c, relationUnitUUID, k, v)
	}

	// Act
	err := s.state.DeleteImportedRelations(context.Background())

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	s.checkTableEmpty(c, "relation_unit_uuid", "relation_unit_settings")
	s.checkTableEmpty(c, "relation_unit_uuid", "relation_unit_settings_hash")
	s.checkTableEmpty(c, "uuid", "relation_unit")
	s.checkTableEmpty(c, "relation_endpoint_uuid", "relation_application_settings")
	s.checkTableEmpty(c, "relation_endpoint_uuid", "relation_application_settings_hash")
	s.checkTableEmpty(c, "uuid", "relation_endpoint")
	s.checkTableEmpty(c, "uuid", "relation")
}

func (s *relationSuite) checkTableEmpty(c *gc.C, colName, tableName string) {
	query := fmt.Sprintf(`
SELECT %s
FROM   %s
`, colName, tableName)

	values := []string{}
	_ = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query)

		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				return errors.Capture(err)
			}
			values = append(values, value)
		}
		return nil
	})
	c.Check(values, jc.DeepEquals, []string{}, gc.Commentf("table %q first value: %q", tableName, strings.Join(values, ", ")))
}

// addRelationUnitSetting inserts a relation unit setting into the database
// using the provided relationUnitUUID.
func (s *relationSuite) addRelationUnitSetting(c *gc.C, relationUnitUUID corerelation.UnitUUID, key, value string) {
	s.query(c, `
INSERT INTO relation_unit_setting (relation_unit_uuid, key, value)
VALUES (?,?,?)
`, relationUnitUUID, key, value)
}

// addRelationUnitSettingsHash inserts a relation unit settings hash into the
// database using the provided relationUnitUUID.
func (s *relationSuite) addRelationUnitSettingsHash(c *gc.C, relationUnitUUID corerelation.UnitUUID, hash string) {
	s.query(c, `
INSERT INTO relation_unit_settings_hash (relation_unit_uuid, sha256)
VALUES (?,?)
`, relationUnitUUID, hash)
}

// addRelationApplicationSetting inserts a relation application setting into the database
// using the provided relation and application ID.
func (s *relationSuite) addRelationApplicationSetting(c *gc.C, relationEndpointUUID, key, value string) {
	s.query(c, `
INSERT INTO relation_application_setting (relation_endpoint_uuid, key, value)
VALUES (?,?,?)
`, relationEndpointUUID, key, value)
}

// addRelationApplicationSettingsHash inserts a relation application settings hash into the
// database using the provided relationEndpointUUID.
func (s *relationSuite) addRelationApplicationSettingsHash(c *gc.C, relationEndpointUUID string, hash string) {
	s.query(c, `
INSERT INTO relation_application_settings_hash (relation_endpoint_uuid, sha256)
VALUES (?,?)
`, relationEndpointUUID, hash)
}

// getRelationApplicationSettings gets the relation application settings.
func (s *relationSuite) getRelationApplicationSettings(c *gc.C, relationEndpointUUID string) map[string]string {
	settings := map[string]string{}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT key, value
FROM relation_application_setting 
WHERE relation_endpoint_uuid = ?
`, relationEndpointUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer rows.Close()
		var (
			key, value string
		)
		for rows.Next() {
			if err := rows.Scan(&key, &value); err != nil {
				return errors.Capture(err)
			}
			settings[key] = value
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) getting relation settings: %s",
		errors.ErrorStack(err)))
	return settings
}

func (s *relationSuite) getRelationApplicationSettingsHash(c *gc.C, relationEndpointUUID string) string {
	var hash string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT sha256
FROM   relation_application_settings_hash
WHERE  relation_endpoint_uuid = ?
`, relationEndpointUUID).Scan(&hash)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return hash
}

// getRelationUnitSettings gets the relation application settings.
func (s *relationSuite) getRelationUnitSettings(c *gc.C, relationUnitUUID corerelation.UnitUUID) map[string]string {
	settings := map[string]string{}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT key, value
FROM relation_unit_setting 
WHERE relation_unit_uuid = ?
`, relationUnitUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer rows.Close()
		var (
			key, value string
		)
		for rows.Next() {
			if err := rows.Scan(&key, &value); err != nil {
				return errors.Capture(err)
			}
			settings[key] = value
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) getting relation settings: %s",
		errors.ErrorStack(err)))
	return settings
}

func (s *relationSuite) getRelationUnitSettingsHash(c *gc.C, relationUnitUUID corerelation.UnitUUID) string {
	var hash string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT sha256
FROM   relation_unit_settings_hash
WHERE  relation_unit_uuid = ?
`, relationUnitUUID).Scan(&hash)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return hash
}

// fetchAllRelationStatusesOrderByRelationIDs retrieves all relation statuses
// ordered by their relation IDs.
// It executes a database query within a transaction and returns a slice of
// corestatus.Status objects.
func (s *addRelationSuite) fetchAllRelationStatusesOrderByRelationIDs(c *gc.C) []corestatus.Status {
	var statuses []corestatus.Status
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		query := `
SELECT rst.name
FROM relation r 
JOIN relation_status rs ON r.uuid = rs.relation_uuid
JOIN relation_status_type rst ON rs.relation_status_type_id = rst.id
ORDER BY r.relation_id
`
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return errors.Capture(err)
		}
		defer rows.Close()
		for rows.Next() {
			var status corestatus.Status
			if err := rows.Scan(&status); err != nil {
				return errors.Capture(err)
			}
			statuses = append(statuses, status)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) fetching inserted relation statuses: %s",
		errors.ErrorStack(err)))
	return statuses
}

// fetchAllEndpointUUIDsByRelationIDs retrieves a mapping of relation IDs to their
// associated endpoint UUIDs from the database.
// It executes a query within a transaction to fetch data from the
// `relation_endpoint` and `relation` tables.  The result is returned as a map
// where the key is the relation ID and the value is a slice of EndpointUUIDs.
func (s *addRelationSuite) fetchAllEndpointUUIDsByRelationIDs(c *gc.C) map[int][]corerelation.EndpointUUID {
	epUUIDsByRelID := make(map[int][]corerelation.EndpointUUID)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		query := `
SELECT re.endpoint_uuid, r.relation_id
FROM relation_endpoint re 
JOIN relation r  ON re.relation_uuid = r.uuid
`
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return errors.Capture(err)
		}
		defer rows.Close()
		for rows.Next() {
			var epUUID string
			var relID int
			if err := rows.Scan(&epUUID, &relID); err != nil {
				return errors.Capture(err)
			}
			epUUIDsByRelID[relID] = append(epUUIDsByRelID[relID], corerelation.EndpointUUID(epUUID))
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) fetching inserted relation endpoint: %s", errors.ErrorStack(err)))
	return epUUIDsByRelID
}

func (s *addRelationSuite) fetchRelationUUIDByRelationID(c *gc.C, id uint64) corerelation.UUID {
	var relationUUID corerelation.UUID
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT r.uuid
FROM   relation AS r
WHERE  r.relation_id = ?
`, id).Scan(&relationUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return relationUUID
}

// getRelationUnitInScope verifies that the expected row is populated in
// relation_unit table.
func (s *relationSuite) getRelationUnitInScope(
	c *gc.C,
	relationUUID corerelation.UUID,
	unitUUID coreunit.UUID,
) corerelation.UnitUUID {
	var relationUnitUUID corerelation.UnitUUID
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT ru.uuid
FROM   relation_unit AS ru
JOIN   relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
WHERE  re.relation_uuid = ?
AND    ru.unit_uuid = ?
`, relationUUID, unitUUID).Scan(&relationUnitUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return relationUnitUUID
}

// setRelationStatus inserts a relation status into the relation_status table.
func (s *relationSuite) setRelationStatus(c *gc.C, relationUUID corerelation.UUID, status corestatus.Status, since time.Time) {
	encodedStatus := s.encodeStatusID(status)
	s.query(c, `
INSERT INTO relation_status (relation_uuid, relation_status_type_id, updated_at)
VALUES (?,?,?)
ON CONFLICT (relation_uuid) DO UPDATE SET relation_status_type_id = ?, updated_at = ?
`, relationUUID, encodedStatus, since, encodedStatus, since)
}

// setUnitSubordinate sets unit 1 to be a subordinate of unit 2.
func (s *relationSuite) setUnitSubordinate(c *gc.C, unitUUID1, unitUUID2 coreunit.UUID) {
	s.query(c, `
INSERT INTO unit_principal (unit_uuid, principal_uuid)
VALUES (?,?)
`, unitUUID1, unitUUID2)
}

func (s *relationSuite) doesRelationUnitExist(c *gc.C, relationUnitUUID corerelation.UnitUUID) bool {
	return s.doesUUIDExist(c, "relation_unit", relationUnitUUID.String())
}

func (s *relationSuite) addContainerScopedRelation(c *gc.C, app1ID, app2ID coreapplication.ID) (corerelation.UUID, string, string) {
	// Arrange: Add two endpoints
	endpoint1 := charm.Relation{
		Name:      "fake-endpoint-name-1",
		Role:      charm.RoleProvider,
		Interface: "database",
		Scope:     charm.ScopeContainer,
	}
	endpoint2 := charm.Relation{
		Name:      "fake-endpoint-name-2",
		Role:      charm.RoleRequirer,
		Interface: "database",
		Scope:     charm.ScopeContainer,
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, app1ID, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, app2ID, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	relationEndpointUUID2 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	return relationUUID, relationEndpointUUID1, relationEndpointUUID2
}

func (s *relationSuite) addGlobalScopedRelation(c *gc.C, app1ID, app2ID coreapplication.ID) (corerelation.UUID, string, string) {
	// Arrange: Add two endpoints
	endpoint1 := charm.Relation{
		Name:      "fake-endpoint-name-1",
		Role:      charm.RoleProvider,
		Interface: "database",
		Scope:     charm.ScopeGlobal,
	}
	endpoint2 := charm.Relation{
		Name:      "fake-endpoint-name-2",
		Role:      charm.RoleRequirer,
		Interface: "database",
		Scope:     charm.ScopeGlobal,
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, app1ID, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, app2ID, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	relationEndpointUUID2 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	return relationUUID, relationEndpointUUID1, relationEndpointUUID2
}

func (s *relationSuite) addPeerRelation(c *gc.C, charmUUID corecharm.ID, appUUID coreapplication.ID) (corerelation.UUID, string) {
	endpoint1 := charm.Relation{
		Name:      "fake-endpoint-name-1",
		Role:      charm.RoleProvider,
		Interface: "database",
		Scope:     charm.ScopeContainer,
	}
	charmRelationUUID1 := s.addCharmRelation(c, charmUUID, endpoint1)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, appUUID, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	return relationUUID, relationEndpointUUID
}

func (s *relationSuite) addUnitPrincipal(c *gc.C, principalUnit, subordinateUnit coreunit.UUID) {
	s.query(c, `
INSERT INTO unit_principal (principal_uuid, unit_uuid)
VALUES (?, ?)
`, principalUnit, subordinateUnit)
}
