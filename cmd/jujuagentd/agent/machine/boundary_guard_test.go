// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

// BoundaryGuardSuite scans the repository's Go source for references
// that would reintroduce controller behaviour into the machine agent
// binary. The scans run as part of the ordinary unit-test run, so CI
// fails when jujuagentd regains a controller worker, a controller
// command, a legacy controller path, or a Dqlite build dependency.
type BoundaryGuardSuite struct {
	testing.BaseSuite
}

func TestBoundaryGuardSuite(t *stdtesting.T) {
	tc.Run(t, &BoundaryGuardSuite{})
}

// repositoryRoot returns the juju repository root that contains this
// test's package.
func repositoryRoot(c *tc.C) string {
	_, file, _, ok := runtime.Caller(0)
	c.Assert(ok, tc.IsTrue)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
}

// skippedDirNames reports whether a directory entry must be skipped by
// the source scans: hidden and build-artifact trees (".git", ".direnv",
// "_build", "_deps"), stale test-run leftovers ("tmp.*"), and charm
// fixtures. The scans only cover first-party Go source.
func skippedDirName(name string) bool {
	return strings.HasPrefix(name, ".") ||
		strings.HasPrefix(name, "_") ||
		strings.HasPrefix(name, "tmp.") ||
		name == "testcharms"
}

// forEachGoSource walks the repository's first-party non-test Go
// source and calls visit with the repository-relative slash-separated
// path and the file content.
func forEachGoSource(c *tc.C, root string, visit func(rel, content string)) {
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root || !skippedDirName(d.Name()) {
				return nil
			}
			return fs.SkipDir
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		visit(filepath.ToSlash(rel), string(content))
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

// bannedEverywhere lists symbols that no longer exist anywhere in the
// codebase and must not be reintroduced: the dead agentconfigupdater
// package, the obsolete machine-agent certificate rewrite, the removed
// serving-info config setter, and the removed runtime.conf path
// fallback helpers. The read-only Agent v3 StateServingInfo facade
// surface (the apiserver facade method, the API client, and the params
// DTO) is still published protocol surface and is deliberately not
// in this list.
var bannedEverywhere = []string{
	"agentconfigupdater",
	"upgradeCertificateDNSNames",
	"SetStateServingInfo",
	"EffectiveSocketDir",
	"EffectiveSharedAgentDir",
}

// newStateMachineConfigAllowlist holds the package trees that may
// reference NewStateMachineConfig: its definition in agent and the
// jujud bootstrap in-memory use.
var newStateMachineConfigAllowlist = []string{
	"agent/",
	"cmd/jujud/",
}

// machineAgentConfigWriterTrees holds the package trees whose
// configuration files are machine-agent or shared agent.conf writers.
// They must not carry the removed Dqlite-only configuration members.
// Controller-owned runtime.conf writers (internal/cloudconfig/
// userdatacfg.go, internal/controllerruntimeconfig, the jujud Dqlite
// config updater) are outside these trees and stay free to use them.
var machineAgentConfigWriterTrees = []string{
	"agent/",
	"cmd/jujuagentd/",
	"cmd/containeragent/initialize/",
	"internal/cloudconfig/instancecfg/",
	"internal/cloudconfig/podcfg/",
	"apiserver/facades/agent/caasapplication/",
}

// removedDqliteMembers are the Dqlite-only configuration members that
// agent.conf and agent.Config no longer carry.
var removedDqliteMembers = []string{
	"DqlitePort",
	"QueryTracingEnabled",
	"QueryTracingThreshold",
	"DqliteBusyTimeout",
}

// removedAgentConfDqliteKeys are the Dqlite keys that agent.conf no
// longer serializes. They are banned in agent, which owns the
// serialization.
var removedAgentConfDqliteKeys = []string{
	"dqlite-port",
	"querytracingenabled",
	"querytracingthreshold",
	"dqlitebusytimeout",
}

// TestStandaloneControllerSourceBoundary scans the repository's
// non-test Go source and fails when a banned symbol reappears, the
// combined-agent constructor escapes its allowlist, or Dqlite
// configuration members come back in machine-agent config writers.
func (*BoundaryGuardSuite) TestStandaloneControllerSourceBoundary(c *tc.C) {
	root := repositoryRoot(c)
	forEachGoSource(c, root, func(rel, content string) {
		for _, banned := range bannedEverywhere {
			c.Check(strings.Contains(content, banned), tc.IsFalse, tc.Commentf(
				"%s references deleted symbol %s", rel, banned))
		}
		if containsAny(rel, newStateMachineConfigAllowlist) {
			return
		}
		if strings.Contains(content, "NewStateMachineConfig") {
			c.Errorf("%s references NewStateMachineConfig outside agent/ and cmd/jujud/", rel)
		}
		if containsAny(rel, machineAgentConfigWriterTrees) {
			for _, member := range removedDqliteMembers {
				c.Check(strings.Contains(content, member), tc.IsFalse, tc.Commentf(
					"%s references removed Dqlite configuration member %s", rel, member))
			}
		}
		if strings.HasPrefix(rel, "agent/") {
			for _, key := range removedAgentConfDqliteKeys {
				c.Check(strings.Contains(content, key), tc.IsFalse, tc.Commentf(
					"%s references removed agent.conf key %s", rel, key))
			}
		}
	})
}

// controllerCommandConstructors are the command constructors registered
// by the jujud controller binary's command surface. None of them may be
// registered by jujuagentd, whose command surface covers machine and
// model agents only. NewCheckConnectionCommand is shared by both
// binaries and is not listed.
var controllerCommandConstructors = []string{
	"NewBootstrapCommand",
	"NewInitCommand",
	"NewGetControllerConfigCommand",
	"NewSetControllerConfigCommand",
	"NewControllerApplicationCommand",
	"NewDBReplAgentCommand",
	"NewSafeModeApplicationCommand",
}

// TestJujuagentdCommandSurface proves no controller command is
// registered in the jujuagentd command surface.
func (*BoundaryGuardSuite) TestJujuagentdCommandSurface(c *tc.C) {
	root := repositoryRoot(c)
	jujuagentdRoot := filepath.Join(root, "cmd", "jujuagentd")
	found := false
	forEachGoSource(c, jujuagentdRoot, func(rel, content string) {
		found = true
		for _, ctor := range controllerCommandConstructors {
			c.Check(strings.Contains(content, ctor), tc.IsFalse, tc.Commentf(
				"cmd/jujuagentd/%s registers controller command constructor %s", rel, ctor))
		}
	})
	c.Check(found, tc.IsTrue, tc.Commentf("no jujuagentd source found to scan"))
}

func containsAny(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
