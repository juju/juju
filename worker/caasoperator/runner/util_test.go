// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/juju/testing"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/caasoperator/runner"
	"github.com/juju/juju/worker/caasoperator/runner/context"
	"github.com/juju/juju/worker/caasoperator/runner/runnertesting"
)

var (
	apiAddrs      = []string{"a1:123", "a2:123"}
	hookName      = "something-happened"
	echoPidScript = "echo $$ > pid"
)

type ContextSuite struct {
	testing.BaseSuite

	paths          runnertesting.MockPaths
	factory        runner.Factory
	contextFactory context.ContextFactory
	membership     map[int][]string

	relationUnitAPIs map[int]*runnertesting.MockRelationUnitAPI
	relIdCounter     int
}

func (s *ContextSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("non windows functionality")
	}
	s.BaseSuite.SetUpSuite(c)
}

func (s *ContextSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.paths = runnertesting.NewMockPaths(c)
	s.membership = map[int][]string{}
	s.relIdCounter = 0
	s.relationUnitAPIs = make(map[int]*runnertesting.MockRelationUnitAPI)
	s.AddContextRelation(c, "db0")
	s.AddContextRelation(c, "db1")

	mockAPI := runnertesting.NewMockContextAPI(apiAddrs, proxy.Settings{})
	var err error
	s.contextFactory, err = context.NewContextFactory(context.FactoryConfig{
		ContextFactoryAPI: mockAPI,
		HookAPI:           mockAPI,
		ModelUUID:         testing.ModelTag.Id(),
		ModelName:         "kate",
		GetRelationInfos:  s.getRelationInfos,
		Paths:             s.paths,
		Clock:             jujutesting.NewClock(time.Time{}),
	})
	c.Assert(err, jc.ErrorIsNil)

	factory, err := runner.NewFactory(
		s.paths,
		s.contextFactory,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.factory = factory
}

func (s *ContextSuite) AddContextRelation(c *gc.C, name string) {
	s.relationUnitAPIs[s.relIdCounter] = runnertesting.NewMockRelationUnitAPI(s.relIdCounter, "relation-name", false)
	s.relIdCounter++
}

func (s *ContextSuite) getRelationInfos() map[int]*context.RelationInfo {
	info := map[int]*context.RelationInfo{}
	for relId, relAPI := range s.relationUnitAPIs {
		info[relId] = &context.RelationInfo{
			RelationUnitAPI: relAPI,
			MemberNames:     s.membership[relId],
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
	printf("#!/bin/bash")
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
