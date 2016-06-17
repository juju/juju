// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	"time"

	//"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/logfwd"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type LastSentSuite struct {
	jujutesting.JujuConnSuite

	conn api.Connection
	// This is a raw State object. Use it for setup and assertions, but
	// should never be touched by the API calls themselves
	//rawMachine *state.Machine
}

var _ = gc.Suite(&LastSentSuite{})

func (s *LastSentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	//s.conn, s.rawMachine = s.OpenAPIAsNewMachine(c)
	s.conn, _ = s.OpenAPIAsNewMachine(c)
}

func (s *LastSentSuite) TestGet(c *gc.C) {
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model)
	s.addModel(c, "other-model", model)
	tsSpam := time.Unix(12345, 0)
	tsEggs := time.Unix(12345, 54321)
	s.setLastSent(c, modelTag, "spam", tsSpam)
	s.setLastSent(c, modelTag, "eggs", tsEggs)
	client := logfwd.NewLastSentClient(s.conn)

	results, err := client.GetBulk([]logfwd.LastSentID{{
		Model: modelTag,
		Sink:  "spam",
	}, {
		Model: modelTag,
		Sink:  "eggs",
	}, {
		Model: modelTag,
		Sink:  "ham",
	}})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, []logfwd.LastSentResult{{
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "spam",
			},
			Timestamp: tsSpam.UTC(),
		},
	}, {
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "eggs",
			},
			Timestamp: tsEggs.UTC(),
		},
	}, {
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "ham",
			},
			Timestamp: time.Time{}.UTC(),
		},
		Error: common.RestoreError(&params.Error{
			Message: `cannot find timestamp of the last forwarded record`,
			Code:    params.CodeNotFound,
		}),
		//Error: errors.NewNotFound(nil, `cannot find timestamp of the last forwarded record`),
	}})
}

func (s *LastSentSuite) TestSet(c *gc.C) {
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model)
	s.addModel(c, "other-model", model)
	tsSpam := time.Unix(12345, 0)
	tsEggs := time.Unix(12345, 54321)
	client := logfwd.NewLastSentClient(s.conn)

	results, err := client.SetBulk([]logfwd.LastSentInfo{{
		LastSentID: logfwd.LastSentID{
			Model: modelTag,
			Sink:  "spam",
		},
		Timestamp: tsSpam,
	}, {
		LastSentID: logfwd.LastSentID{
			Model: modelTag,
			Sink:  "eggs",
		},
		Timestamp: tsEggs,
	}})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, []logfwd.LastSentResult{{
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "spam",
			},
			Timestamp: tsSpam,
		},
	}, {
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "eggs",
			},
			Timestamp: tsEggs,
		},
	}})
	ts := s.getLastSent(c, modelTag, "spam")
	c.Check(ts, jc.DeepEquals, tsSpam.UTC())
	ts = s.getLastSent(c, modelTag, "eggs")
	c.Check(ts, jc.DeepEquals, tsEggs.UTC())
}

func (s *LastSentSuite) addModel(c *gc.C, name, uuid string) {
	_, modelState, err := s.State.NewModel(state.ModelArgs{
		Config: testing.CustomModelConfig(c, testing.Attrs{
			"name": name,
			"uuid": uuid,
		}),
		Owner: s.AdminUserTag(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { modelState.Close() })
}

func (s *LastSentSuite) getLastSent(c *gc.C, model names.ModelTag, sink string) time.Time {
	st, err := s.State.ForModel(model)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	lsl := state.NewLastSentLogger(st, sink)

	ts, err := lsl.Get()
	c.Assert(err, jc.ErrorIsNil)
	return ts
}

func (s *LastSentSuite) setLastSent(c *gc.C, model names.ModelTag, sink string, timestamp time.Time) {
	st, err := s.State.ForModel(model)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	lsl := state.NewLastSentLogger(st, sink)

	err = lsl.Set(timestamp)
	c.Assert(err, jc.ErrorIsNil)
}
