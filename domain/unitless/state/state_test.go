// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"testing"

	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/unitless"
	unitlessinternal "github.com/juju/juju/domain/unitless/internal"
	unitlessstate "github.com/juju/juju/domain/unitless/state"
)

type stateSuite struct {
	schematesting.ModelSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestGetScriptletApplication(c *tc.C) {
	charmID := tc.Must(c, corecharm.NewID)
	applicationID := tc.Must(c, coreapplication.NewUUID)
	s.insertApplication(c, charmID.String(), applicationID.String(), true)

	st := unitlessstate.NewState(s.TxnRunnerFactory())
	scriptlet, err := st.GetScriptletApplication(c.Context(), applicationID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(scriptlet, tc.DeepEquals, unitlessinternal.ScriptletApplication{
		UUID: applicationID.String(),
		Name: applicationID.String(),
		Life: 0,
		Sources: []unitlessinternal.ScriptSource{{
			LoadPath: "hook.star",
			Source:   "load hook",
		}, {
			LoadPath: "status.star",
			Source:   "set status",
		}},
	})
}

func (s *stateSuite) TestGetScriptletApplicationNotFound(c *tc.C) {
	st := unitlessstate.NewState(s.TxnRunnerFactory())
	scriptlet, err := st.GetScriptletApplication(c.Context(), tc.Must(c, coreapplication.NewUUID).String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(scriptlet, tc.DeepEquals, unitlessinternal.ScriptletApplication{})
}

func (s *stateSuite) TestFilterScriptletApplications(c *tc.C) {
	scriptletCharmID := tc.Must(c, corecharm.NewID)
	scriptletApplicationID := tc.Must(c, coreapplication.NewUUID)
	s.insertApplication(c, scriptletCharmID.String(), scriptletApplicationID.String(), true)

	regularCharmID := tc.Must(c, corecharm.NewID)
	regularApplicationID := tc.Must(c, coreapplication.NewUUID)
	s.insertApplication(c, regularCharmID.String(), regularApplicationID.String(), false)

	st := unitlessstate.NewState(s.TxnRunnerFactory())
	result, err := st.FilterScriptletApplications(c.Context(), []string{
		regularApplicationID.String(), scriptletApplicationID.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []string{scriptletApplicationID.String()})
}

func (s *stateSuite) TestGetScriptletEvent(c *tc.C) {
	event, err := unitlessstate.NewState(s.TxnRunnerFactory()).GetScriptletEvent(
		c.Context(), "application-uuid", "config-changed",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(event, tc.DeepEquals, unitless.Event{})
}

func (s *stateSuite) insertApplication(c *tc.C, charmID, applicationID string, scriptlet bool) {
	_, err := s.DB().Exec(`
INSERT INTO charm (uuid, reference_name, architecture_id)
VALUES (?, ?, 0);
`, charmID, charmID)
	c.Assert(err, tc.ErrorIsNil)

	if scriptlet {
		_, err = s.DB().Exec(`
INSERT INTO charm_scriptlet (charm_uuid, path, content)
VALUES (?, 'status.star', 'set status'), (?, 'hook.star', 'load hook');
`, charmID, charmID)
		c.Assert(err, tc.ErrorIsNil)
	}

	_, err = s.DB().Exec(`
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid)
VALUES (?, ?, 0, ?, '656b4a82-e28c-53d6-a014-f0dd53417eb6');
`, applicationID, applicationID, charmID)
	c.Assert(err, tc.ErrorIsNil)
}
