// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/charm"
	corecharmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/relation"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

// baseRelationSuite is a struct embedding ModelSuite for testing relation
// between application. It provides a set of builder function to create all
// the necessary context to actually create relation, like charms and applications
type baseRelationSuite struct {
	schematesting.ModelSuite
	state *State
}

func (s *baseRelationSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

// query executes a given SQL query with optional arguments within a transactional context using the test database.
func (s *baseRelationSuite) query(c *gc.C, query string, args ...any) {

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %s)", err, query, args)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}

// addApplication adds a new application to the database with the specified
// charm UUID and application name.
// It return the application UUID
func (s *baseRelationSuite) addApplication(c *gc.C, charmUUID charm.ID, appName string) coreapplication.ID {
	appUUID := coreapplicationtesting.GenApplicationUUID(c)
	s.query(c, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) 
VALUES (?, ?, ?, ?, ?)
`, appUUID, appName, 0 /* alive */, charmUUID.String(), network.AlphaSpaceId)
	return appUUID
}

// addCharm inserts a new charm into the database and return the UUID.
func (s *baseRelationSuite) addCharm(c *gc.C) charm.ID {
	charmUUID := corecharmtesting.GenCharmID(c)
	// The UUID is also used as the reference_name as there is a unique
	// constraint on the reference_name, revision and source_id.
	s.query(c, `
INSERT INTO charm (uuid, reference_name, architecture_id) 
VALUES (?, ?, 0)
`, charmUUID, charmUUID)
	return charmUUID
}

// setCharmSubordinate updates the charm's metadata to mark it as subordinate,
// or inserts it if not present in the database.
func (s *baseRelationSuite) setCharmSubordinate(c *gc.C, charmUUID charm.ID) {
	s.query(c, `
INSERT INTO charm_metadata (charm_uuid, name, subordinate)
VALUES (?,?,true)
ON CONFLICT DO UPDATE SET subordinate = true
`, charmUUID, charmUUID)
}

// newEndpointIdentifier converts an endpoint string into a relation.EndpointIdentifier and asserts no parsing errors.
func (s *baseRelationSuite) newEndpointIdentifier(c *gc.C, endpoint string) relation.CandidateEndpointIdentifier {
	result, err := relation.NewCandidateEndpointIdentifier(endpoint)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to parse endpoint %q: %v", endpoint,
		errors.ErrorStack(err)))
	return result
}
