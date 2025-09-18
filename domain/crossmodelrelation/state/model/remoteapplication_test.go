// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	internalerrors "github.com/juju/juju/internal/errors"
)

type modelRemoteApplicationSuite struct {
	baseSuite
}

func TestModelRemoteApplicationSuite(t *testing.T) {
	tc.Run(t, &modelRemoteApplicationSuite{})
}

func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationOffererInsertsApplicationAndCharm(c *tc.C) {
	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote offerer application",
			Provides: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleProvider,
					Interface: "db",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{
				"cache": {
					Name:      "cache",
					Role:      charm.RoleRequirer,
					Interface: "cacher",
					Scope:     charm.ScopeContainer,
				},
			},
			Peers: map[string]charm.Relation{},
		},
	}
	err := s.state.AddRemoteApplicationOfferer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		Charm: charm,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplication(c, "foo")
	s.assertCharmMetadata(c, "foo", charm)
}

func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationOffererInsertsApplicationAndCharmWithNoRelations(c *tc.C) {
	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote offerer application",
		},
	}
	err := s.state.AddRemoteApplicationOfferer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		Charm: charm,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplication(c, "foo")
	s.assertCharmMetadata(c, "foo", charm)
}

func (s *modelRemoteApplicationSuite) assertApplication(
	c *tc.C,
	name string,
) {
	var (
		gotName      string
		gotUUID      string
		gotCharmUUID string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid, charm_uuid, name FROM application WHERE name=?", name).
			Scan(&gotUUID, &gotCharmUUID, &gotName)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotName, tc.Equals, name)
}

func (s *modelRemoteApplicationSuite) assertCharmMetadata(
	c *tc.C,
	name string,
	expected charm.Charm,
) {
	var (
		gotReferenceName string
		gotSourceName    string
		gotCharmName     string

		gotProvides = make(map[string]charm.Relation)
		gotRequires = make(map[string]charm.Relation)
		gotPeers    = make(map[string]charm.Relation)
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT ch.reference_name, cs.name, cm.name
FROM application
JOIN charm AS ch ON application.charm_uuid = ch.uuid
JOIN charm_metadata AS cm ON ch.uuid = cm.charm_uuid
JOIN charm_source AS cs ON ch.source_id = cs.id
WHERE application.name=?`, name).
			Scan(&gotReferenceName, &gotSourceName, &gotCharmName)
		if err != nil {
			return err
		}

		rows, err := tx.QueryContext(ctx, `
SELECT name, role_id, interface, capacity, scope_id
FROM charm_relation
WHERE charm_uuid = (SELECT charm_uuid FROM application WHERE name=?)`, name)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var (
				relName  string
				roleID   int
				iface    string
				capacity int
				scopeID  int
			)
			if err := rows.Scan(&relName, &roleID, &iface, &capacity, &scopeID); err != nil {
				return err
			}
			rel := charm.Relation{
				Name:      relName,
				Interface: iface,
				Limit:     capacity,
			}
			switch roleID {
			case 0:
				rel.Role = charm.RoleProvider
			case 1:
				rel.Role = charm.RoleRequirer
			case 2:
				rel.Role = charm.RolePeer
			default:
				return internalerrors.Errorf("unknown role ID %d", roleID)
			}
			switch scopeID {
			case 0:
				rel.Scope = charm.ScopeGlobal
			case 1:
				rel.Scope = charm.ScopeContainer
			default:
				return internalerrors.Errorf("unknown scope ID %d", scopeID)
			}
			switch rel.Role {
			case charm.RoleProvider:
				gotProvides[rel.Name] = rel
			case charm.RoleRequirer:
				gotRequires[rel.Name] = rel
			case charm.RolePeer:
				gotPeers[rel.Name] = rel
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(gotReferenceName, tc.Equals, expected.ReferenceName)
	c.Check(gotSourceName, tc.Equals, "cmr")
	c.Check(gotCharmName, tc.Equals, expected.Metadata.Name)

	// Every remote application will automatically get a "juju-info" provider
	// relation.
	// Check that it has been added correctly.
	provides := make(map[string]charm.Relation)
	maps.Copy(provides, expected.Metadata.Provides)
	provides["juju-info"] = charm.Relation{
		Name:      "juju-info",
		Role:      charm.RoleProvider,
		Interface: "juju-info",
		Limit:     0,
		Scope:     charm.ScopeGlobal,
	}

	c.Check(gotProvides, tc.DeepEquals, provides)
	c.Check(gotRequires, tc.DeepEquals, expected.Metadata.Requires)
	c.Check(gotPeers, tc.DeepEquals, expected.Metadata.Peers)
}
