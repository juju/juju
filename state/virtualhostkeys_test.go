// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/virtualhostkeys"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/testing/factory"
)

type VirtualHostKeysSuite struct {
	ConnSuite
}

var _ = gc.Suite(&VirtualHostKeysSuite{})

func (s *VirtualHostKeysSuite) TestMachineVirtualHostKey(c *gc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	lookupID := virtualhostkeys.MachineHostKeyID(machine.Id())
	key, err := s.State.MachineVirtualHostKey(lookupID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key.HostKey(), gc.Not(gc.HasLen), 0)

	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.MachineVirtualHostKey(lookupID)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

// TestCAASUnitVirtualHostKey copies setup from
// `(s *CAASApplicationSuite) TestUpsertCAASUnit`
// to verify CAAS unit host keys are created.
func (s *VirtualHostKeysSuite) TestCAASUnitVirtualHostKey(c *gc.C) {
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { _ = caasSt.Close() })

	// Consume the initial construction events from the watchers.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	registry := &storage.StaticProviderRegistry{
		Providers: map[storage.ProviderType]storage.Provider{
			"kubernetes": &dummy.StorageProvider{
				StorageScope: storage.ScopeEnviron,
				IsDynamic:    true,
				IsReleasable: true,
				SupportsFunc: func(k storage.StorageKind) bool {
					return k == storage.StorageKindBlock
				},
			},
		},
	}

	st := s.Factory.MakeCAASModel(c, &factory.ModelParams{
		CloudName: "caascloud",
	})
	s.AddCleanup(func(_ *gc.C) { _ = st.Close() })

	pm := poolmanager.New(state.NewStateSettings(st), registry)
	_, err := pm.Create("kubernetes", "kubernetes", map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	s.policy = testing.MockPolicy{
		GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
			return registry, nil
		},
	}

	sb, err := state.NewStorageBackend(st)
	c.Assert(err, jc.ErrorIsNil)

	fsInfo := state.FilesystemInfo{
		Size: 100,
		Pool: "kubernetes",
	}
	volumeInfo := state.VolumeInfo{
		VolumeId:   "pv-database-0",
		Size:       100,
		Pool:       "kubernetes",
		Persistent: true,
	}
	storageTag, err := sb.AddExistingFilesystem(fsInfo, &volumeInfo, "database")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageTag.Id(), gc.Equals, "database/0")

	ch := state.AddTestingCharmForSeries(c, st, "quantal", "cockroachdb")
	cockroachdb := state.AddTestingApplicationWithStorage(c, st, "cockroachdb", ch, map[string]state.StorageConstraints{
		"database": {
			Pool:  "kubernetes",
			Size:  100,
			Count: 0,
		},
	})

	unitName := "cockroachdb/0"
	providerId := "cockroachdb-0"
	address := "1.2.3.4"
	ports := []string{"80", "443"}

	// output of utils.AgentPasswordHash("juju")
	passwordHash := "v+jK3ht5NEdKeoQBfyxmlYe0"

	p := state.UpsertCAASUnitParams{
		AddUnitParams: state.AddUnitParams{
			UnitName:       &unitName,
			ProviderId:     &providerId,
			Address:        &address,
			Ports:          &ports,
			PasswordHash:   &passwordHash,
			VirtualHostKey: []byte("foo"),
		},
		OrderedScale:              true,
		OrderedId:                 0,
		ObservedAttachedVolumeIDs: []string{"pv-database-0"},
	}

	err = cockroachdb.SetScale(1, 0, true)
	c.Assert(err, jc.ErrorIsNil)

	unit, err := cockroachdb.UpsertCAASUnit(p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit, gc.NotNil)

	lookupID := virtualhostkeys.UnitHostKeyID(unit.UnitTag().Id())
	key, err := st.UnitVirtualHostKey(lookupID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(key.HostKey()), gc.Equals, "foo")

	// Unable to remove the unit because storage is attached
	// but cannot remove storage because of an error.

	// err = sb.RemoveStorageAttachment(storageTag, unit.UnitTag(), false)
	// c.Assert(err, jc.ErrorIsNil)
	// err = sb.DetachStorage(storageTag, unit.UnitTag(), false, dontWait)
	// c.Assert(err, jc.ErrorIsNil)
	// err = sb.DestroyStorageInstance(storageTag, true, false, dontWait)
	// c.Assert(err, jc.ErrorIsNil)

	// err = cockroachdb.Destroy()
	// c.Assert(err, jc.ErrorIsNil)
	// err = unit.EnsureDead()
	// c.Assert(err, jc.ErrorIsNil)
	// err = unit.Remove()
	// c.Assert(err, jc.ErrorIsNil)

	// _, err = st.UnitVirtualHostKey(lookupID)
	// c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *VirtualHostKeysSuite) TestIAASUnitVirtualHostKeyDoesNotExist(c *gc.C) {
	charm := s.AddTestingCharm(c, "wordpress")
	application := s.AddTestingApplication(c, "wordpress", charm)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.UnitVirtualHostKey(virtualhostkeys.UnitHostKeyID(unit.Name()))
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *VirtualHostKeysSuite) TestIAASUnitWithPlacement(c *gc.C) {
	ch := state.AddTestingCharmForSeries(c, s.State, "quantal", "wordpress")
	app := s.AddTestingApplication(c, "wordpress", ch)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	id, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	m, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	lookupID := virtualhostkeys.MachineHostKeyID(m.Id())
	key, err := s.State.MachineVirtualHostKey(lookupID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key.HostKey(), gc.Not(gc.HasLen), 0)
}
