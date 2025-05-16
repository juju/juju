// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type applicationRefreshSuite struct {
	baseSuite

	state         *State
	otherAppCount int
}

func TestApplicationRefreshSuite(t *stdtesting.T) { tc.Run(t, &applicationRefreshSuite{}) }
func (s *applicationRefreshSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

// TestSetApplicationCharmNoRelation verifies that an application charm can be
// updated when no active relations exist, even if the new charm has no relation
func (s *applicationRefreshSuite) TestSetApplicationCharmNoRelation(c *tc.C) {
	// Arrange
	appID := s.createApplication(c, createApplicationArgs{
		relations: []charm.Relation{
			{Role: charm.RoleProvider},
			{Role: charm.RoleRequirer},
		},
	})
	newCharm, finish := s.createCharm(c, createCharmArgs{})
	defer finish()

	// Act
	err := s.state.SetApplicationCharm(c.Context(), appID, application.UpdateCharmParams{
		Charm: newCharm,
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetApplicationCharmSuccessWithRelationEstablished verifies that an
// application charm can be updated successfully when an active relation exists
// and the updated charm's relation limit is greater than the currently
// established usages.
func (s *applicationRefreshSuite) TestSetApplicationCharmSuccessWithRelationEstablished(c *tc.C) {
	// Arrange
	appID := s.createApplication(c, createApplicationArgs{
		relations: []charm.Relation{
			{
				Name:      "established",
				Role:      charm.RoleProvider,
				Interface: "limited",
				Limit:     2,
			},
		},
	})
	// establish relation to the max capacity.
	s.establishRelationWith(c, appID, "established", charm.RoleRequirer)
	s.establishRelationWith(c, appID, "established", charm.RoleRequirer)

	// Create a charm with a different limit, but bigger.
	newCharm, finish := s.createCharm(c, createCharmArgs{relations: []internalcharm.Relation{
		{
			Name:      "established",
			Role:      internalcharm.RoleProvider,
			Interface: "limited",
			Limit:     3,
		},
	}})
	defer finish()

	// Act
	err := s.state.SetApplicationCharm(c.Context(), appID, application.UpdateCharmParams{
		Charm: newCharm,
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetApplicationCharmErrorWithRelation verifies that an application charm cannot
// be updated if an established relation is suppressed.
func (s *applicationRefreshSuite) TestSetApplicationCharmErrorWithEstablishedRelationSuppressed(c *tc.C) {
	// Arrange
	appID := s.createApplication(c, createApplicationArgs{
		appName: "my-app",
		relations: []charm.Relation{
			{
				Name:      "established",
				Role:      charm.RoleProvider,
				Interface: "not-implemented",
				Optional:  true,
				Limit:     42,
				Scope:     charm.ScopeContainer,
			},
			{Role: charm.RoleRequirer},
		},
	})
	s.establishRelationWith(c, appID, "established", charm.RoleRequirer)
	newCharm, finish := s.createCharm(c, createCharmArgs{})
	defer finish()

	// Act
	err := s.state.SetApplicationCharm(c.Context(), appID, application.UpdateCharmParams{
		Charm: newCharm,
	})

	// Assert
	c.Assert(err, tc.ErrorMatches, `would break relation my-app:established`)
}

// TestSetApplicationCharmErrorWithEstablishedRelationExceedLimits verifies
// that updating an application charm fails when the new charm's relation
// limit is lower than the number of already established relations.
func (s *applicationRefreshSuite) TestSetApplicationCharmErrorWithEstablishedRelationExceedLimits(c *tc.C) {
	// Arrange
	appID := s.createApplication(c, createApplicationArgs{
		appName: "my-app",
		relations: []charm.Relation{
			{
				Name:      "established",
				Role:      charm.RoleProvider,
				Interface: "limited",
				Limit:     2,
			},
		},
	})
	// establish relation to the max capacity.
	s.establishRelationWith(c, appID, "established", charm.RoleRequirer)
	s.establishRelationWith(c, appID, "established", charm.RoleRequirer)

	// Create a charm with a lesser limit.
	newCharm, finish := s.createCharm(c, createCharmArgs{relations: []internalcharm.Relation{
		{
			Name:      "established",
			Role:      internalcharm.RoleProvider,
			Interface: "limited",
			Limit:     1,
		},
	}})
	defer finish()

	// Act
	err := s.state.SetApplicationCharm(c.Context(), appID, application.UpdateCharmParams{
		Charm: newCharm,
	})

	// Assert
	c.Assert(err, tc.ErrorMatches,
		".*limit of 1 for my-app:established.*established relations[^0-9]+2[^0-9]+")
}

// createApplication creates a new application in the state with the provided arguments and returns its unique ID.
func (s *applicationRefreshSuite) createApplication(c *tc.C, args createApplicationArgs) coreapplication.ID {
	appName := args.appName
	if appName == "" {
		appName = "some-app"
	}

	// Create an application with a charm.
	platform := deployment.Platform{
		Channel:      "22.04/stable",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "stable",
		Branch: "branch",
	}

	originalCharm := charm.Charm{
		Metadata: charm.Metadata{
			Name:     appName,
			Provides: args.relationMap(c, charm.RoleProvider),
			Requires: args.relationMap(c, charm.RoleRequirer),
			Peers:    args.relationMap(c, charm.RolePeer),
		},
		Manifest:      s.minimalManifest(c),
		ReferenceName: appName,
		Source:        charm.LocalSource,
		Revision:      42,
	}

	appID, err := s.state.CreateApplication(c.Context(), appName, application.AddApplicationArg{
		Platform:          platform,
		Charm:             originalCharm,
		CharmDownloadInfo: nil,
		Scale:             1,
		Channel:           channel,
	}, nil)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to create application %q", appName))
	return appID
}

// createCharm creates a mock Charm instance with provided relation metadata
// and returns it along with a cleanup function.
func (s *applicationRefreshSuite) createCharm(c *tc.C, args createCharmArgs) (internalcharm.Charm, func()) {
	ctrl := gomock.NewController(c)

	newCharm := NewMockCharm(ctrl)
	newCharm.EXPECT().Meta().Return(&internalcharm.Meta{
		Provides: args.relationMap(c, internalcharm.RoleProvider),
		Requires: args.relationMap(c, internalcharm.RoleRequirer),
		Peers:    args.relationMap(c, internalcharm.RolePeer),
	})
	return newCharm, ctrl.Finish
}

// establishRelationWith creates a new relation between the current application
// and another, created on the fly, based on the given parameters.
func (s *applicationRefreshSuite) establishRelationWith(c *tc.C, currentAppID coreapplication.ID, relationName string,
	role charm.RelationRole) {
	s.otherAppCount++
	// Create relation metadata based on the role.
	relations := []charm.Relation{
		{
			Name:      relationName,
			Role:      role,
			Interface: "test",
			Scope:     charm.ScopeGlobal,
		},
	}

	// Create application args with the appropriate relation type.
	args := createApplicationArgs{
		appName:   fmt.Sprintf("some-other-app-%d", s.otherAppCount),
		relations: relations,
	}

	// Create the new application.
	newAppID := s.createApplication(c, args)

	// Create a new relation with a generated UUID and link both applications.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Get the application endpoints for both applications
		var origEndpointUUID, newEndpointUUID string

		getEndpointUUIDQuery := `
			SELECT ae.uuid
			FROM application_endpoint AS ae
			JOIN charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid 
			WHERE ae.application_uuid = ?
			AND cr.name = ?
		`
		// Get the endpoint for the original application.
		err := tx.QueryRowContext(ctx, getEndpointUUIDQuery, currentAppID.String(), relationName).Scan(&origEndpointUUID)
		if err != nil {
			return errors.Errorf("getting original endpoint UUID: %w", err)
		}

		// Get the endpoint for the new application.
		err = tx.QueryRowContext(ctx, getEndpointUUIDQuery, newAppID.String(), relationName).Scan(&newEndpointUUID)
		if err != nil {
			return errors.Errorf("getting new endpoint UUID: %w", err)
		}

		// Generate a required uuids.
		relUUID := uuid.MustNewUUID().String()
		endpointUUID1 := uuid.MustNewUUID().String()
		endpointUUID2 := uuid.MustNewUUID().String()

		// Insert the relation.
		_, err = tx.ExecContext(ctx, `
			INSERT INTO relation (uuid, life_id, relation_id)
			VALUES (?, 0, ?)
		`, relUUID, s.otherAppCount)
		if err != nil {
			return errors.Errorf("inserting relation: %w", err)
		}

		// Insert relation endpoints for both applications.
		insertRelationEndpointQuery := `
			INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
			VALUES (?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertRelationEndpointQuery, endpointUUID1, relUUID, origEndpointUUID)
		if err != nil {
			return errors.Errorf("inserting first relation endpoint: %w", err)
		}
		_, err = tx.ExecContext(ctx, insertRelationEndpointQuery, endpointUUID2, relUUID, newEndpointUUID)
		if err != nil {
			return errors.Errorf("inserting second relation endpoint: %w", err)
		}

		return nil
	})

	c.Assert(err, tc.ErrorIsNil)
}

// createApplicationArgs represents the arguments required to create a
// new application.
type createApplicationArgs struct {
	// appName specifies the name of the application.
	appName string
	// relations define the list of relations associated with the application.
	relations []charm.Relation
}

// relationMap processes the relations of a createApplicationArgs instance,
// filtering by role and returning a mapped result.
func (caa createApplicationArgs) relationMap(c *tc.C,
	role charm.RelationRole) map[string]charm.Relation {
	result := transform.SliceToMap(caa.relations, func(f charm.Relation) (string, charm.Relation) {
		c.Assert(f.Role, tc.Not(tc.Equals), "", tc.Commentf("(Arrange) relation role must not be empty"))
		if f.Role != role {
			return "", charm.Relation{}
		}
		name := f.Name
		if f.Scope == "" {
			f.Scope = charm.ScopeGlobal
		}
		if name == "" {
			name = fmt.Sprintf("rel-%s-%s", f.Role, f.Scope)
		}
		if f.Interface == "" {
			f.Interface = "test"
		}

		return name, charm.Relation{
			Name:      name,
			Role:      f.Role,
			Interface: f.Interface,
			Optional:  f.Optional,
			Limit:     f.Limit,
			Scope:     f.Scope,
		}
	})
	delete(result, "")
	return result
}

// createCharmArgs holds the arguments required for creating a charm in tests, including its relations.
type createCharmArgs struct {
	// relations define the list of relations associated with the application.
	relations []internalcharm.Relation
}

// relationMap processes the relations of a createCharmArgs instance,
// filtering by role and returning a mapped result.
func (cca createCharmArgs) relationMap(c *tc.C,
	role internalcharm.RelationRole) map[string]internalcharm.Relation {
	result := transform.SliceToMap(cca.relations, func(f internalcharm.Relation) (string, internalcharm.Relation) {
		c.Assert(f.Role, tc.Not(tc.Equals), "", tc.Commentf("(Arrange) relation role must not be empty"))
		if f.Role != role {
			return "", internalcharm.Relation{}
		}
		name := f.Name
		if f.Scope == "" {
			f.Scope = internalcharm.ScopeGlobal
		}
		if name == "" {
			name = fmt.Sprintf("rel-%s-%s", f.Role, f.Scope)
		}
		if f.Interface == "" {
			f.Interface = "test"
		}

		return name, internalcharm.Relation{
			Name:      name,
			Role:      f.Role,
			Interface: f.Interface,
			Optional:  f.Optional,
			Limit:     f.Limit,
			Scope:     f.Scope,
		}
	})
	delete(result, "")
	return result
}
