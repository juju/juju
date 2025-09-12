// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type applicationRefreshSuite struct {
	baseSuite

	state         *State
	otherAppCount int
}

func TestApplicationRefreshSuite(t *testing.T) {
	tc.Run(t, &applicationRefreshSuite{})
}

func (s *applicationRefreshSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *applicationRefreshSuite) TestSetApplicationCharm(c *tc.C) {
	// Arrange
	appID := s.createApplication(c, createApplicationArgs{
		relations: []charm.Relation{
			{Role: charm.RoleProvider},
			{Role: charm.RoleRequirer},
		},
	})
	charmID := s.createCharm(c, createCharmArgs{
		name: "foo",
	})

	// Act
	err := s.state.SetApplicationCharm(c.Context(), appID, charmID, application.SetCharmParams{})

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	var newCharmUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT charm_uuid FROM application WHERE uuid = ?", appID).Scan(&newCharmUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newCharmUUID, tc.Equals, charmID.String())
}

func (s *applicationRefreshSuite) TestSetApplicationCharmNoApplication(c *tc.C) {
	// Arrange
	appID := applicationtesting.GenApplicationUUID(c)
	charmID := s.createCharm(c, createCharmArgs{name: "foo"})

	// Act
	err := s.state.SetApplicationCharm(c.Context(), appID, charmID, application.SetCharmParams{})

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationRefreshSuite) TestSetApplicationCharmNoCharm(c *tc.C) {
	// Arrange
	appID := s.createApplication(c, createApplicationArgs{appName: "my-app"})
	charmID := charmtesting.GenCharmID(c)

	// Act
	err := s.state.SetApplicationCharm(c.Context(), appID, charmID, application.SetCharmParams{})

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmNotFound)
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
	charmID := s.createCharm(c, createCharmArgs{name: "foo", relations: []charm.Relation{
		{
			Name:      "established",
			Role:      charm.RoleProvider,
			Interface: "limited",
			Limit:     3,
		},
	}})

	// Act
	err := s.state.SetApplicationCharm(c.Context(), appID, charmID, application.SetCharmParams{})

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
	charmID := s.createCharm(c, createCharmArgs{name: "foo"})

	// Act
	err := s.state.SetApplicationCharm(c.Context(), appID, charmID, application.SetCharmParams{})

	// Assert
	c.Assert(err, tc.ErrorMatches, `.*charm has no corresponding relation "established"`)
}

func (s *applicationRefreshSuite) TestSetApplicationCharmErrorWithEstablishedRelationRoleMismatch(c *tc.C) {
	// Arrange
	appID := s.createApplication(c, createApplicationArgs{
		appName: "my-app",
		relations: []charm.Relation{
			{
				Name:      "established",
				Role:      charm.RoleProvider,
				Interface: "interf",
				Limit:     42,
				Scope:     charm.ScopeContainer,
			},
		},
	})
	s.establishRelationWith(c, appID, "established", charm.RoleRequirer)
	charmID := s.createCharm(c, createCharmArgs{name: "foo", relations: []charm.Relation{
		{
			Name:      "established",
			Role:      charm.RoleRequirer,
			Interface: "interf",
			Limit:     42,
			Scope:     charm.ScopeContainer,
		},
	}})

	// Act
	err := s.state.SetApplicationCharm(c.Context(), appID, charmID, application.SetCharmParams{})

	// Assert
	c.Assert(err, tc.ErrorMatches, `.*cannot change role of relation "established" from provider to requirer`)
}

func (s *applicationRefreshSuite) TestSetApplicationCharmErrorWithEstablishedRelationInterfaceMismatch(c *tc.C) {
	// Arrange
	appID := s.createApplication(c, createApplicationArgs{
		appName: "my-app",
		relations: []charm.Relation{
			{
				Name:      "established",
				Role:      charm.RoleProvider,
				Interface: "interf",
				Limit:     42,
				Scope:     charm.ScopeContainer,
			},
		},
	})
	s.establishRelationWith(c, appID, "established", charm.RoleRequirer)
	charmID := s.createCharm(c, createCharmArgs{name: "foo", relations: []charm.Relation{
		{
			Name:      "established",
			Role:      charm.RoleProvider,
			Interface: "not-interf",
			Limit:     42,
			Scope:     charm.ScopeContainer,
		},
	}})

	// Act
	err := s.state.SetApplicationCharm(c.Context(), appID, charmID, application.SetCharmParams{})

	// Assert
	c.Assert(err, tc.ErrorMatches, `.*cannot change interface of relation "established" from interf to not-interf`)
}

func (s *applicationRefreshSuite) TestSetApplicationCharmErrorWithEstablishedRelationScopeMismatch(c *tc.C) {
	// Arrange
	appID := s.createApplication(c, createApplicationArgs{
		appName: "my-app",
		relations: []charm.Relation{
			{
				Name:      "established",
				Role:      charm.RoleProvider,
				Interface: "interf",
				Limit:     42,
				Scope:     charm.ScopeGlobal,
			},
		},
	})
	s.establishRelationWith(c, appID, "established", charm.RoleRequirer)
	charmID := s.createCharm(c, createCharmArgs{name: "foo", relations: []charm.Relation{
		{
			Name:      "established",
			Role:      charm.RoleProvider,
			Interface: "interf",
			Limit:     42,
			Scope:     charm.ScopeContainer,
		},
	}})

	// Act
	err := s.state.SetApplicationCharm(c.Context(), appID, charmID, application.SetCharmParams{})

	// Assert
	c.Assert(err, tc.ErrorMatches, `.*cannot change scope of relation "established" from global to container`)
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
	charmID := s.createCharm(c, createCharmArgs{name: "foo", relations: []charm.Relation{
		{
			Name:      "established",
			Role:      charm.RoleProvider,
			Interface: "limited",
			Limit:     1,
		},
	}})

	// Act
	err := s.state.SetApplicationCharm(c.Context(), appID, charmID, application.SetCharmParams{})

	// Assert
	c.Assert(err, tc.ErrorMatches,
		`.*limit of 1 for "established".*established relations[^0-9]+2[^0-9]+`)
}

func (s *applicationRefreshSuite) TestSetApplicationCharmMergesEndpointBindings(c *tc.C) {
	// Arrange
	appID := s.createApplication(c, createApplicationArgs{
		appName: "my-app",
		relations: []charm.Relation{
			{
				Name:      "established",
				Role:      charm.RoleProvider,
				Interface: "interf",
			},
		},
	})
	charmID := s.createCharm(c, createCharmArgs{name: "foo", relations: []charm.Relation{
		{
			Name:      "established",
			Role:      charm.RoleProvider,
			Interface: "interf",
		},
	}})

	spaceUUID := networktesting.GenSpaceUUID(c)
	spaceName := network.SpaceName("beta")
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO space (uuid, name)
VALUES (?, ?)`, spaceUUID, spaceName)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act
	err = s.state.SetApplicationCharm(c.Context(), appID, charmID, application.SetCharmParams{
		EndpointBindings: map[string]network.SpaceName{
			"established": spaceName,
		},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	bindings, err := s.state.GetApplicationEndpointBindings(c.Context(), appID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bindings["established"], tc.Equals, spaceUUID)
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

	appID, _, err := s.state.CreateIAASApplication(c.Context(), appName, application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform:          platform,
			Charm:             originalCharm,
			CharmDownloadInfo: nil,
			Channel:           channel,
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to create application %q", appName))
	return appID
}

// createCharm creates a mock Charm instance with provided relation metadata
// and returns it along with a cleanup function.
func (s *applicationRefreshSuite) createCharm(c *tc.C, args createCharmArgs) corecharm.ID {
	ch := charm.Charm{
		Metadata: charm.Metadata{
			Name:     args.name,
			Provides: args.relationMap(c, charm.RoleProvider),
			Requires: args.relationMap(c, charm.RoleRequirer),
			Peers:    args.relationMap(c, charm.RolePeer),
		},
		Manifest:      s.minimalManifest(c),
		ReferenceName: args.name,
		Source:        charm.LocalSource,
		Revision:      43,
	}
	charmID, _, err := s.state.AddCharm(c.Context(), ch, nil, false)
	c.Assert(err, tc.ErrorIsNil)
	return charmID
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
			INSERT INTO relation (uuid, life_id, relation_id, scope_id)
			VALUES (?, 0, ?, 0)
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
func (caa createApplicationArgs) relationMap(
	c *tc.C,
	role charm.RelationRole,
) map[string]charm.Relation {
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
	name string
	// relations define the list of relations associated with the application.
	relations []charm.Relation
}

// relationMap processes the relations of a createCharmArgs instance,
// filtering by role and returning a mapped result.
func (cca createCharmArgs) relationMap(
	c *tc.C,
	role charm.RelationRole,
) map[string]charm.Relation {
	result := transform.SliceToMap(cca.relations, func(f charm.Relation) (string, charm.Relation) {
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
