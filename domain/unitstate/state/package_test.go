// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/model"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
)

type baseSuite struct {
	schematesting.ModelSuite
	state *State

	unitUUID string
	unitName string
}

func (s *baseSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := tc.Must(c, model.NewUUID)
	s.query(c, `
			INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "prod", "iaas", "test-model", "ec2")
		`, modelUUID, coretesting.ControllerTag.Id())

	appState := applicationstate.NewState(s.TxnRunnerFactory(), modelUUID, clock.WallClock, loggertesting.WrapCheckLog(c))

	appArg := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: "app",
				},
				Manifest: charm.Manifest{
					Bases: []charm.Base{{
						Name:          "ubuntu",
						Channel:       charm.Channel{Risk: charm.RiskStable},
						Architectures: []string{"amd64"},
					}},
				},
				ReferenceName: "app",
				Source:        charm.LocalSource,
				Architecture:  architecture.AMD64,
			},
		},
	}

	s.unitName = unittesting.GenNewName(c, "app/0").String()
	unitArgs := []application.AddIAASUnitArg{{}}

	ctx := c.Context()
	_, _, err := appState.CreateIAASApplication(ctx, "app", appArg, unitArgs)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&s.unitUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Cleanup(func() {
		s.state = nil
		s.unitName = ""
		s.unitUUID = ""
	})
}

func (s *baseSuite) addUnitStateCharm(c *tc.C, key any, value string) {
	q := "INSERT INTO unit_state_charm VALUES (?, ?, ?)"
	s.query(c, q, s.unitUUID, key, value)
}

func (s *baseSuite) addCharm(c *tc.C) string {
	charmUUID := tc.Must(c, corecharm.NewID).String()
	s.query(c, `INSERT INTO charm (uuid, reference_name, create_time) VALUES (?, ?, ?)`,
		charmUUID, charmUUID, time.Now())
	return charmUUID
}

func (s *baseSuite) addApplication(c *tc.C, charmUUID, appName, spaceUUID string) string {
	appUUID := tc.Must(c, coreapplication.NewUUID).String()
	s.query(c, `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)`,
		appUUID, appName, life.Alive, charmUUID, spaceUUID)
	return appUUID
}

func (s *baseSuite) checkUnitUUID(c *tc.C, unitUUID string) {
	var uuid string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&uuid)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, unitUUID)
}

// query executes a given SQL query with optional arguments within a
// transactional context using the test database.
func (s *baseSuite) query(c *tc.C, query string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%v: query: %s (args: %v)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}
