// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
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

// goEnv captures the process environment before the test suites isolate
// HOME and the Juju variables. The go tool needs HOME, GOPATH or
// GOMODCACHE to resolve the module cache.
var goEnv = os.Environ()

// repositoryRoot returns the juju repository root that contains this
// test's package.
func repositoryRoot(c *tc.C) string {
	_, file, _, ok := runtime.Caller(0)
	c.Assert(ok, tc.IsTrue)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
}

// skippedDirName reports whether a directory entry must be skipped by
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

// bannedDependencies lists package paths that the jujuagentd binary's
// transitive dependency graph must not contain. Dqlite, the controller
// API server, and the controller worker graph belong to the jujud
// controller binary. The source scans above cannot see indirect
// dependencies, so TestJujuagentdDependencyBoundary asks the Go tool for
// the real graph.
var bannedDependencies = []struct {
	path  string
	exact bool
}{
	// Dqlite and its Juju wrappers.
	{path: "github.com/canonical/go-dqlite"},
	{path: "github.com/juju/juju/internal/dqlite"},
	// The controller worker graph.
	{path: "github.com/juju/juju/internal/worker/apiserver"},
	{path: "github.com/juju/juju/internal/worker/dbaccessor"},
	{path: "github.com/juju/juju/internal/worker/dqliteconfigupdater"},
	// The controller command package.
	{path: "github.com/juju/juju/cmd/jujud"},
	// The root apiserver package is the controller's API server. Its
	// errors and apiserverhttp subpackages are shared client helpers and
	// are legitimately imported by the machine agent, so ban only the
	// root package.
	{path: "github.com/juju/juju/apiserver", exact: true},
}

// TestStandaloneControllerSourceBoundary scans the repository's
// non-test Go source and fails when a banned symbol reappears, the
// combined-agent constructor escapes its allowlist, or Dqlite
// configuration members come back in machine-agent config writers.
func (*BoundaryGuardSuite) TestStandaloneControllerSourceBoundary(c *tc.C) {
	root := repositoryRoot(c)
	// Guard the walk itself: a wrong root would otherwise make every
	// check below pass without scanning the repository.
	_, err := os.Stat(filepath.Join(root, "go.mod"))
	c.Assert(err, tc.ErrorIsNil, tc.Commentf(
		"repository root %s does not contain go.mod", root))

	scanned := 0
	forEachGoSource(c, root, func(rel, content string) {
		scanned++
		if violations := sourceBoundaryViolations(rel, content); len(violations) > 0 {
			c.Check(violations, tc.HasLen, 0, tc.Commentf(
				"boundary violations in %s:\n%s", rel, strings.Join(violations, "\n")))
		}
	})
	c.Check(scanned > 0, tc.IsTrue, tc.Commentf(
		"no Go source scanned under %s", root))
}

// sourceBoundaryViolations returns the standalone-controller boundary
// violations in a single non-test Go file, or nil when the file respects
// the boundary.
//
// The rules live in one pure function so they can be table-tested: a
// regression that skips a rule (for example by returning early for an
// allowlisted tree) fails TestSourceBoundaryPredicate instead of
// silently disabling the guard.
func sourceBoundaryViolations(rel, content string) []string {
	var violations []string
	report := func(format string, args ...any) {
		violations = append(violations, fmt.Sprintf(format, args...))
	}

	for _, banned := range bannedEverywhere {
		if strings.Contains(content, banned) {
			report("%s references deleted symbol %s", rel, banned)
		}
	}
	if !containsAny(rel, newStateMachineConfigAllowlist) &&
		strings.Contains(content, "NewStateMachineConfig") {
		report("%s references NewStateMachineConfig outside %v",
			rel, newStateMachineConfigAllowlist)
	}
	if containsAny(rel, machineAgentConfigWriterTrees) {
		for _, member := range removedDqliteMembers {
			if strings.Contains(content, member) {
				report("%s references removed Dqlite configuration member %s",
					rel, member)
			}
		}
	}
	if strings.HasPrefix(rel, "agent/") {
		for _, key := range removedAgentConfDqliteKeys {
			if strings.Contains(content, key) {
				report("%s references removed agent.conf key %s", rel, key)
			}
		}
	}
	return violations
}

// TestSourceBoundaryPredicate is the positive control for the source
// guard. It proves each rule fires for a file it targets and stays quiet
// for the trees that are deliberately exempt, so a rule cannot become
// unreachable without failing this test.
func (*BoundaryGuardSuite) TestSourceBoundaryPredicate(c *tc.C) {
	tests := []struct {
		name    string
		rel     string
		content string
		want    []string
	}{{
		name:    "banned symbol outside any allowlist",
		rel:     "internal/foo/bar.go",
		content: "func SetStateServingInfo() {}",
		want: []string{
			"internal/foo/bar.go references deleted symbol SetStateServingInfo",
		},
	}, {
		name:    "dqlite member in the agent config writer",
		rel:     "agent/config.go",
		content: "DqlitePort int",
		want: []string{
			"agent/config.go references removed Dqlite configuration member DqlitePort",
		},
	}, {
		name:    "agent.conf key in agent",
		rel:     "agent/format-2.0.go",
		content: `yaml:"dqlitebusytimeout,omitempty"`,
		want: []string{
			"agent/format-2.0.go references removed agent.conf key dqlitebusytimeout",
		},
	}, {
		name:    "combined-agent constructor outside the allowlist",
		rel:     "cmd/jujuagentd/agent/model.go",
		content: "agent.NewStateMachineConfig(",
		want: []string{
			"cmd/jujuagentd/agent/model.go references NewStateMachineConfig outside [agent/ cmd/jujud/]",
		},
	}, {
		name:    "combined-agent constructor inside the agent allowlist",
		rel:     "agent/agent.go",
		content: "agent.NewStateMachineConfig(",
	}, {
		name:    "combined-agent constructor inside the jujud allowlist",
		rel:     "cmd/jujud/agent/bootstrap.go",
		content: "agent.NewStateMachineConfig(",
	}, {
		name:    "dqlite member in a controller-owned runtime writer",
		rel:     "internal/controllerruntimeconfig/config.go",
		content: "DqlitePort int",
	}}

	for _, test := range tests {
		got := sourceBoundaryViolations(test.rel, test.content)
		c.Check(got, tc.DeepEquals, test.want, tc.Commentf("case %q", test.name))
	}
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

// TestJujuagentdDependencyBoundary proves the jujuagentd binary's
// transitive dependency graph carries no Dqlite or controller package.
// The source scans above cannot see indirect dependencies; this test
// asks the Go tool for the real graph.
func (*BoundaryGuardSuite) TestJujuagentdDependencyBoundary(c *tc.C) {
	deps := goListDeps(c, repositoryRoot(c), "./cmd/jujuagentd")
	c.Assert(len(deps) > 0, tc.IsTrue, tc.Commentf(
		"go list returned no dependencies for ./cmd/jujuagentd"))

	var violations []string
	for _, dep := range deps {
		if banned := bannedDependency(dep); banned != "" {
			violations = append(violations, fmt.Sprintf(
				"jujuagentd depends on %s, which matches banned package %s", dep, banned))
		}
	}
	c.Check(violations, tc.HasLen, 0, tc.Commentf("%s", strings.Join(violations, "\n")))
}

// goListDeps returns the transitive package dependencies of pkg, as
// reported by the Go tool from the repository root.
func goListDeps(c *tc.C, root, pkg string) []string {
	cmd := exec.Command("go", "list", "-deps", pkg)
	cmd.Dir = root
	cmd.Env = goEnv
	out, err := cmd.CombinedOutput()
	c.Assert(err, tc.ErrorIsNil, tc.Commentf(
		"go list -deps %s failed: %v\n%s", pkg, err, out))
	return strings.Fields(string(out))
}

// bannedDependency returns the banned path that dep matches, or "" when
// dep is allowed.
func bannedDependency(dep string) string {
	for _, banned := range bannedDependencies {
		if dep == banned.path {
			return banned.path
		}
		if !banned.exact && strings.HasPrefix(dep, banned.path+"/") {
			return banned.path
		}
	}
	return ""
}

func containsAny(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
