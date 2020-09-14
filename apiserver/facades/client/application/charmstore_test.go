// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/client/application/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/testcharms"
)

type CharmStoreSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CharmStoreSuite{})

func (s *CharmStoreSuite) TestAddCharmWithAuthorization(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := "cs:~juju-qa/bionic/lxd-profile-0"
	charmURL, err := charm.ParseURL(url)
	c.Assert(err, gc.IsNil)

	mockState := mocks.NewMockState(ctrl)
	mockStateCharm := mocks.NewMockStateCharm(ctrl)
	mockStorage := mocks.NewMockStorage(ctrl)
	mockRepository := mocks.NewMockRepository(ctrl)

	ch := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile")

	// inject the mock as a back handed dependency
	s.PatchValue(application.NewStateStorage, func(uuid string, session *mgo.Session) storage.Storage {
		return mockStorage
	})

	sExp := mockState.EXPECT()
	sExp.PrepareCharmUpload(charmURL).Return(mockStateCharm, nil)
	sExp.ModelUUID().Return("model-id")
	sExp.MongoSession().Return(&mgo.Session{})
	sExp.UpdateUploadedCharm(gomock.Any()).Return(nil, nil)

	cExp := mockStateCharm.EXPECT()
	cExp.IsUploaded().Return(false)

	stExp := mockStorage.EXPECT()
	stExp.Put(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	iExp := mockRepository.EXPECT()
	iExp.DownloadCharm(charmURL.String(), gomock.Any()).Return(ch, nil)

	err = application.AddCharmWithAuthorizationAndRepo(mockState, params.AddCharmWithAuthorization{
		URL: url,
	}, func() (application.Repository, error) {
		return mockRepository, nil
	})
	c.Assert(err, gc.IsNil)
}

func (s *CharmStoreSuite) TestAddCharmWithAuthorizationWithInvalidLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := "cs:~juju-qa/bionic/lxd-profile-fail-0"
	charmURL, err := charm.ParseURL(url)
	c.Assert(err, gc.IsNil)

	mockState := mocks.NewMockState(ctrl)
	mockStateCharm := mocks.NewMockStateCharm(ctrl)
	mockStorage := mocks.NewMockStorage(ctrl)
	mockRepository := mocks.NewMockRepository(ctrl)

	ch := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile-fail")

	// inject the mock as a back handed dependency
	s.PatchValue(application.NewStateStorage, func(uuid string, session *mgo.Session) storage.Storage {
		return mockStorage
	})

	sExp := mockState.EXPECT()
	sExp.PrepareCharmUpload(charmURL).Return(mockStateCharm, nil)

	cExp := mockStateCharm.EXPECT()
	cExp.IsUploaded().Return(false)

	iExp := mockRepository.EXPECT()
	iExp.DownloadCharm(charmURL.String(), gomock.Any()).Return(ch, nil)

	err = application.AddCharmWithAuthorizationAndRepo(mockState, params.AddCharmWithAuthorization{
		URL: url,
	}, func() (application.Repository, error) {
		return mockRepository, nil
	})
	c.Assert(err, gc.ErrorMatches, "cannot add charm: invalid lxd-profile.yaml: contains device type \"unix-disk\"")
}

func (s *CharmStoreSuite) TestAddCharmWithAuthorizationAndForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := "cs:~juju-qa/bionic/lxd-profile-0"
	charmURL, err := charm.ParseURL(url)
	c.Assert(err, gc.IsNil)

	mockState := mocks.NewMockState(ctrl)
	mockStateCharm := mocks.NewMockStateCharm(ctrl)
	mockStorage := mocks.NewMockStorage(ctrl)
	mockRepository := mocks.NewMockRepository(ctrl)

	ch := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile")

	// inject the mock as a back handed dependency
	s.PatchValue(application.NewStateStorage, func(uuid string, session *mgo.Session) storage.Storage {
		return mockStorage
	})

	sExp := mockState.EXPECT()
	sExp.PrepareCharmUpload(charmURL).Return(mockStateCharm, nil)
	sExp.ModelUUID().Return("model-id")
	sExp.MongoSession().Return(&mgo.Session{})
	sExp.UpdateUploadedCharm(gomock.Any()).Return(nil, nil)

	cExp := mockStateCharm.EXPECT()
	cExp.IsUploaded().Return(false)

	stExp := mockStorage.EXPECT()
	stExp.Put(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	iExp := mockRepository.EXPECT()
	iExp.DownloadCharm(charmURL.String(), gomock.Any()).Return(ch, nil)

	err = application.AddCharmWithAuthorizationAndRepo(mockState, params.AddCharmWithAuthorization{
		URL:   url,
		Force: true,
	}, func() (application.Repository, error) {
		return mockRepository, nil
	})
	c.Assert(err, gc.IsNil)
}

func (s *CharmStoreSuite) TestAddCharmWithAuthorizationWithInvalidLXDProfileAndForceStilSucceeds(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := "cs:~juju-qa/bionic/lxd-profile-fail-0"
	charmURL, err := charm.ParseURL(url)
	c.Assert(err, gc.IsNil)

	mockState := mocks.NewMockState(ctrl)
	mockStateCharm := mocks.NewMockStateCharm(ctrl)
	mockStorage := mocks.NewMockStorage(ctrl)
	mockRepository := mocks.NewMockRepository(ctrl)

	ch := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile-fail")

	// inject the mock as a back handed dependency
	s.PatchValue(application.NewStateStorage, func(uuid string, session *mgo.Session) storage.Storage {
		return mockStorage
	})

	sExp := mockState.EXPECT()
	sExp.PrepareCharmUpload(charmURL).Return(mockStateCharm, nil)
	sExp.ModelUUID().Return("model-id")
	sExp.MongoSession().Return(&mgo.Session{})
	sExp.UpdateUploadedCharm(gomock.Any()).Return(nil, nil)

	cExp := mockStateCharm.EXPECT()
	cExp.IsUploaded().Return(false)

	stExp := mockStorage.EXPECT()
	stExp.Put(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	iExp := mockRepository.EXPECT()
	iExp.DownloadCharm(charmURL.String(), gomock.Any()).Return(ch, nil)

	err = application.AddCharmWithAuthorizationAndRepo(mockState, params.AddCharmWithAuthorization{
		URL:   url,
		Force: true,
	}, func() (application.Repository, error) {
		return mockRepository, nil
	})
	c.Assert(err, gc.IsNil)
}

func (s *CharmStoreSuite) TestAddVersionedCharmWithAuthorization(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := "cs:~juju-qa/bionic/versioned-0"
	charmURL, err := charm.ParseURL(url)
	c.Assert(err, gc.IsNil)

	mockState := mocks.NewMockState(ctrl)
	mockStateCharm := mocks.NewMockStateCharm(ctrl)
	mockStorage := mocks.NewMockStorage(ctrl)
	mockRepository := mocks.NewMockRepository(ctrl)

	expVersion := "929903d"
	pathToArchive := testcharms.Repo.CharmArchivePath(c.MkDir(), "versioned")
	err = testcharms.InjectFilesToCharmArchive(pathToArchive, map[string]string{
		"version": expVersion,
	})
	c.Assert(err, gc.IsNil)
	ch, err := charm.ReadCharmArchive(pathToArchive)
	c.Assert(err, gc.IsNil)

	// inject the mock as a back handed dependency
	s.PatchValue(application.NewStateStorage, func(uuid string, session *mgo.Session) storage.Storage {
		return mockStorage
	})

	sExp := mockState.EXPECT()
	sExp.PrepareCharmUpload(charmURL).Return(mockStateCharm, nil)
	sExp.ModelUUID().Return("model-id")
	sExp.MongoSession().Return(&mgo.Session{})
	sExp.UpdateUploadedCharm(charmVersionMatcher{expVersion}).Return(nil, nil)

	cExp := mockStateCharm.EXPECT()
	cExp.IsUploaded().Return(false)

	stExp := mockStorage.EXPECT()
	stExp.Put(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	iExp := mockRepository.EXPECT()
	iExp.DownloadCharm(charmURL.String(), gomock.Any()).Return(ch, nil)

	err = application.AddCharmWithAuthorizationAndRepo(mockState, params.AddCharmWithAuthorization{
		URL: url,
	}, func() (application.Repository, error) {
		return mockRepository, nil
	})
	c.Assert(err, gc.IsNil)
}

type charmVersionMatcher struct {
	expVersion string
}

func (m charmVersionMatcher) Matches(x interface{}) bool {
	info, ok := x.(state.CharmInfo)
	if !ok {
		return false
	}

	return info.Version == m.expVersion
}

func (m charmVersionMatcher) String() string {
	return fmt.Sprintf("state.CharmInfo.Version == %q", m.expVersion)
}
