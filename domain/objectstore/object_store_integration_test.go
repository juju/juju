// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/objectstore"
	domainobjectstore "github.com/juju/juju/domain/objectstore"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/domain/objectstore/service"
	"github.com/juju/juju/domain/objectstore/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type integrationSuite struct {
	schematesting.ControllerSuite
}

func TestIntegrationSuite(t *stdtesting.T) {
	tc.Run(t, &integrationSuite{})
}

func (s *integrationSuite) newState() *state.State {
	return state.NewState(s.TxnRunnerFactory(), clock.WallClock)
}

func (s *integrationSuite) newService() *service.Service {
	return service.NewService(s.newState())
}

func (s *integrationSuite) newDrainingService() *service.WatchableDrainingService {
	return service.NewWatchableDrainingService(s.newState(), nil)
}

// TestPutAndGetMetadata verifies that metadata can be stored and retrieved
// correctly through the service → state integration.
func (s *integrationSuite) TestPutAndGetMetadata(c *tc.C) {
	svc := s.newService()

	metadata := objectstore.Metadata{
		Path:   "agents/agent-1234.tar.gz",
		SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SHA384: "38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
		Size:   1024,
	}

	uuid, err := svc.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid, tc.Not(tc.Equals), objectstore.UUID(""))

	got, err := svc.GetMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.Path, tc.Equals, metadata.Path)
	c.Check(got.SHA256, tc.Equals, metadata.SHA256)
	c.Check(got.SHA384, tc.Equals, metadata.SHA384)
	c.Check(got.Size, tc.Equals, metadata.Size)
}

// TestPutMetadataAndGetBySHA256 verifies retrieval by SHA256.
func (s *integrationSuite) TestPutMetadataAndGetBySHA256(c *tc.C) {
	svc := s.newService()

	metadata := objectstore.Metadata{
		Path:   "tools/jujud-3.6.tar.gz",
		SHA256: "a3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SHA384: "48b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
		Size:   2048,
	}

	_, err := svc.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	got, err := svc.GetMetadataBySHA256(c.Context(), metadata.SHA256)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.Path, tc.Equals, metadata.Path)
	c.Check(got.SHA256, tc.Equals, metadata.SHA256)
}

// TestPutMetadataAndGetBySHA256Prefix verifies retrieval by SHA256 prefix.
func (s *integrationSuite) TestPutMetadataAndGetBySHA256Prefix(c *tc.C) {
	svc := s.newService()

	metadata := objectstore.Metadata{
		Path:   "tools/jujud-4.0.tar.gz",
		SHA256: "b4c0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SHA384: "58b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
		Size:   4096,
	}

	_, err := svc.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	// Retrieve by 7-character prefix.
	got, err := svc.GetMetadataBySHA256Prefix(c.Context(), metadata.SHA256[:7])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.Path, tc.Equals, metadata.Path)
}

// TestPutAndRemoveMetadata verifies that metadata can be stored and then
// removed.
func (s *integrationSuite) TestPutAndRemoveMetadata(c *tc.C) {
	svc := s.newService()

	metadata := objectstore.Metadata{
		Path:   "charm/foo-1.charm",
		SHA256: "c5d0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SHA384: "68b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
		Size:   512,
	}

	_, err := svc.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	err = svc.RemoveMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIsNil)

	_, err = svc.GetMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
}

// TestListMetadata verifies listing all metadata.
func (s *integrationSuite) TestListMetadata(c *tc.C) {
	svc := s.newService()

	items := []objectstore.Metadata{
		{
			Path:   "a/first.tar.gz",
			SHA256: "1100c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			SHA384: "1100060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
			Size:   100,
		},
		{
			Path:   "b/second.tar.gz",
			SHA256: "2200c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			SHA384: "2200060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
			Size:   200,
		},
	}

	for _, m := range items {
		_, err := svc.PutMetadata(c.Context(), m)
		c.Assert(err, tc.ErrorIsNil)
	}

	list, err := svc.ListMetadata(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(list) >= 2, tc.IsTrue)

	// Verify both items are present.
	paths := make(map[string]bool)
	for _, m := range list {
		paths[m.Path] = true
	}
	c.Check(paths["a/first.tar.gz"], tc.IsTrue)
	c.Check(paths["b/second.tar.gz"], tc.IsTrue)
}

// TestGetMetadataNotFound verifies that retrieving non-existent metadata
// returns the expected error.
func (s *integrationSuite) TestGetMetadataNotFound(c *tc.C) {
	svc := s.newService()

	_, err := svc.GetMetadata(c.Context(), "does/not/exist")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
}

// TestTransitionBackendToS3 verifies the full backend transition through the
// service, including that the draining phase is set atomically.
func (s *integrationSuite) TestTransitionBackendToS3(c *tc.C) {
	svc := s.newDrainingService()

	// Initially there is no drain in progress.
	phase, err := svc.GetDrainingPhase(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(phase, tc.Equals, objectstore.PhaseUnknown)

	// Record the initial backend.
	initialBackend, err := svc.GetActiveObjectStoreBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(initialBackend.Type, tc.Equals, objectstore.FileBackend)

	// Transition to S3.
	err = svc.TransitionBackendToS3(c.Context(), domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "I AM AN ACCESS KEY",
		SecretKey: "I AM A SECRET",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Verify the draining phase was set atomically.
	phase, err = svc.GetDrainingPhase(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(phase, tc.Equals, objectstore.PhaseDraining)

	// Verify the active backend is now S3.
	newBackend, err := svc.GetActiveObjectStoreBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newBackend.Type, tc.Equals, objectstore.S3Backend)
	c.Assert(newBackend.UUID, tc.Not(tc.Equals), initialBackend.UUID)

	// Verify S3 credentials are accessible.
	creds, ok := newBackend.S3Credentials()
	c.Assert(ok, tc.IsTrue)
	c.Check(creds.Endpoint, tc.Equals, "https://s3.example.com")
	c.Check(creds.AccessKey, tc.Equals, "I AM AN ACCESS KEY")
	c.Check(creds.SecretKey, tc.Equals, "I AM A SECRET")

	// Verify the phase info has both backend UUIDs.
	phaseInfo, err := svc.GetDrainingPhaseInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(phaseInfo.Phase, tc.Equals, objectstore.PhaseDraining)
	c.Assert(phaseInfo.FromBackendUUID, tc.Not(tc.IsNil))
	c.Check(*phaseInfo.FromBackendUUID, tc.Equals, initialBackend.UUID)
	c.Check(phaseInfo.ActiveBackendUUID, tc.Equals, newBackend.UUID)
}

// TestTransitionBackendToS3AlreadyInProgress verifies that attempting to
// transition while a drain is already in progress returns the expected error.
func (s *integrationSuite) TestTransitionBackendToS3AlreadyInProgress(c *tc.C) {
	svc := s.newDrainingService()

	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	err := svc.TransitionBackendToS3(c.Context(), creds)
	c.Assert(err, tc.ErrorIsNil)

	// Second call should fail because draining is already in progress.
	err = svc.TransitionBackendToS3(c.Context(), creds)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrDrainingAlreadyInProgress)
}

// TestTransitionBackendToS3InvalidCredentials verifies that invalid
// credentials are rejected at the service layer.
func (s *integrationSuite) TestTransitionBackendToS3InvalidCredentials(c *tc.C) {
	svc := s.newDrainingService()

	err := svc.TransitionBackendToS3(c.Context(), domainobjectstore.S3Credentials{
		Endpoint: "",
	})
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

// TestDrainingLifecycleCompleted verifies the full drain lifecycle from
// initiation through completion. SetDrainingPhase(PhaseCompleted) atomically
// marks the from-backend as dead and transitions the phase.
func (s *integrationSuite) TestDrainingLifecycleCompleted(c *tc.C) {
	svc := s.newDrainingService()

	// 1. Start the drain via TransitionBackendToS3.
	err := svc.TransitionBackendToS3(c.Context(), domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	})
	c.Assert(err, tc.ErrorIsNil)

	phase, err := svc.GetDrainingPhase(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(phase, tc.Equals, objectstore.PhaseDraining)

	// 2. Complete the drain. This atomically marks the from-backend as dead.
	err = svc.SetDrainingPhase(c.Context(), objectstore.PhaseCompleted)
	c.Assert(err, tc.ErrorIsNil)

	// After completing, the drain is no longer "active".
	phase, err = svc.GetDrainingPhase(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(phase, tc.Equals, objectstore.PhaseUnknown)

	// Verify only the new S3 backend is active.
	backend, err := svc.GetActiveObjectStoreBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(backend.Type, tc.Equals, objectstore.S3Backend)
}

// TestDrainingLifecycleError verifies the drain lifecycle when transitioning
// to an error state.
func (s *integrationSuite) TestDrainingLifecycleError(c *tc.C) {
	svc := s.newDrainingService()

	// 1. Start the drain.
	err := svc.TransitionBackendToS3(c.Context(), domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	})
	c.Assert(err, tc.ErrorIsNil)

	// 2. Transition to error. Error is terminal, so the drain is no longer
	// active and GetDrainingPhase returns PhaseUnknown.
	err = svc.SetDrainingPhase(c.Context(), objectstore.PhaseError)
	c.Assert(err, tc.ErrorIsNil)

	phase, err := svc.GetDrainingPhase(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(phase, tc.Equals, objectstore.PhaseUnknown)
}

// TestDrainingPhaseInvalidTransition verifies that invalid phase transitions
// are rejected.
func (s *integrationSuite) TestDrainingPhaseInvalidTransition(c *tc.C) {
	svc := s.newDrainingService()

	// Start draining.
	err := svc.TransitionBackendToS3(c.Context(), domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Trying to go back to draining (from draining) should fail.
	err = svc.SetDrainingPhase(c.Context(), objectstore.PhaseDraining)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrDrainingAlreadyInProgress)
}

// TestGetObjectStoreBackend verifies that we can retrieve a specific backend
// by UUID.
func (s *integrationSuite) TestGetObjectStoreBackend(c *tc.C) {
	svc := s.newDrainingService()

	// Get the active backend.
	active, err := svc.GetActiveObjectStoreBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Retrieve it by UUID.
	backend, err := svc.GetObjectStoreBackend(c.Context(), active.UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(backend.UUID, tc.Equals, active.UUID)
	c.Check(backend.Type, tc.Equals, active.Type)
}

// TestSetDrainingPhaseCompletedWithoutDrainIsNoop verifies that calling
// SetDrainingPhase(PhaseCompleted) when there is no active drain is a no-op.
func (s *integrationSuite) TestSetDrainingPhaseCompletedWithoutDrainIsNoop(c *tc.C) {
	svc := s.newDrainingService()

	err := svc.SetDrainingPhase(c.Context(), objectstore.PhaseCompleted)
	c.Assert(err, tc.ErrorIsNil)
}

// TestPutMetadataWithControllerIDHint verifies storing metadata with a
// controller ID hint and retrieving the hint.
func (s *integrationSuite) TestPutMetadataWithControllerIDHint(c *tc.C) {
	svc := s.newService()

	metadata := objectstore.Metadata{
		Path:   "tools/jujud-hint.tar.gz",
		SHA256: "d6e0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SHA384: "78b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
		Size:   8192,
	}

	_, err := svc.PutMetadataWithControllerIDHint(c.Context(), metadata, "controller-1")
	c.Assert(err, tc.ErrorIsNil)

	hints, err := svc.GetControllerIDHints(c.Context(), metadata.SHA384)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hints, tc.DeepEquals, []string{"controller-1"})
}

// TestAddControllerIDHint verifies adding a controller ID hint to existing
// metadata.
func (s *integrationSuite) TestAddControllerIDHint(c *tc.C) {
	svc := s.newService()

	metadata := objectstore.Metadata{
		Path:   "tools/jujud-multi-hint.tar.gz",
		SHA256: "e7f0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SHA384: "88b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
		Size:   16384,
	}

	_, err := svc.PutMetadataWithControllerIDHint(c.Context(), metadata, "controller-1")
	c.Assert(err, tc.ErrorIsNil)

	err = svc.AddControllerIDHint(c.Context(), metadata.SHA384, "controller-2")
	c.Assert(err, tc.ErrorIsNil)

	hints, err := svc.GetControllerIDHints(c.Context(), metadata.SHA384)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(hints), tc.Equals, 2)
}

// TestTransitionBackendToS3FromS3NotSupported verifies that once S3 is active,
// a subsequent transition to another S3 backend is rejected.
func (s *integrationSuite) TestTransitionBackendToS3FromS3NotSupported(c *tc.C) {
	svc := s.newDrainingService()

	// First cycle: transition to S3.
	err := svc.TransitionBackendToS3(c.Context(), domainobjectstore.S3Credentials{
		Endpoint:  "https://s3-first.example.com",
		AccessKey: "access-key-1",
		SecretKey: "secret-key-1",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Complete the drain (atomically marks old backend dead).
	err = svc.SetDrainingPhase(c.Context(), objectstore.PhaseCompleted)
	c.Assert(err, tc.ErrorIsNil)

	firstBackend, err := svc.GetActiveObjectStoreBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(firstBackend.Type, tc.Equals, objectstore.S3Backend)

	// A second transition from S3 to S3 is not supported.
	err = svc.TransitionBackendToS3(c.Context(), domainobjectstore.S3Credentials{
		Endpoint:  "https://s3-second.example.com",
		AccessKey: "access-key-2",
		SecretKey: "secret-key-2",
	})
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrBackendTransitionNotSupported)
}

// TestMetadataPersistsThroughBackendTransition verifies that object metadata
// persists across backend transitions. Metadata is stored independently of
// the backend type.
func (s *integrationSuite) TestMetadataPersistsThroughBackendTransition(c *tc.C) {
	svc := s.newService()
	drainingSvc := s.newDrainingService()

	// Store metadata while using file backend.
	metadata := objectstore.Metadata{
		Path:   "tools/jujud-persist.tar.gz",
		SHA256: "f8a0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SHA384: "98b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
		Size:   32768,
	}

	_, err := svc.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	// Transition to S3.
	err = drainingSvc.TransitionBackendToS3(c.Context(), domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Metadata should still be retrievable.
	got, err := svc.GetMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.Path, tc.Equals, metadata.Path)
	c.Check(got.SHA256, tc.Equals, metadata.SHA256)
}

// newDrainingServiceWithState returns a draining service and the underlying
// state so tests can call state methods directly for setup.
func (s *integrationSuite) newDrainingServiceWithState() (*service.WatchableDrainingService, *state.State) {
	st := s.newState()
	svc := service.NewWatchableDrainingService(st, nil)
	return svc, st
}

// TestStateAndServiceConsistency verifies that calling state methods directly
// produces results consistent with what the service returns.
func (s *integrationSuite) TestStateAndServiceConsistency(c *tc.C) {
	svc, st := s.newDrainingServiceWithState()

	// Get the active backend via state directly.
	stateBackend, err := st.GetActiveObjectStoreBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Get the active backend via service.
	svcBackend, err := svc.GetActiveObjectStoreBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Both should return the same UUID and type.
	c.Check(string(svcBackend.UUID), tc.Equals, stateBackend.UUID)
	c.Check(string(svcBackend.Type), tc.Equals, stateBackend.ObjectStoreType)
}

// newServiceFromFactory creates a service using a factory function, similar
// to how production code creates services.
func (s *integrationSuite) newServiceFromFactory() *service.Service {
	factory := func(ctx context.Context) (database.TxnRunner, error) {
		return s.TxnRunnerFactory()(ctx)
	}
	return service.NewService(state.NewState(factory, clock.WallClock))
}

// TestServiceFromFactory verifies that a service created from a factory
// function works correctly end-to-end.
func (s *integrationSuite) TestServiceFromFactory(c *tc.C) {
	svc := s.newServiceFromFactory()

	metadata := objectstore.Metadata{
		Path:   "factory/test.tar.gz",
		SHA256: "abcdc44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SHA384: "abcd60a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
		Size:   256,
	}

	uuid, err := svc.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid, tc.Not(tc.Equals), objectstore.UUID(""))

	got, err := svc.GetMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.Path, tc.Equals, metadata.Path)
}
