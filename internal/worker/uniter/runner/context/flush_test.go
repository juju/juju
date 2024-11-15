// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	stdcontext "context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/rpc/params"
)

const allEndpoints = ""

type FlushContextSuite struct {
	BaseHookContextSuite
	stub testing.Stub
}

var _ = gc.Suite(&FlushContextSuite{})

func (s *FlushContextSuite) SetUpTest(c *gc.C) {
	s.BaseHookContextSuite.SetUpTest(c)
	s.stub.ResetCalls()
}

func (s *FlushContextSuite) TestRunHookRelationFlushingError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.context(c, ctrl)

	// Mess with multiple relation settings.
	relCtx0, err := ctx.Relation(0)
	c.Assert(err, jc.ErrorIsNil)
	node0, err := relCtx0.Settings(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	node0.Set("foo", "1")
	relCtx1, err := ctx.Relation(1)
	c.Assert(err, jc.ErrorIsNil)
	node1, err := relCtx1.Settings(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	node1.Set("bar", "2")

	// Flush the context with a failure.
	err = ctx.Flush(stdcontext.Background(), "some badge", errors.New("blam pow"))
	c.Assert(err, gc.ErrorMatches, "blam pow")
}

func (s *FlushContextSuite) TestRunHookRelationFlushingSuccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.context(c, ctrl)

	// Mess with multiple relation settings.
	relCtx0, err := ctx.Relation(0)
	c.Assert(err, jc.ErrorIsNil)
	node0, err := relCtx0.Settings(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	node0.Set("baz", "3")
	relCtx1, err := ctx.Relation(1)
	c.Assert(err, jc.ErrorIsNil)
	node1, err := relCtx1.Settings(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	node1.Set("qux", "4")

	arg := params.CommitHookChangesArg{
		Tag: s.unit.Tag().String(),
		RelationUnitSettings: []params.RelationUnitSettings{{
			Relation:            "relation-mysql.server#wordpress.db0",
			Unit:                s.unit.Tag().String(),
			Settings:            params.Settings{"baz": "3"},
			ApplicationSettings: nil,
		}, {
			Relation:            "relation-mysql.server#wordpress.db1",
			Unit:                s.unit.Tag().String(),
			Settings:            params.Settings{"qux": "4"},
			ApplicationSettings: nil,
		}},
	}

	s.unit.EXPECT().CommitHookChanges(gomock.Any(), hookCommitMatcher{c: c, expected: params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{arg},
	}}).Return(nil)

	// Flush the context with a success.
	err = ctx.Flush(stdcontext.Background(), "some badge", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FlushContextSuite) TestRebootAfterHook(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.context(c, ctrl)

	// Set reboot priority
	err := ctx.RequestReboot(stdcontext.Background(), jujuc.RebootAfterHook)
	c.Assert(err, jc.ErrorIsNil)

	// Flush the context with an error and check that reboot is not triggered.
	expErr := errors.New("hook execution failed")
	err = ctx.Flush(stdcontext.Background(), "some badge", expErr)
	c.Assert(err, gc.Equals, expErr)

	// Flush the context without an error and check that reboot is triggered.
	s.unit.EXPECT().SetAgentStatus(gomock.Any(), status.Rebooting, "", nil).Return(nil)
	s.unit.EXPECT().RequestReboot(gomock.Any()).Return(nil)
	err = ctx.Flush(stdcontext.Background(), "some badge", nil)
	c.Assert(err, gc.Equals, context.ErrReboot)
}

func (s *FlushContextSuite) TestRebootWhenHookFails(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.context(c, ctrl)

	var stub testing.Stub
	ctx.SetProcess(&mockProcess{func() error {
		priority := ctx.GetRebootPriority()
		c.Assert(priority, gc.Equals, jujuc.RebootAfterHook)
		return stub.NextErr()
	}})
	stub.SetErrors(errors.New("process is already dead"))

	// Set reboot priority
	err := ctx.RequestReboot(stdcontext.Background(), jujuc.RebootAfterHook)
	c.Assert(err, jc.ErrorIsNil)

	// Flush the context with an error and check that reboot is not triggered.
	expErr := errors.New("hook execution failed")
	err = ctx.Flush(stdcontext.Background(), "some badge", expErr)
	c.Assert(err, gc.ErrorMatches, "hook execution failed")
}

func (s *FlushContextSuite) TestRebootNowWhenHookFails(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.context(c, ctrl)

	var stub testing.Stub
	ctx.SetProcess(&mockProcess{func() error {
		priority := ctx.GetRebootPriority()
		c.Assert(priority, gc.Equals, jujuc.RebootNow)
		return stub.NextErr()
	}})
	stub.SetErrors(errors.New("process is already dead"))

	// Set reboot priority
	err := ctx.RequestReboot(stdcontext.Background(), jujuc.RebootNow)
	c.Assert(err, jc.ErrorIsNil)

	// Flush the context with an error and check that reboot is triggered regardless.
	s.unit.EXPECT().SetAgentStatus(gomock.Any(), status.Rebooting, "", nil).Return(nil)
	s.unit.EXPECT().RequestReboot(gomock.Any()).Return(nil)

	expErr := errors.New("hook execution failed")
	err = ctx.Flush(stdcontext.Background(), "some badge", expErr)
	c.Assert(err, gc.Equals, context.ErrRequeueAndReboot)
}

func (s *FlushContextSuite) TestRebootNow(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.context(c, ctrl)

	var stub testing.Stub
	ctx.SetProcess(&mockProcess{func() error {
		priority := ctx.GetRebootPriority()
		c.Assert(priority, gc.Equals, jujuc.RebootNow)
		return stub.NextErr()
	}})
	stub.SetErrors(errors.New("process is already dead"))

	// Set reboot priority
	err := ctx.RequestReboot(stdcontext.Background(), jujuc.RebootNow)
	c.Assert(err, jc.ErrorIsNil)

	// Flush the context without an error and check that reboot is triggered.
	s.unit.EXPECT().SetAgentStatus(gomock.Any(), status.Rebooting, "", nil).Return(nil)
	s.unit.EXPECT().RequestReboot(gomock.Any()).Return(nil)

	err = ctx.Flush(stdcontext.Background(), "some badge", nil)
	c.Assert(err, gc.Equals, context.ErrRequeueAndReboot)
}

func (s *FlushContextSuite) TestRunHookOpensAndClosesPendingPorts(c *gc.C) {
	// Open some ports on this unit and another one.
	s.machinePortRanges = map[names.UnitTag]network.GroupedPortRanges{
		s.unit.Tag(): {
			allEndpoints: []network.PortRange{network.MustParsePortRange("100-200/tcp")},
		},
		names.NewUnitTag("u/1"): {
			allEndpoints: []network.PortRange{network.MustParsePortRange("200-300/udp")},
		},
	}

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.context(c, ctrl)

	// Try opening some ports via the context.
	err := ctx.OpenPortRange(stdcontext.Background(), "", network.MustParsePortRange("100-200/tcp"))
	c.Assert(err, jc.ErrorIsNil) // duplicates are ignored
	err = ctx.OpenPortRange(stdcontext.Background(), "", network.MustParsePortRange("200-300/udp"))
	c.Assert(err, gc.ErrorMatches, `cannot open 200-300/udp \(unit "u/0"\): port range conflicts with 200-300/udp \(unit "u/1"\)`)
	err = ctx.OpenPortRange(stdcontext.Background(), "", network.MustParsePortRange("100-200/udp"))
	c.Assert(err, gc.ErrorMatches, `cannot open 100-200/udp \(unit "u/0"\): port range conflicts with 200-300/udp \(unit "u/1"\)`)
	err = ctx.OpenPortRange(stdcontext.Background(), "", network.MustParsePortRange("10-20/udp"))
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.OpenPortRange(stdcontext.Background(), "", network.MustParsePortRange("50-100/tcp"))
	c.Assert(err, gc.ErrorMatches, `cannot open 50-100/tcp \(unit "u/0"\): port range conflicts with 100-200/tcp \(unit "u/0"\)`)
	err = ctx.OpenPortRange(stdcontext.Background(), "", network.MustParsePortRange("50-80/tcp"))
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.OpenPortRange(stdcontext.Background(), "", network.MustParsePortRange("40-90/tcp"))
	c.Assert(err, gc.ErrorMatches, `cannot open 40-90/tcp \(unit "u/0"\): port range conflicts with 50-80/tcp \(unit "u/0"\) requested earlier`)

	// Now try closing some ports as well.
	err = ctx.ClosePortRange(stdcontext.Background(), "", network.MustParsePortRange("8080-8088/udp"))
	c.Assert(err, jc.ErrorIsNil) // not existing -> ignored
	err = ctx.ClosePortRange(stdcontext.Background(), "", network.MustParsePortRange("100-200/tcp"))
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.ClosePortRange(stdcontext.Background(), "", network.MustParsePortRange("100-200/tcp"))
	c.Assert(err, jc.ErrorIsNil) // duplicates are ignored
	err = ctx.ClosePortRange(stdcontext.Background(), "", network.MustParsePortRange("200-300/udp"))
	c.Assert(err, gc.ErrorMatches, `.*port range conflicts with 200-300/udp \(unit "u/1"\)`)
	err = ctx.ClosePortRange(stdcontext.Background(), "", network.MustParsePortRange("50-80/tcp"))
	c.Assert(err, jc.ErrorIsNil) // still pending -> no longer pending

	s.unit.EXPECT().CommitHookChanges(gomock.Any(), hookCommitMatcher{c: c, expected: params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{{
			Tag: s.unit.Tag().String(),
			OpenPorts: []params.EntityPortRange{{
				Tag:      s.unit.Tag().String(),
				Protocol: "udp",
				FromPort: 10,
				ToPort:   20,
				Endpoint: "",
			}},
			ClosePorts: []params.EntityPortRange{{
				Tag:      s.unit.Tag().String(),
				Protocol: "tcp",
				FromPort: 50,
				ToPort:   80,
				Endpoint: "",
			}, {
				Tag:      s.unit.Tag().String(),
				Protocol: "tcp",
				FromPort: 100,
				ToPort:   200,
				Endpoint: "",
			}, {
				Tag:      s.unit.Tag().String(),
				Protocol: "udp",
				FromPort: 8080,
				ToPort:   8088,
				Endpoint: "",
			}},
		}},
	}}).Return(nil)

	// Flush the context with a success.
	err = ctx.Flush(stdcontext.Background(), "some badge", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FlushContextSuite) TestRunHookUpdatesSecrets(c *gc.C) {
	uri := secrets.NewURI()
	uri2 := secrets.NewURI()

	s.secretMetadata = map[string]jujuc.SecretMetadata{
		uri.ID: {
			Description:    "some secret",
			LatestRevision: 1,
			LatestChecksum: "deadbeef",
			Owner:          secrets.Owner{Kind: secrets.ApplicationOwner, ID: "mariadb"},
		},
	}

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.context(c, ctrl)

	err := ctx.UpdateSecret(uri, &jujuc.SecretUpdateArgs{
		RotatePolicy: ptr(secrets.RotateDaily),
		Description:  ptr("a secret"),
		Label:        ptr("foobar"),
		Value:        secrets.NewSecretValue(map[string]string{"foo": "bar2"}),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.RemoveSecret(uri2, ptr(1))
	c.Assert(err, jc.ErrorIsNil)

	app, _ := names.UnitApplication(s.unit.Name())
	err = ctx.RevokeSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: ptr(app),
	})
	c.Assert(err, jc.ErrorIsNil)

	appTag := names.NewApplicationTag(app)
	arg := params.CommitHookChangesArg{
		Tag: s.unit.Tag().String(),
		SecretUpdates: []params.UpdateSecretArg{{
			URI: uri.String(),
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: ptr(secrets.RotateDaily),
				Description:  ptr("a secret"),
				Label:        ptr("foobar"),
				Content: params.SecretContentParams{
					Data:     map[string]string{"foo": "bar2"},
					Checksum: "f6956a0bbc93272e46689a2a3ccde66bbb8add5166df232f3b27644a589c656c",
				},
			},
		}},
		SecretRevokes: []params.GrantRevokeSecretArg{{
			URI:         uri.String(),
			ScopeTag:    appTag.String(),
			SubjectTags: []string{appTag.String()},
			Role:        "",
		}},
		SecretDeletes: []params.DeleteSecretArg{{
			URI:       uri2.String(),
			Revisions: []int{1},
		}},
	}

	s.unit.EXPECT().CommitHookChanges(gomock.Any(), hookCommitMatcher{c: c, expected: params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{arg},
	}}).Return(nil)

	// Flush the context with a success.
	err = ctx.Flush(stdcontext.Background(), "some badge", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BaseHookContextSuite) context(c *gc.C, ctrl *gomock.Controller) *context.HookContext {
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.AddContextRelation(c, ctrl, "db0")
	s.AddContextRelation(c, ctrl, "db1")

	return s.getHookContext(c, ctrl, uuid.String(), -1, "", names.StorageTag{})
}
