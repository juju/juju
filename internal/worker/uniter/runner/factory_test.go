// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	stdcontext "context"
	"strings"

	"github.com/juju/charm/v12/hooks"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/runner"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/rpc/params"
)

type FactorySuite struct {
	ContextSuite
}

var _ = gc.Suite(&FactorySuite{})

func (s *FactorySuite) AssertPaths(c *gc.C, rnr runner.Runner) {
	c.Assert(runner.RunnerPaths(rnr), gc.DeepEquals, s.paths)
}

func (s *FactorySuite) TestNewCommandRunnerNoRelation(c *gc.C) {
	rnr, err := s.factory.NewCommandRunner(stdcontext.Background(), context.CommandInfo{RelationId: -1})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewCommandRunnerRelationIdDoesNotExist(c *gc.C) {
	for _, value := range []bool{true, false} {
		_, err := s.factory.NewCommandRunner(stdcontext.Background(), context.CommandInfo{
			RelationId: 12, ForceRemoteUnit: value,
		})
		c.Check(err, gc.ErrorMatches, `unknown relation id: 12`)
	}
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitInvalid(c *gc.C) {
	for _, value := range []bool{true, false} {
		_, err := s.factory.NewCommandRunner(stdcontext.Background(), context.CommandInfo{
			RelationId: 0, RemoteUnitName: "blah", ForceRemoteUnit: value,
		})
		c.Check(err, gc.ErrorMatches, `invalid remote unit: blah`)
	}
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitInappropriate(c *gc.C) {
	for _, value := range []bool{true, false} {
		_, err := s.factory.NewCommandRunner(stdcontext.Background(), context.CommandInfo{
			RelationId: -1, RemoteUnitName: "blah/123", ForceRemoteUnit: value,
		})
		c.Check(err, gc.ErrorMatches, `remote unit provided without a relation: blah/123`)
	}
}

func (s *FactorySuite) TestNewCommandRunnerEmptyRelation(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupFactory(c, ctrl)

	_, err := s.factory.NewCommandRunner(stdcontext.Background(), context.CommandInfo{RelationId: 1})
	c.Check(err, gc.ErrorMatches, `cannot infer remote unit in empty relation 1`)
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitAmbiguous(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupFactory(c, ctrl)

	s.membership[1] = []string{"foo/0", "foo/1"}
	_, err := s.factory.NewCommandRunner(stdcontext.Background(), context.CommandInfo{RelationId: 1})
	c.Check(err, gc.ErrorMatches, `ambiguous remote unit; possibilities are \[foo/0 foo/1\]`)
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitMissing(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupFactory(c, ctrl)

	s.membership[0] = []string{"foo/0", "foo/1"}
	_, err := s.factory.NewCommandRunner(stdcontext.Background(), context.CommandInfo{
		RelationId: 0, RemoteUnitName: "blah/123",
	})
	c.Check(err, gc.ErrorMatches, `unknown remote unit blah/123; possibilities are \[foo/0 foo/1\]`)
}

func (s *FactorySuite) TestNewCommandRunnerForceNoRemoteUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.setupFactory(c, ctrl)
	rnr, err := s.factory.NewCommandRunner(stdcontext.Background(), context.CommandInfo{
		RelationId: 0, ForceRemoteUnit: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewCommandRunnerForceRemoteUnitMissing(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupFactory(c, ctrl)

	_, err := s.factory.NewCommandRunner(stdcontext.Background(), context.CommandInfo{
		RelationId: 0, RemoteUnitName: "blah/123", ForceRemoteUnit: true,
	})
	c.Assert(err, gc.IsNil)
}

func (s *FactorySuite) TestNewCommandRunnerInferRemoteUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupFactory(c, ctrl)

	s.membership[0] = []string{"foo/2"}
	rnr, err := s.factory.NewCommandRunner(stdcontext.Background(), context.CommandInfo{RelationId: 0})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewHookRunner(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(stdcontext.Background(), hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewHookRunnerWithBadHook(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(stdcontext.Background(), hook.Info{})
	c.Assert(rnr, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `unknown hook kind ""`)
}

func (s *FactorySuite) TestNewHookRunnerWithStorage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupFactory(c, ctrl)

	s.uniter.EXPECT().StorageAttachment(names.NewStorageTag("data/0"), names.NewUnitTag("u/0")).Return(params.StorageAttachment{
		Kind:     params.StorageKindBlock,
		Location: "/dev/sdb",
	}, nil).AnyTimes()

	rnr, err := s.factory.NewHookRunner(stdcontext.Background(), hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: "data/0",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	c.Assert(ctx, gc.NotNil)
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
}

func (s *FactorySuite) TestNewHookRunnerWithRelation(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupFactory(c, ctrl)

	rnr, err := s.factory.NewHookRunner(stdcontext.Background(), hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewHookRunnerWithBadRelation(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(stdcontext.Background(), hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 12345,
	})
	c.Assert(rnr, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `unknown relation id: 12345`)
}

func (s *FactorySuite) TestNewActionRunnerGood(c *gc.C) {
	for i, test := range []struct {
		actionName string
		payload    map[string]interface{}
	}{
		{
			actionName: "snapshot",
			payload: map[string]interface{}{
				"outfile": "/some/file.bz2",
			},
		},
		{
			// juju-exec should work as a predefined action even if
			// it's not part of the charm
			actionName: "juju-exec",
			payload: map[string]interface{}{
				"command": "foo",
				"timeout": 0.0,
			},
		},
	} {
		c.Logf("test %d", i)
		ctrl := gomock.NewController(c)
		s.setupFactory(c, ctrl)

		actionTag := names.NewActionTag("2")
		action := apiuniter.NewAction(
			actionTag.ID,
			test.actionName,
			test.payload,
			false,
			"",
		)
		s.setCharm(c, "dummy")
		rnr, err := s.factory.NewActionRunner(stdcontext.Background(), action, nil)
		c.Assert(err, jc.ErrorIsNil)
		s.AssertPaths(c, rnr)
		ctx := rnr.Context()
		data, err := ctx.ActionData()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(data, jc.DeepEquals, &context.ActionData{
			Name:       test.actionName,
			Tag:        actionTag,
			Params:     test.payload,
			ResultsMap: map[string]interface{}{},
		})
		vars, err := ctx.HookVars(s.paths, false, context.NewRemoteEnvironmenter(
			func() []string { return []string{} },
			func(k string) string {
				switch k {
				case "PATH", "Path":
					return "pathy"
				}
				return ""
			},
			func(k string) (string, bool) {
				switch k {
				case "PATH", "Path":
					return "pathy", true
				}
				return "", false
			},
		))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(len(vars) > 0, jc.IsTrue, gc.Commentf("expected HookVars but found none"))
		combined := strings.Join(vars, "|")
		c.Assert(combined, gc.Matches, `(^|.*\|)JUJU_ACTION_NAME=`+test.actionName+`(\|.*|$)`)
		c.Assert(combined, gc.Matches, `(^|.*\|)JUJU_ACTION_UUID=`+actionTag.Id()+`(\|.*|$)`)
		c.Assert(combined, gc.Matches, `(^|.*\|)JUJU_ACTION_TAG=`+actionTag.String()+`(\|.*|$)`)
		ctrl.Finish()
	}
}

func (s *FactorySuite) TestNewActionRunnerBadName(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupFactory(c, ctrl)

	s.setCharm(c, "dummy")
	action := apiuniter.NewAction("666", "no-such-action", nil, false, "")
	rnr, err := s.factory.NewActionRunner(stdcontext.Background(), action, nil)
	c.Check(rnr, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "cannot run \"no-such-action\" action: not defined")
	c.Check(err, jc.Satisfies, charmrunner.IsBadActionError)
}

func (s *FactorySuite) TestNewActionRunnerBadParams(c *gc.C) {
	action := apiuniter.NewAction("666", "snapshot", map[string]interface{}{
		"outfile": 123,
	}, true, "group")
	rnr, err := s.factory.NewActionRunner(stdcontext.Background(), action, nil)
	c.Check(rnr, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "cannot run \"snapshot\" action: .*")
	c.Check(err, jc.Satisfies, charmrunner.IsBadActionError)
}

func (s *FactorySuite) TestNewActionRunnerWithCancel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupFactory(c, ctrl)

	actionName := "snapshot"
	payload := map[string]interface{}{
		"outfile": "/some/file.bz2",
	}
	cancel := make(chan struct{})

	actionTag := names.NewActionTag("2")
	action := apiuniter.NewAction(
		actionTag.ID,
		actionName,
		payload,
		false,
		"",
	)
	s.setCharm(c, "dummy")
	rnr, err := s.factory.NewActionRunner(stdcontext.Background(), action, cancel)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	data, err := ctx.ActionData()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, &context.ActionData{
		Name:       actionName,
		Tag:        actionTag,
		Params:     payload,
		ResultsMap: map[string]interface{}{},
		Cancel:     cancel,
	})
	vars, err := ctx.HookVars(s.paths, false, context.NewRemoteEnvironmenter(
		func() []string { return []string{} },
		func(k string) string {
			switch k {
			case "PATH", "Path":
				return "pathy"
			}
			return ""
		},
		func(k string) (string, bool) {
			switch k {
			case "PATH", "Path":
				return "pathy", true
			}
			return "", false
		},
	))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(vars) > 0, jc.IsTrue, gc.Commentf("expected HookVars but found none"))
	combined := strings.Join(vars, "|")
	c.Assert(combined, gc.Matches, `(^|.*\|)JUJU_ACTION_NAME=`+actionName+`(\|.*|$)`)
	c.Assert(combined, gc.Matches, `(^|.*\|)JUJU_ACTION_UUID=`+actionTag.ID+`(\|.*|$)`)
	c.Assert(combined, gc.Matches, `(^|.*\|)JUJU_ACTION_TAG=`+actionTag.String()+`(\|.*|$)`)
}
