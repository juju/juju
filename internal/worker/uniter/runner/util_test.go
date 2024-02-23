// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	stdcontext "context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/juju/charm"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4/fs"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	uniterapi "github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/runner"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/context/mocks"
	runnertesting "github.com/juju/juju/internal/worker/uniter/runner/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

var (
	hookName      = "something-happened"
	echoPidScript = "echo $$ > pid"
)

type ContextSuite struct {
	jujutesting.IsolationSuite

	paths          runnertesting.RealPaths
	factory        runner.Factory
	contextFactory context.ContextFactory
	membership     map[int][]string

	uniter   *uniterapi.MockUniterClient
	unit     *uniterapi.MockUnit
	payloads *mocks.MockPayloadAPIClient
	secrets  *runnertesting.SecretsContextAccessor

	relunits map[int]*uniterapi.MockRelationUnit
}

func (s *ContextSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.relunits = map[int]*uniterapi.MockRelationUnit{}
	s.secrets = &runnertesting.SecretsContextAccessor{}
}

func (s *ContextSuite) AddContextRelation(c *gc.C, ctrl *gomock.Controller, name string) {
	num := len(s.relunits)
	rel := uniterapi.NewMockRelation(ctrl)
	rel.EXPECT().Id().Return(num).AnyTimes()
	rel.EXPECT().Tag().Return(names.NewRelationTag("mysql:server wordpress:" + name)).AnyTimes()

	relUnit := uniterapi.NewMockRelationUnit(ctrl)
	relUnit.EXPECT().Relation().Return(rel).AnyTimes()
	relUnit.EXPECT().Endpoint().Return(apiuniter.Endpoint{Relation: charm.Relation{Name: "db"}}).AnyTimes()
	relUnit.EXPECT().Settings().Return(
		apiuniter.NewSettings(rel.Tag().String(), names.NewUnitTag("u/0").String(), params.Settings{}), nil,
	).AnyTimes()

	s.relunits[num] = relUnit
}

func (s *ContextSuite) setupUnit(ctrl *gomock.Controller) names.MachineTag {
	unitTag := names.NewUnitTag("u/0")
	s.unit = uniterapi.NewMockUnit(ctrl)
	s.unit.EXPECT().Tag().Return(unitTag).AnyTimes()
	s.unit.EXPECT().Name().Return(unitTag.Id()).AnyTimes()
	s.unit.EXPECT().PublicAddress().Return("u-0.testing.invalid", nil).AnyTimes()
	s.unit.EXPECT().PrivateAddress().Return("u-0.testing.invalid", nil).AnyTimes()
	s.unit.EXPECT().AvailabilityZone().Return("a-zone", nil).AnyTimes()

	machineTag := names.NewMachineTag("0")
	s.unit.EXPECT().AssignedMachine().Return(machineTag, nil).AnyTimes()
	return machineTag
}

func (s *ContextSuite) setupUniter(ctrl *gomock.Controller) names.MachineTag {
	machineTag := s.setupUnit(ctrl)
	s.uniter = uniterapi.NewMockUniterClient(ctrl)
	s.uniter.EXPECT().OpenedMachinePortRangesByEndpoint(gomock.Any(), machineTag).DoAndReturn(func(_ stdcontext.Context, _ names.MachineTag) (map[names.UnitTag]network.GroupedPortRanges, error) {
		return nil, nil
	}).AnyTimes()
	s.uniter.EXPECT().OpenedPortRangesByEndpoint(gomock.Any()).Return(nil, nil).AnyTimes()
	return machineTag
}

func (s *ContextSuite) setupFactory(c *gc.C, ctrl *gomock.Controller) {
	s.setupUniter(ctrl)

	s.unit.EXPECT().PrincipalName().Return("", false, nil).AnyTimes()
	s.uniter.EXPECT().Model(stdcontext.Background()).Return(&model.Model{
		Name:      "test-model",
		UUID:      coretesting.ModelTag.Id(),
		ModelType: model.IAAS,
	}, nil).AnyTimes()
	s.uniter.EXPECT().LeadershipSettings().Return(&stubLeadershipSettingsAccessor{}).AnyTimes()
	s.uniter.EXPECT().APIAddresses().Return([]string{"10.6.6.6"}, nil).AnyTimes()
	s.uniter.EXPECT().CloudAPIVersion(gomock.Any()).Return("6.6.6", nil).AnyTimes()

	cfg := coretesting.ModelConfig(c)
	s.uniter.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil).AnyTimes()

	s.payloads = mocks.NewMockPayloadAPIClient(ctrl)
	s.payloads.EXPECT().List().Return(nil, nil).AnyTimes()

	contextFactory, err := context.NewContextFactory(stdcontext.Background(), context.FactoryConfig{
		Uniter:           s.uniter,
		Unit:             s.unit,
		Tracker:          &runnertesting.FakeTracker{},
		GetRelationInfos: s.getRelationInfos,
		SecretsClient:    s.secrets,
		Payloads:         s.payloads,
		Paths:            s.paths,
		Clock:            testclock.NewClock(time.Time{}),
		Logger:           loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.contextFactory = contextFactory

	s.paths = runnertesting.NewRealPaths(c)
	s.membership = map[int][]string{}

	s.contextFactory, err = context.NewContextFactory(stdcontext.Background(), context.FactoryConfig{
		Uniter:           s.uniter,
		Unit:             s.unit,
		Payloads:         s.payloads,
		Tracker:          &runnertesting.FakeTracker{},
		GetRelationInfos: s.getRelationInfos,
		SecretsClient:    s.secrets,
		Paths:            s.paths,
		Clock:            testclock.NewClock(time.Time{}),
		Logger:           loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)

	factory, err := runner.NewFactory(
		s.paths,
		s.contextFactory,
		runner.NewRunner,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.factory = factory

	s.AddContextRelation(c, ctrl, "db0")
	s.AddContextRelation(c, ctrl, "db1")
}

func (s *ContextSuite) setCharm(c *gc.C, name string) {
	err := os.RemoveAll(s.paths.GetCharmDir())
	c.Assert(err, jc.ErrorIsNil)
	err = fs.Copy(testcharms.Repo.CharmDirPath(name), s.paths.GetCharmDir())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ContextSuite) getRelationInfos() map[int]*context.RelationInfo {
	info := map[int]*context.RelationInfo{}
	for relId, relUnit := range s.relunits {
		info[relId] = &context.RelationInfo{
			RelationUnit: relUnit,
			MemberNames:  s.membership[relId],
		}
	}
	return info
}

// hookSpec supports makeCharm.
type hookSpec struct {
	// dir is the directory to create the hook in.
	dir string
	// name is the name of the hook.
	name string
	// perm is the file permissions of the hook.
	perm os.FileMode
	// code is the exit status of the hook.
	code int
	// stdout holds a string to print to stdout
	stdout string
	// stderr holds a string to print to stderr
	stderr string
	// background holds a string to print in the background after 0.2s.
	background string
	// missingShebang will omit the '#!/bin/bash' line
	missingShebang bool
	// charmMissing will remove the charm before running the hook
	charmMissing bool
}

// makeCharm constructs a fake charm dir containing a single named hook
// with permissions perm and exit code code. If output is non-empty,
// the charm will write it to stdout and stderr, with each one prefixed
// by name of the stream.
func makeCharm(c *gc.C, spec hookSpec, charmDir string) {
	dir := charmDir
	if spec.dir != "" {
		dir = filepath.Join(dir, spec.dir)
		err := os.Mkdir(dir, 0755)
		c.Assert(err, jc.ErrorIsNil)
	}
	if !spec.charmMissing {
		makeCharmMetadata(c, charmDir)
	}
	c.Logf("openfile perm %v", spec.perm)
	hook, err := os.OpenFile(
		filepath.Join(dir, spec.name), os.O_CREATE|os.O_WRONLY, spec.perm,
	)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Assert(hook.Close(), gc.IsNil)
	}()

	printf := func(f string, a ...interface{}) {
		_, err := fmt.Fprintf(hook, f+"\n", a...)
		c.Assert(err, jc.ErrorIsNil)
	}
	if !spec.missingShebang {
		printf("#!/bin/bash")
	}
	printf(echoPidScript)
	if spec.stdout != "" {
		printf("echo %s", spec.stdout)
	}
	if spec.stderr != "" {
		printf("echo %s >&2", spec.stderr)
	}
	if spec.background != "" {
		// Print something fairly quickly, then sleep for
		// quite a long time - if the hook execution is
		// blocking because of the background process,
		// the hook execution will take much longer than
		// expected.
		printf("(sleep 0.2; echo %s; sleep 10) &", spec.background)
	}
	printf("exit %d", spec.code)
}

func makeCharmMetadata(c *gc.C, charmDir string) {
	err := os.MkdirAll(charmDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(path.Join(charmDir, "metadata.yaml"), nil, 0664)
	c.Assert(err, jc.ErrorIsNil)
}

type stubLeadershipSettingsAccessor struct {
	results map[string]string
}

func (s *stubLeadershipSettingsAccessor) Read(_ string) (result map[string]string, _ error) {
	return result, nil
}

func (s *stubLeadershipSettingsAccessor) Merge(_, _ string, settings map[string]string) error {
	if s.results == nil {
		s.results = make(map[string]string)
	}
	for k, v := range settings {
		s.results[k] = v
	}
	return nil
}
