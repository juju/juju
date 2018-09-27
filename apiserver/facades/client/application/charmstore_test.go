package application_test

import (
	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"
	charm "gopkg.in/juju/charm.v6"
	charmrepo "gopkg.in/juju/charmrepo.v3"
	"gopkg.in/mgo.v2"

	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/client/application/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/storage"
	jujutesting "github.com/juju/juju/testing"
)

type CharmStoreSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CharmStoreSuite{})

func (s *CharmStoreSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.PatchValue(&charmrepo.CacheDir, c.MkDir())
}

func (s *CharmStoreSuite) TestAddCharmWithAuthorization(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := "cs:~juju-qa/bionic/lxd-profile-0"
	charmURL, err := charm.ParseURL(url)
	c.Assert(err, gc.IsNil)

	controllerConfig := map[string]interface{}{
		controller.CharmStoreURL: "",
	}
	config, err := config.New(config.NoDefaults, jujutesting.FakeConfig())
	c.Assert(err, gc.IsNil)

	mockState := mocks.NewMockState(ctrl)
	mockStateCharm := mocks.NewMockStateCharm(ctrl)
	mockStateModel := mocks.NewMockStateModel(ctrl)
	mockStorage := mocks.NewMockStorage(ctrl)

	// inject the mock as a back handed dependency
	s.PatchValue(application.NewStateStorage, func(uuid string, session *mgo.Session) storage.Storage {
		return mockStorage
	})

	sExp := mockState.EXPECT()
	sExp.PrepareStoreCharmUpload(charmURL).Return(mockStateCharm, nil)
	sExp.ControllerConfig().Return(controllerConfig, nil)
	sExp.Model().Return(mockStateModel, nil)
	sExp.ModelUUID().Return("model-id")
	sExp.MongoSession().Return(&mgo.Session{})
	sExp.UpdateUploadedCharm(gomock.Any()).Return(nil, nil)

	cExp := mockStateCharm.EXPECT()
	cExp.IsUploaded().Return(false)

	mExp := mockStateModel.EXPECT()
	mExp.ModelConfig().Return(config, nil)

	stExp := mockStorage.EXPECT()
	stExp.Put(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	err = application.AddCharmWithAuthorization(mockState, params.AddCharmWithAuthorization{
		URL: url,
	})
	c.Assert(err, gc.IsNil)
}
