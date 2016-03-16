// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/migrationmaster"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

// Ensure that Backend remains compatible with *state.State
var _ migrationmaster.Backend = (*state.State)(nil)

type Suite struct {
	testing.BaseSuite

	backend    *stubBackend
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.backend = &stubBackend{
		migration: new(stubMigration),
	}
	migrationmaster.PatchState(s, s.backend)

	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}
}

func (s *Suite) TestNotEnvironManager(c *gc.C) {
	s.authorizer.EnvironManager = false

	api, err := s.makeAPI()
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.Equals, common.ErrPerm)
}

func (s *Suite) TestWatch(c *gc.C) {
	api := s.mustMakeAPI(c)

	watchResult, err := api.Watch()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watchResult.NotifyWatcherId, gc.Not(gc.Equals), "")
}

func (s *Suite) TestWatchError(c *gc.C) {
	s.backend.watchError = errors.New("boom")
	api := s.mustMakeAPI(c)

	w, err := api.Watch()
	c.Assert(w, gc.Equals, params.NotifyWatchResult{})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *Suite) TestSetPhase(c *gc.C) {
	api := s.mustMakeAPI(c)

	err := api.SetPhase(params.SetMigrationPhaseArgs{Phase: "ABORT"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.backend.migration.phaseSet, gc.Equals, coremigration.ABORT)
}

func (s *Suite) TestSetPhaseNoMigration(c *gc.C) {
	s.backend.getErr = errors.New("boom")
	api := s.mustMakeAPI(c)

	err := api.SetPhase(params.SetMigrationPhaseArgs{Phase: "ABORT"})
	c.Assert(err, gc.ErrorMatches, "could not get migration: boom")
}

func (s *Suite) TestSetPhaseBadPhase(c *gc.C) {
	api := s.mustMakeAPI(c)

	err := api.SetPhase(params.SetMigrationPhaseArgs{Phase: "wat"})
	c.Assert(err, gc.ErrorMatches, `invalid phase: "wat"`)
}

func (s *Suite) TestSetPhaseError(c *gc.C) {
	s.backend.migration.setPhaseErr = errors.New("blam")
	api := s.mustMakeAPI(c)

	err := api.SetPhase(params.SetMigrationPhaseArgs{Phase: "ABORT"})
	c.Assert(err, gc.ErrorMatches, "failed to set phase: blam")
}

func (s *Suite) TestExport(c *gc.C) {
	exportModel := func(migration.StateExporter) ([]byte, error) {
		return []byte("foo"), nil
	}
	migrationmaster.PatchExportModel(s, exportModel)
	api := s.mustMakeAPI(c)

	serialized, err := api.Export()

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serialized, gc.DeepEquals, params.SerializedModel{
		Bytes: []byte("foo"),
	})
}

func (s *Suite) makeAPI() (*migrationmaster.API, error) {
	return migrationmaster.NewAPI(nil, s.resources, s.authorizer)
}

func (s *Suite) mustMakeAPI(c *gc.C) *migrationmaster.API {
	api, err := migrationmaster.NewAPI(nil, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

type stubBackend struct {
	migrationmaster.Backend

	watchError error
	getErr     error
	migration  *stubMigration
}

func (b *stubBackend) WatchForModelMigration() (state.NotifyWatcher, error) {
	if b.watchError != nil {
		return nil, b.watchError
	}
	return apiservertesting.NewFakeNotifyWatcher(), nil
}

func (b *stubBackend) GetModelMigration() (state.ModelMigration, error) {
	if b.getErr != nil {
		return nil, b.getErr
	}
	return b.migration, nil
}

type stubMigration struct {
	state.ModelMigration
	setPhaseErr error
	phaseSet    coremigration.Phase
}

func (m *stubMigration) SetPhase(phase coremigration.Phase) error {
	if m.setPhaseErr != nil {
		return m.setPhaseErr
	}
	m.phaseSet = phase
	return nil
}
