// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/life"
	domainobjectstore "github.com/juju/juju/domain/objectstore"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestGetMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := tc.Must(c, uuid.NewUUID).String()

	metadata := objectstore.Metadata{
		Path:   path,
		SHA256: tc.Must(c, uuid.NewUUID).String(),
		SHA384: tc.Must(c, uuid.NewUUID).String(),
		Size:   666,
	}

	s.state.EXPECT().GetMetadata(gomock.Any(), path).Return(objectstore.Metadata{
		Path:   metadata.Path,
		Size:   metadata.Size,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
	}, nil)

	p, err := NewService(s.state).GetMetadata(c.Context(), path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(p, tc.DeepEquals, metadata)
}

func (s *serviceSuite) TestGetMetadataBySHA256(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sha256 := sha256.New()
	sha256.Write([]byte(tc.Must(c, uuid.NewUUID).String()))
	sha := hex.EncodeToString(sha256.Sum(nil))

	metadata := objectstore.Metadata{
		Path:   "path",
		SHA256: tc.Must(c, uuid.NewUUID).String(),
		SHA384: tc.Must(c, uuid.NewUUID).String(),
		Size:   666,
	}

	s.state.EXPECT().GetMetadataBySHA256(gomock.Any(), sha).Return(objectstore.Metadata{
		Path:   metadata.Path,
		Size:   metadata.Size,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
	}, nil)

	p, err := NewService(s.state).GetMetadataBySHA256(c.Context(), sha)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(p, tc.DeepEquals, metadata)
}

func (s *serviceSuite) TestGetMetadataBySHA256Invalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sha256 := sha256.New()
	sha256.Write([]byte(tc.Must(c, uuid.NewUUID).String()))
	sha := hex.EncodeToString(sha256.Sum(nil))

	illegalSha := "!" + sha[1:]

	_, err := NewService(s.state).GetMetadataBySHA256(c.Context(), illegalSha)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrInvalidHash)
}

func (s *serviceSuite) TestGetMetadataBySHA256TooShort(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sha256 := sha256.New()
	sha256.Write([]byte(tc.Must(c, uuid.NewUUID).String()))
	sha := hex.EncodeToString(sha256.Sum(nil))

	_, err := NewService(s.state).GetMetadataBySHA256(c.Context(), sha+"a")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrInvalidHashLength)
}

func (s *serviceSuite) TestGetMetadataBySHA256TooLong(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetMetadataBySHA256(c.Context(), "beef")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrInvalidHashLength)
}

func (s *serviceSuite) TestGetMetadataBySHA256Prefix(c *tc.C) {
	defer s.setupMocks(c).Finish()

	shaPrefix := "deadbeef"

	metadata := objectstore.Metadata{
		Path:   "path",
		SHA256: tc.Must(c, uuid.NewUUID).String(),
		SHA384: tc.Must(c, uuid.NewUUID).String(),
		Size:   666,
	}

	s.state.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), shaPrefix).Return(objectstore.Metadata{
		Path:   metadata.Path,
		Size:   metadata.Size,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
	}, nil)

	p, err := NewService(s.state).GetMetadataBySHA256Prefix(c.Context(), shaPrefix)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(p, tc.DeepEquals, metadata)
}

func (s *serviceSuite) TestGetMetadataBySHA256PrefixTooShort(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetMetadataBySHA256Prefix(c.Context(), "beef")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrHashPrefixTooShort)
}

func (s *serviceSuite) TestGetMetadataBySHA256PrefixTooLong(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetMetadataBySHA256Prefix(c.Context(), "deadbeef1")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrInvalidHashPrefix)
}

func (s *serviceSuite) TestGetMetadataBySHA256PrefixInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetMetadataBySHA256Prefix(c.Context(), "abcdefg")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrInvalidHashPrefix)
}

func (s *serviceSuite) TestListMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := tc.Must(c, uuid.NewUUID).String()

	metadata := objectstore.Metadata{
		Path:   path,
		SHA256: tc.Must(c, uuid.NewUUID).String(),
		SHA384: tc.Must(c, uuid.NewUUID).String(),
		Size:   666,
	}

	s.state.EXPECT().ListMetadata(gomock.Any()).Return([]objectstore.Metadata{{
		Path:   metadata.Path,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
		Size:   metadata.Size,
	}}, nil)

	p, err := NewService(s.state).ListMetadata(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(p, tc.DeepEquals, []objectstore.Metadata{{
		Path:   metadata.Path,
		Size:   metadata.Size,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
	}})
}

func (s *serviceSuite) TestPutMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := tc.Must(c, uuid.NewUUID).String()
	metadata := objectstore.Metadata{
		Path:   path,
		SHA256: tc.Must(c, uuid.NewUUID).String(),
		SHA384: tc.Must(c, uuid.NewUUID).String(),
		Size:   666,
	}

	var rUUID string
	s.state.EXPECT().PutMetadata(gomock.Any(), gomock.Any(), metadata).
		DoAndReturn(func(ctx context.Context, uuid string, data objectstore.Metadata) (string, error) {
			rUUID = uuid
			return uuid, nil
		})

	result, err := NewService(s.state).PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, objectstore.UUID(rUUID))
}

func (s *serviceSuite) TestPutMetadataMissingSHA384(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := tc.Must(c, uuid.NewUUID).String()
	metadata := objectstore.Metadata{
		Path:   path,
		SHA256: tc.Must(c, uuid.NewUUID).String(),
		Size:   666,
	}

	_, err := NewService(s.state).PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrMissingHash)
}

func (s *serviceSuite) TestPutMetadataMissingSHA256(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := tc.Must(c, uuid.NewUUID).String()
	metadata := objectstore.Metadata{
		Path:   path,
		SHA384: tc.Must(c, uuid.NewUUID).String(),
		Size:   666,
	}

	_, err := NewService(s.state).PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrMissingHash)
}

func (s *serviceSuite) TestPutMetadataWithControllerIDHint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerID := "1"
	path := tc.Must(c, uuid.NewUUID).String()
	metadata := objectstore.Metadata{
		Path:   path,
		SHA256: tc.Must(c, uuid.NewUUID).String(),
		SHA384: tc.Must(c, uuid.NewUUID).String(),
		Size:   666,
	}

	var rUUID string
	s.state.EXPECT().PutMetadataWithControllerIDHint(gomock.Any(), gomock.Any(), metadata, controllerID).
		DoAndReturn(func(ctx context.Context, uuid string, data objectstore.Metadata, _ string) (string, error) {

			rUUID = uuid
			return uuid, nil
		})

	result, err := NewService(s.state).PutMetadataWithControllerIDHint(c.Context(), metadata, controllerID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, objectstore.UUID(rUUID))
}

func (s *serviceSuite) TestPutMetadataWithControllerIDHintMissingSHA384(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := tc.Must(c, uuid.NewUUID).String()
	metadata := objectstore.Metadata{
		Path:   path,
		SHA256: tc.Must(c, uuid.NewUUID).String(),
		Size:   666,
	}

	_, err := NewService(s.state).PutMetadataWithControllerIDHint(c.Context(), metadata, "1")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrMissingHash)
}

func (s *serviceSuite) TestPutMetadataWithControllerIDHintMissingSHA256(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := tc.Must(c, uuid.NewUUID).String()
	metadata := objectstore.Metadata{
		Path:   path,
		SHA384: tc.Must(c, uuid.NewUUID).String(),
		Size:   666,
	}

	_, err := NewService(s.state).PutMetadataWithControllerIDHint(c.Context(), metadata, "1")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrMissingHash)
}

func (s *serviceSuite) TestPutMetadataWithControllerIDHintMissingControllerID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := tc.Must(c, uuid.NewUUID).String()
	metadata := objectstore.Metadata{
		Path:   path,
		SHA256: tc.Must(c, uuid.NewUUID).String(),
		SHA384: tc.Must(c, uuid.NewUUID).String(),
		Size:   666,
	}

	_, err := NewService(s.state).PutMetadataWithControllerIDHint(c.Context(), metadata, "")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrMissingControllerID)
}

func (s *serviceSuite) TestAddControllerIDHint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerID := "1"
	sha384 := tc.Must(c, uuid.NewUUID).String()

	s.state.EXPECT().AddControllerIDHint(gomock.Any(), sha384, controllerID).Return(nil)

	err := NewService(s.state).AddControllerIDHint(c.Context(), sha384, controllerID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestAddControllerIDHintMissingHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerID := "1"

	err := NewService(s.state).AddControllerIDHint(c.Context(), "", controllerID)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrMissingHash)
}

func (s *serviceSuite) TestAddControllerIDHintMissingControllerNodeID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerID := ""
	sha384 := tc.Must(c, uuid.NewUUID).String()

	err := NewService(s.state).AddControllerIDHint(c.Context(), sha384, controllerID)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrMissingControllerID)
}

func (s *serviceSuite) TestGetControllerIDHints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sha384 := tc.Must(c, uuid.NewUUID).String()
	hints := []string{"1", "2", "3"}

	s.state.EXPECT().GetControllerIDHints(gomock.Any(), sha384).Return(hints, nil)

	result, err := NewService(s.state).GetControllerIDHints(c.Context(), sha384)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, hints)
}

func (s *serviceSuite) TestGetControllerIDHintsMissingHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetControllerIDHints(c.Context(), "")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrMissingHash)
}

func (s *serviceSuite) TestGetControllerIDHintsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sha384 := tc.Must(c, uuid.NewUUID).String()

	s.state.EXPECT().GetControllerIDHints(gomock.Any(), sha384).Return(nil, errors.Errorf("boom"))

	_, err := NewService(s.state).GetControllerIDHints(c.Context(), sha384)
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *serviceSuite) TestGetControllerIDHintsNoHints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sha384 := tc.Must(c, uuid.NewUUID).String()

	s.state.EXPECT().GetControllerIDHints(gomock.Any(), sha384).Return(nil, nil)

	_, err := NewService(s.state).GetControllerIDHints(c.Context(), sha384)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNoHints)
}

// Test watch returns a watcher that watches the specified path.
func (s *serviceSuite) TestWatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	watcher := watchertest.NewMockStringsWatcher(nil)
	defer workertest.DirtyKill(c, watcher)

	table := "objectstore"
	stmt := "SELECT key FROM objectstore"
	s.state.EXPECT().InitialWatchStatement().Return(table, stmt)

	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(watcher, nil)

	w, err := NewWatchableService(s.state, s.watcherFactory).Watch(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	c.Cleanup(func() {
		s.state = nil
		s.watcherFactory = nil
	})

	return ctrl
}

type drainingServiceSuite struct {
	testhelpers.IsolationSuite

	state          *MockDrainingState
	watcherFactory *MockWatcherFactory
}

func TestDrainingServiceSuite(t *testing.T) {
	tc.Run(t, &drainingServiceSuite{})
}

func (s *drainingServiceSuite) TestSetDrainingPhase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentPhase := objectstore.PhaseDraining
	newPhase := objectstore.PhaseCompleted
	uuid := tc.Must(c, objectstore.NewUUID)

	s.state.EXPECT().GetActiveDrainingInfo(gomock.Any()).Return(domainobjectstore.DrainingInfo{
		UUID:  uuid.String(),
		Phase: currentPhase.String(),
	}, nil)
	s.state.EXPECT().SetDrainingPhase(gomock.Any(), uuid.String(), newPhase).Return(nil)

	err := NewWatchableDrainingService(s.state, s.watcherFactory).SetDrainingPhase(c.Context(), newPhase)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *drainingServiceSuite) TestSetDrainingPhaseNoInitial(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newPhase := objectstore.PhaseDraining

	s.state.EXPECT().GetActiveDrainingInfo(gomock.Any()).Return(domainobjectstore.DrainingInfo{}, objectstoreerrors.ErrDrainingPhaseNotFound)
	s.state.EXPECT().StartDraining(gomock.Any(), gomock.Any()).Return(nil)

	err := NewWatchableDrainingService(s.state, s.watcherFactory).SetDrainingPhase(c.Context(), newPhase)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *drainingServiceSuite) TestSetDrainingPhaseInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	phase := objectstore.Phase("invalid")

	err := NewWatchableDrainingService(s.state, s.watcherFactory).SetDrainingPhase(c.Context(), phase)
	c.Assert(err, tc.ErrorMatches, "invalid phase \"invalid\"")
}

func (s *drainingServiceSuite) TestSetDrainingPhaseError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentPhase := objectstore.PhaseDraining
	newPhase := objectstore.PhaseCompleted
	uuid := tc.Must(c, objectstore.NewUUID)

	s.state.EXPECT().GetActiveDrainingInfo(gomock.Any()).Return(domainobjectstore.DrainingInfo{
		UUID:  uuid.String(),
		Phase: currentPhase.String(),
	}, nil)
	s.state.EXPECT().SetDrainingPhase(gomock.Any(), uuid.String(), newPhase).Return(errors.Errorf("boom"))

	err := NewWatchableDrainingService(s.state, s.watcherFactory).SetDrainingPhase(c.Context(), newPhase)
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *drainingServiceSuite) TestGetDrainingPhaseInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	phase := objectstore.PhaseDraining
	fromBackendUUID := tc.Must(c, objectstore.NewUUID).String()
	activeBackendUUID := tc.Must(c, objectstore.NewUUID).String()

	s.state.EXPECT().GetActiveDrainingInfo(gomock.Any()).Return(domainobjectstore.DrainingInfo{
		Phase:             phase.String(),
		FromBackendUUID:   &fromBackendUUID,
		ActiveBackendUUID: activeBackendUUID,
	}, nil)

	p, err := NewWatchableDrainingService(s.state, s.watcherFactory).GetDrainingPhaseInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	fb := objectstore.UUID(fromBackendUUID)
	c.Check(p, tc.DeepEquals, objectstore.DrainingPhaseInfo{
		Phase:             phase,
		FromBackendUUID:   &fb,
		ActiveBackendUUID: objectstore.UUID(activeBackendUUID),
	})
}

func (s *drainingServiceSuite) TestGetDrainingPhaseInfoError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	phase := objectstore.PhaseDraining

	s.state.EXPECT().GetActiveDrainingInfo(gomock.Any()).Return(domainobjectstore.DrainingInfo{Phase: phase.String()}, errors.Errorf("boom"))

	_, err := NewWatchableDrainingService(s.state, s.watcherFactory).GetDrainingPhaseInfo(c.Context())
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *drainingServiceSuite) TestGetActiveObjectStoreBackend(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, objectstore.NewUUID)
	endpoint := "https://s3.example.com"
	accessKey := "access-key"
	secretKey := "secret-key"
	sessionToken := "session-token"

	s.state.EXPECT().GetActiveObjectStoreBackend(gomock.Any()).Return(domainobjectstore.BackendInfo{
		UUID:            uuid.String(),
		ObjectStoreType: string(objectstore.S3Backend),
		LifeID:          life.Alive,
		Endpoint:        &endpoint,
		AccessKey:       &accessKey,
		SecretKey:       &secretKey,
		SessionToken:    &sessionToken,
	}, nil)

	info, err := NewWatchableDrainingService(s.state, s.watcherFactory).GetActiveObjectStoreBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.UUID, tc.Equals, uuid)
	c.Check(info.ObjectStoreType, tc.Equals, objectstore.S3Backend)
	c.Assert(info.Endpoint, tc.NotNil)
	c.Check(*info.Endpoint, tc.Equals, endpoint)
	c.Assert(info.AccessKey, tc.NotNil)
	c.Check(*info.AccessKey, tc.Equals, accessKey)
	c.Assert(info.SecretKey, tc.NotNil)
	c.Check(*info.SecretKey, tc.Equals, secretKey)
	c.Assert(info.SessionToken, tc.NotNil)
	c.Check(*info.SessionToken, tc.Equals, sessionToken)

	creds, ok := info.S3Credentials()
	c.Assert(ok, tc.IsTrue)
	c.Check(creds.Endpoint, tc.Equals, endpoint)
	c.Check(creds.AccessKey, tc.Equals, accessKey)
	c.Check(creds.SecretKey, tc.Equals, secretKey)
	c.Check(creds.SessionToken, tc.Equals, sessionToken)
}

func (s *drainingServiceSuite) TestGetActiveObjectStoreBackendError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetActiveObjectStoreBackend(gomock.Any()).Return(domainobjectstore.BackendInfo{}, errors.Errorf("boom"))

	_, err := NewWatchableDrainingService(s.state, s.watcherFactory).GetActiveObjectStoreBackend(c.Context())
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *drainingServiceSuite) TestGetObjectStoreBackend(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, objectstore.NewUUID)
	endpoint := "https://s3.example.com"
	accessKey := "access-key"
	secretKey := "secret-key"
	sessionToken := "session-token"

	backendInfo := domainobjectstore.BackendInfo{
		UUID:            uuid.String(),
		ObjectStoreType: "s3",
		Endpoint:        &endpoint,
		AccessKey:       &accessKey,
		SecretKey:       &secretKey,
		SessionToken:    &sessionToken,
	}

	s.state.EXPECT().GetObjectStoreBackend(gomock.Any(), uuid.String()).Return(backendInfo, nil)

	info, err := NewWatchableDrainingService(s.state, s.watcherFactory).GetObjectStoreBackend(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info, tc.DeepEquals, BackendInfo{
		UUID:            objectstore.UUID(backendInfo.UUID),
		ObjectStoreType: objectstore.BackendType(backendInfo.ObjectStoreType),
		Endpoint:        &endpoint,
		AccessKey:       &accessKey,
		SecretKey:       &secretKey,
		SessionToken:    &sessionToken,
	})

	creds, ok := info.S3Credentials()
	c.Assert(ok, tc.IsTrue)
	c.Check(creds.Endpoint, tc.Equals, endpoint)
	c.Check(creds.AccessKey, tc.Equals, accessKey)
	c.Check(creds.SecretKey, tc.Equals, secretKey)
	c.Check(creds.SessionToken, tc.Equals, sessionToken)
}

func (s *drainingServiceSuite) TestGetObjectStoreBackendError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, objectstore.NewUUID)

	s.state.EXPECT().GetObjectStoreBackend(gomock.Any(), uuid.String()).Return(domainobjectstore.BackendInfo{}, errors.Errorf("boom"))

	_, err := NewWatchableDrainingService(s.state, s.watcherFactory).GetObjectStoreBackend(c.Context(), uuid)
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *drainingServiceSuite) TestRemoveMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key := tc.Must(c, uuid.NewUUID).String()

	s.state.EXPECT().RemoveMetadata(gomock.Any(), key).Return(nil)

	err := NewWatchableDrainingService(s.state, s.watcherFactory).RemoveMetadata(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *drainingServiceSuite) TestSetObjectStoreBackendToS3(c *tc.C) {
	defer s.setupMocks(c).Finish()

	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access",
		SecretKey: "secret",
	}

	s.state.EXPECT().SetObjectStoreBackendToS3(gomock.Any(), gomock.Any(), creds).Return(nil)

	err := NewWatchableDrainingService(s.state, s.watcherFactory).SetObjectStoreBackendToS3(c.Context(), creds)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *drainingServiceSuite) TestSetObjectStoreBackendToS3InvalidCreds(c *tc.C) {
	defer s.setupMocks(c).Finish()

	creds := domainobjectstore.S3Credentials{}

	err := NewWatchableDrainingService(s.state, s.watcherFactory).SetObjectStoreBackendToS3(c.Context(), creds)
	c.Assert(err, tc.ErrorMatches, "validating S3 credentials: .*endpoint is required")
}

func (s *drainingServiceSuite) TestMarkObjectStoreBackendAsDrained(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backendUUID := tc.Must(c, objectstore.NewUUID).String()
	fromBackendUUID := tc.Must(c, objectstore.NewUUID).String()

	s.state.EXPECT().GetActiveDrainingInfo(gomock.Any()).Return(domainobjectstore.DrainingInfo{
		UUID:            backendUUID,
		Phase:           objectstore.PhaseDraining.String(),
		FromBackendUUID: &fromBackendUUID,
	}, nil)
	s.state.EXPECT().MarkObjectStoreBackendAsDrained(gomock.Any(), fromBackendUUID).Return(nil)

	err := NewWatchableDrainingService(s.state, s.watcherFactory).MarkObjectStoreBackendAsDrained(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *drainingServiceSuite) TestMarkObjectStoreBackendAsDrainedNotDraining(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backendUUID := tc.Must(c, objectstore.NewUUID).String()
	fromBackendUUID := tc.Must(c, objectstore.NewUUID).String()

	s.state.EXPECT().GetActiveDrainingInfo(gomock.Any()).Return(domainobjectstore.DrainingInfo{
		UUID:            backendUUID,
		Phase:           objectstore.PhaseUnknown.String(),
		FromBackendUUID: &fromBackendUUID,
	}, nil)

	err := NewWatchableDrainingService(s.state, s.watcherFactory).MarkObjectStoreBackendAsDrained(c.Context())
	c.Assert(err, tc.ErrorMatches, "cannot mark object store backend as drained when phase is \"unknown\"")
}

func (s *drainingServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockDrainingState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	c.Cleanup(func() {
		s.state = nil
		s.watcherFactory = nil
	})

	return ctrl
}
