// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
)

type strategySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&strategySuite{})

func (s strategySuite) TestValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("cs:redis-0")

	mockStore := NewMockStore(ctrl)
	mockStore.EXPECT().Validate(curl).Return(nil)

	strategy := &Strategy{
		charmURL: curl,
		store:    mockStore,
	}
	err := strategy.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s strategySuite) TestValidateWithError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("cs:redis-0")

	mockStore := NewMockStore(ctrl)
	mockStore.EXPECT().Validate(curl).Return(errors.New("boom"))

	strategy := &Strategy{
		charmURL: curl,
		store:    mockStore,
	}
	err := strategy.Validate()
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s strategySuite) TestDownloadResult(c *gc.C) {
	file, err := ioutil.TempFile("", "foo")
	c.Assert(err, jc.ErrorIsNil)

	fmt.Fprintln(file, "meshuggah")
	err = file.Sync()
	c.Assert(err, jc.ErrorIsNil)

	strategy := &Strategy{}
	result, err := strategy.downloadResult(file.Name(), AlwaysChecksum)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.SHA256, gc.Equals, "4e97ed7423be2ea12939e8fdd592cfb3dcd4d0097d7d193ef998ab6b4db70461")
	c.Assert(result.Size, gc.Equals, int64(10))
}

func (s strategySuite) TestDownloadResultWithOpenError(c *gc.C) {
	strategy := &Strategy{}
	_, err := strategy.downloadResult("foo-123", AlwaysChecksum)
	c.Assert(err, gc.ErrorMatches, "cannot read downloaded charm: open foo-123: no such file or directory")
}

func (s strategySuite) TestRunWithCharmAlreadyUploaded(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("cs:redis-0")

	mockStore := NewMockStore(ctrl)
	mockVersionValidator := NewMockVersionValidator(ctrl)

	mockStateCharm := NewMockStateCharm(ctrl)
	mockStateCharm.EXPECT().IsUploaded().Return(true)

	mockState := NewMockState(ctrl)
	mockState.EXPECT().PrepareCharmUpload(curl).Return(mockStateCharm, nil)

	strategy := &Strategy{
		charmURL: curl,
		store:    mockStore,
	}
	_, alreadyExists, err := strategy.Run(mockState, mockVersionValidator)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alreadyExists, jc.IsTrue)
}

func (s strategySuite) TestRunWithPrepareUploadError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("cs:redis-0")

	mockStore := NewMockStore(ctrl)
	mockVersionValidator := NewMockVersionValidator(ctrl)

	mockState := NewMockState(ctrl)
	mockState.EXPECT().PrepareCharmUpload(curl).Return(nil, errors.New("boom"))

	strategy := &Strategy{
		charmURL: curl,
		store:    mockStore,
	}
	_, alreadyExists, err := strategy.Run(mockState, mockVersionValidator)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(alreadyExists, jc.IsFalse)
}

func (s strategySuite) TestRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("cs:redis-0")
	meta := &charm.Meta{
		MinJujuVersion: version.Number{Major: 2},
	}

	mockVersionValidator := NewMockVersionValidator(ctrl)
	mockVersionValidator.EXPECT().Validate(meta).Return(nil)

	mockStateCharm := NewMockStateCharm(ctrl)
	mockStateCharm.EXPECT().IsUploaded().Return(false)

	mockStoreCharm := NewMockStoreCharm(ctrl)
	mockStoreCharm.EXPECT().Meta().Return(meta)

	// We're replicating a charm without a LXD profile here and ensuring it
	// correctly handles nil.
	mockStoreCharm.EXPECT().LXDProfile().Return(nil)

	mockState := NewMockState(ctrl)
	mockState.EXPECT().PrepareCharmUpload(curl).Return(mockStateCharm, nil)

	mockStore := NewMockStore(ctrl)
	mockStore.EXPECT().Download(curl, gomock.Any()).DoAndReturn(mustWriteToTempFile(c, mockStoreCharm))

	strategy := &Strategy{
		charmURL: curl,
		store:    mockStore,
	}
	_, alreadyExists, err := strategy.Run(mockState, mockVersionValidator)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alreadyExists, jc.IsFalse)
}

func (s strategySuite) TestRunWithInvalidLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("cs:redis-0")
	meta := &charm.Meta{
		MinJujuVersion: version.Number{Major: 2},
	}

	mockVersionValidator := NewMockVersionValidator(ctrl)
	mockVersionValidator.EXPECT().Validate(meta).Return(nil)

	mockStateCharm := NewMockStateCharm(ctrl)
	mockStateCharm.EXPECT().IsUploaded().Return(false)

	mockStoreCharm := NewMockStoreCharm(ctrl)
	mockStoreCharm.EXPECT().Meta().Return(meta)

	// Handle a failure from LXDProfiles
	lxdProfile := &charm.LXDProfile{
		Config: map[string]string{
			"boot": "",
		},
	}

	mockStoreCharm.EXPECT().LXDProfile().Return(lxdProfile)

	mockState := NewMockState(ctrl)
	mockState.EXPECT().PrepareCharmUpload(curl).Return(mockStateCharm, nil)

	mockStore := NewMockStore(ctrl)
	mockStore.EXPECT().Download(curl, gomock.Any()).DoAndReturn(mustWriteToTempFile(c, mockStoreCharm))

	strategy := &Strategy{
		charmURL: curl,
		store:    mockStore,
	}
	_, alreadyExists, err := strategy.Run(mockState, mockVersionValidator)
	c.Assert(err, gc.ErrorMatches, `cannot add charm: invalid lxd-profile.yaml: contains config value "boot"`)
	c.Assert(alreadyExists, jc.IsFalse)
}

func (s strategySuite) TestFinishAfterRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("cs:redis-0")
	meta := &charm.Meta{
		MinJujuVersion: version.Number{Major: 2},
	}

	mockVersionValidator := NewMockVersionValidator(ctrl)
	mockVersionValidator.EXPECT().Validate(meta).Return(nil)

	mockStateCharm := NewMockStateCharm(ctrl)
	mockStateCharm.EXPECT().IsUploaded().Return(false)

	mockStoreCharm := NewMockStoreCharm(ctrl)
	mockStoreCharm.EXPECT().Meta().Return(meta)
	mockStoreCharm.EXPECT().LXDProfile().Return(nil)

	mockState := NewMockState(ctrl)
	mockState.EXPECT().PrepareCharmUpload(curl).Return(mockStateCharm, nil)

	var tmpFile string

	mockStore := NewMockStore(ctrl)
	mockStore.EXPECT().Download(curl, gomock.Any()).DoAndReturn(func(curl *charm.URL, file string) (StoreCharm, Checksum, error) {
		tmpFile = file
		return mustWriteToTempFile(c, mockStoreCharm)(curl, file)
	})

	strategy := &Strategy{
		charmURL: curl,
		store:    mockStore,
	}
	_, alreadyExists, err := strategy.Run(mockState, mockVersionValidator)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alreadyExists, jc.IsFalse)

	err = strategy.Finish()
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Stat(tmpFile)
	c.Assert(os.IsNotExist(err), jc.IsTrue)
}

func mustWriteToTempFile(c *gc.C, mockCharm *MockStoreCharm) func(*charm.URL, string) (StoreCharm, Checksum, error) {
	return func(curl *charm.URL, file string) (StoreCharm, Checksum, error) {
		f, err := os.Open(file)
		c.Assert(err, jc.ErrorIsNil)

		fmt.Fprintln(f, "meshuggah")
		err = f.Sync()
		c.Assert(err, jc.ErrorIsNil)

		return mockCharm, AlwaysChecksum, nil
	}
}
