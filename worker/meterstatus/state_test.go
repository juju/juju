// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"path"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/meterstatus"
	"github.com/juju/juju/worker/meterstatus/mocks"
)

type DiskBackedStateSuite struct {
	path  string
	state *meterstatus.DiskBackedState
}

var _ = gc.Suite(&DiskBackedStateSuite{})

func (t *DiskBackedStateSuite) SetUpTest(c *gc.C) {
	t.path = path.Join(c.MkDir(), "state.yaml")
	t.state = meterstatus.NewDiskBackedState(t.path)
}

func (t *DiskBackedStateSuite) TestReadNonExist(c *gc.C) {
	_, err := t.state.Read()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (t *DiskBackedStateSuite) TestWriteRead(c *gc.C) {
	initial := &meterstatus.State{
		Code: "GREEN",
		Info: "some message",
	}
	err := t.state.Write(initial)
	c.Assert(err, jc.ErrorIsNil)

	st, err := t.state.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.Code, gc.Equals, initial.Code)
	c.Assert(st.Info, gc.Equals, initial.Info)
}

func (t *DiskBackedStateSuite) TestWriteReadExtra(c *gc.C) {
	initial := &meterstatus.State{
		Code: "GREEN",
		Info: "some message",
	}
	err := t.state.Write(initial)
	c.Assert(err, jc.ErrorIsNil)

	st, err := t.state.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.Code, gc.Equals, initial.Code)
	c.Assert(st.Info, gc.Equals, initial.Info)
	c.Assert(st.Disconnected, gc.IsNil)

	st.Disconnected = &meterstatus.Disconnected{
		Disconnected: time.Now().Unix(),
		State:        meterstatus.WaitingRed,
	}

	err = t.state.Write(st)
	c.Assert(err, jc.ErrorIsNil)

	newSt, err := t.state.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newSt.Code, gc.Equals, st.Code)
	c.Assert(newSt.Info, gc.Equals, st.Info)
	c.Assert(newSt.Disconnected, gc.DeepEquals, st.Disconnected)
}

type ControllerBackedStateSuite struct {
}

var _ = gc.Suite(&ControllerBackedStateSuite{})

func (t *ControllerBackedStateSuite) TestReadNonExist(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := mocks.NewMockUnitStateAPI(ctrl)
	api.EXPECT().State().Return(params.UnitStateResult{}, nil)

	stateReader := meterstatus.NewControllerBackedState(api)
	_, err := stateReader.Read()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (t *ControllerBackedStateSuite) TestRead(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	exp := &meterstatus.State{
		Code: "code",
		Info: "info",
		Disconnected: &meterstatus.Disconnected{
			Disconnected: 123,
			State:        meterstatus.WaitingRed,
		},
	}

	data, err := yaml.Marshal(exp)
	c.Assert(err, jc.ErrorIsNil)

	api := mocks.NewMockUnitStateAPI(ctrl)
	api.EXPECT().State().Return(params.UnitStateResult{MeterStatusState: string(data)}, nil)

	stateReader := meterstatus.NewControllerBackedState(api)
	got, err := stateReader.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, exp)
}

func (t *ControllerBackedStateSuite) TestWrite(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := &meterstatus.State{
		Code: "code",
		Info: "info",
		Disconnected: &meterstatus.Disconnected{
			Disconnected: 123,
		},
	}

	api := mocks.NewMockUnitStateAPI(ctrl)
	api.EXPECT().SetState(expUnitStateArg(c, st)).Return(nil)

	stateReadWriter := meterstatus.NewControllerBackedState(api)
	err := stateReadWriter.Write(st)
	c.Assert(err, jc.ErrorIsNil)
}

func expUnitStateArg(c *gc.C, st *meterstatus.State) params.SetUnitStateArg {
	data, err := yaml.Marshal(st)
	c.Assert(err, jc.ErrorIsNil)

	dataStr := string(data)
	return params.SetUnitStateArg{
		MeterStatusState: &dataStr,
	}
}
