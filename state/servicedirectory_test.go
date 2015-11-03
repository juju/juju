// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/state"
)

type serviceDirectorySuite struct {
	ConnSuite
	record state.ServiceDirectoryRecord
}

var _ = gc.Suite(&serviceDirectorySuite{})

func (s *serviceDirectorySuite) createDirectoryRecord(c *gc.C) *state.ServiceDirectoryRecord {
	eps := []state.Endpoint{
		{
			ServiceName: "mysql",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		},
		{
			ServiceName: "mysql",
			Relation: charm.Relation{
				Interface: "mysql-root",
				Name:      "db-admin",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		},
	}
	record, err := s.State.AddServiceDirectoryRecord(state.AddServiceDirectoryParams{
		ServiceName:   "mysql",
		Endpoints:     eps,
		SourceEnvUUID: "source-uuid",
		SourceLabel:   "source",
	})
	c.Assert(err, jc.ErrorIsNil)
	return record
}

func (s *serviceDirectorySuite) TestEndpoints(c *gc.C) {
	record := s.createDirectoryRecord(c)
	_, err := record.Endpoint("foo")
	c.Assert(err, gc.ErrorMatches, `service directory record "source-uuid-mysql" has no \"foo\" relation`)

	serverEP, err := record.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverEP, gc.DeepEquals, state.Endpoint{
		ServiceName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	adminEp := state.Endpoint{
		ServiceName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql-root",
			Name:      "db-admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	eps, err := record.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eps, gc.DeepEquals, []state.Endpoint{serverEP, adminEp})
}

func (s *serviceDirectorySuite) TestDirectoryRecordRefresh(c *gc.C) {
	record := s.createDirectoryRecord(c)
	s1, err := s.State.ServiceDirectoryRecord(record.ServiceName())
	c.Assert(err, jc.ErrorIsNil)

	err = s1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = record.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *serviceDirectorySuite) TestDestroy(c *gc.C) {
	record := s.createDirectoryRecord(c)
	err := record.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = record.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *serviceDirectorySuite) TestAllServiceDirectoryRecordsNone(c *gc.C) {
	services, err := s.State.AllServiceDirectoryEntries()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(services), gc.Equals, 0)
}

func (s *serviceDirectorySuite) TestAddServiceDirectoryRecords(c *gc.C) {
	record := s.createDirectoryRecord(c)
	records, err := s.State.AllServiceDirectoryEntries()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(records), gc.Equals, 1)
	c.Assert(records[0], jc.DeepEquals, record)

	_, err = s.State.AddServiceDirectoryRecord(state.AddServiceDirectoryParams{
		ServiceName:   "another",
		SourceEnvUUID: "uuid",
	})
	c.Assert(err, jc.ErrorIsNil)
	records, err = s.State.AllServiceDirectoryEntries()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(records, gc.HasLen, 2)

	// Check the returned record, order is defined by sorted keys.
	names := make([]string, len(records))
	for i, record := range records {
		names[i] = record.ServiceName()
	}
	sort.Strings(names)
	c.Assert(names[0], gc.Equals, "another")
	c.Assert(names[1], gc.Equals, "mysql")
}

func (s *serviceDirectorySuite) TestAddServiceDirectoryRecordUUIDRequired(c *gc.C) {
	_, err := s.State.AddServiceDirectoryRecord(state.AddServiceDirectoryParams{
		ServiceName: "another",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add service direcotry record "another": missing source environment UUID`)
}

func (s *serviceDirectorySuite) TestAddServiceDirectoryRecordDuplicate(c *gc.C) {
	_, err := s.State.AddServiceDirectoryRecord(state.AddServiceDirectoryParams{
		ServiceName:   "another",
		SourceEnvUUID: "uuid",
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddServiceDirectoryRecord(state.AddServiceDirectoryParams{
		ServiceName:   "another",
		SourceEnvUUID: "another-uuid",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add service direcotry record "another": service directory record already exists`)
}

func (s *remoteServiceSuite) TestAddServiceDirectoryEntryDuplicateAddedAfterInitial(c *gc.C) {
	// Check that a record with a name conflict cannot be added if
	// there is no conflict initially but a record is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.AddServiceDirectoryRecord(state.AddServiceDirectoryParams{
			ServiceName:   "record",
			SourceEnvUUID: "uuid",
		})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddServiceDirectoryRecord(state.AddServiceDirectoryParams{
		ServiceName:   "record",
		SourceEnvUUID: "another-uuid",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add service direcotry record "record": service directory record already exists`)
}
