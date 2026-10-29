// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/juju/tc"
)

// runSuite pins the exit-code contract of run(). Each test runs inside
// a temporary fake repository with a stub sqlfluff shell script on PATH,
// so no real repository configuration or sqlfluff installation is
// required. cwd, PATH and environment variables are swapped in and
// restored via cleanup; the suite deliberately does not embed
// internal/testing.BaseSuite because it needs explicit control over
// exactly those resources, and the script is a standalone tool with
// no dependency on Juju state.
type runSuite struct {
	repo string
	bin  string
}

func TestRunSuite(t *testing.T) {
	tc.Run(t, &runSuite{})
}

func (s *runSuite) SetUpTest(c *tc.C) {
	s.bin = c.MkDir()
	s.writeStub(c)
	s.repo = s.makeRepo(c, true)
	s.chdir(c, s.repo)
	s.setenv(c, "PATH", s.bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	s.setenv(c, "GITHUB_ACTIONS", "")
	s.setenv(c, "STUB_CLEAN", "")
}

// fixtureGo is a Go source file with one raw-string SQL statement. The
// SQL line "SELECT id  FROM t" is literal content: a violation pointing
// at it in the rendered .sql file maps back to the same line and column
// in this Go source.
const (
	fixtureHeader = "package p\n\nfunc f() {\n\tst.Prepare(`"
	fixtureSQL    = "\nSELECT id  FROM t\nWHERE x = 1\n"
	fixtureGo     = fixtureHeader + fixtureSQL + "`)\n}\n"
)

// fixtureViolationPos returns the line and column of "FROM" in the SQL
// line of fixtureGo, i.e. the position a diagnostic mapped from the
// rendered SQL must report.
func fixtureViolationPos() (line, col int) {
	for i, l := range strings.Split(fixtureGo, "\n") {
		if strings.HasPrefix(l, "SELECT") {
			return i + 1, strings.Index(l, "FROM") + 1
		}
	}
	panic("fixture SQL line not found")
}

func (s *runSuite) makeRepo(c *tc.C, withConfig bool) string {
	dir := c.MkDir()
	c.Assert(os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fakerepo\n\ngo 1.24\n"), 0o644), tc.ErrorIsNil)
	if withConfig {
		c.Assert(os.Mkdir(filepath.Join(dir, ".github"), 0o755), tc.ErrorIsNil)
		c.Assert(os.WriteFile(filepath.Join(dir, ".github", ".sqlfluff"), []byte("[sqlfluff]\n"), 0o644), tc.ErrorIsNil)
	}
	return dir
}

// writeStub installs a portable /bin/sh sqlfluff stub in s.bin. It
// inspects its last argument (the extraction directory), finds a .sql
// file, and emits a violation at a position that maps to literal SQL
// content. With STUB_CLEAN=1 it reports no violations instead.
func (s *runSuite) writeStub(c *tc.C) {
	script := `#!/bin/sh
# Stub sqlfluff for run() contract tests.
dir=""
for arg in "$@"; do
	dir="$arg"
done
if [ "$STUB_CLEAN" = "1" ]; then
	printf '[]'
	exit 0
fi
sql=$(find "$dir" -type f -name '*.sql' | head -n 1)
if [ -z "$sql" ]; then
	printf '[]'
	exit 1
fi
line=$(grep -n '^SELECT' "$sql" | head -n 1 | cut -d: -f1)
if [ -z "$line" ]; then
	line=1
fi
col=$(awk -v n="$line" 'NR == n { print index($0, "FROM") }' "$sql")
if [ -z "$col" ] || [ "$col" = "0" ]; then
	col=1
fi
printf '[{"filepath":"%s","violations":[{"line_no":%s,"line_pos":%s,"code":"LT01","description":"stub spacing violation"}]}]' "$sql" "$line" "$col"
exit 1
`
	path := filepath.Join(s.bin, "sqlfluff")
	c.Assert(os.WriteFile(path, []byte(script), 0o755), tc.ErrorIsNil)
}

func (s *runSuite) chdir(c *tc.C, dir string) {
	old, err := os.Getwd()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(os.Chdir(dir), tc.ErrorIsNil)
	c.Cleanup(func() {
		_ = os.Chdir(old)
	})
}

func (s *runSuite) setenv(c *tc.C, name, value string) {
	old, had := os.LookupEnv(name)
	if had {
		c.Cleanup(func() { _ = os.Setenv(name, old) })
	} else {
		c.Cleanup(func() { _ = os.Unsetenv(name) })
	}
	_ = os.Setenv(name, value)
}

// captureStdout swaps os.Stdout for a pipe around fn and returns fn's
// result together with everything written to stdout. Output is small,
// so the pipe cannot fill before it is drained.
func captureStdout(c *tc.C, fn func() int) (int, string) {
	old := os.Stdout
	r, w, err := os.Pipe()
	c.Assert(err, tc.ErrorIsNil)
	os.Stdout = w
	defer func() { os.Stdout = old }()
	code := fn()
	c.Assert(w.Close(), tc.ErrorIsNil)
	data, err := io.ReadAll(r)
	c.Assert(err, tc.ErrorIsNil)
	return code, string(data)
}

func (s *runSuite) TestHelpExitsZero(c *tc.C) {
	for _, arg := range []string{"-h", "--help"} {
		code, _ := captureStdout(c, func() int { return run([]string{arg}) })
		c.Check(code, tc.Equals, 0, tc.Commentf("arg %q", arg))
	}
}

func (s *runSuite) TestBadFlagExitsTwo(c *tc.C) {
	code := run([]string{"--no-such-flag"})
	c.Check(code, tc.Equals, 2)
}

func (s *runSuite) TestMissingConfigExitsTwo(c *tc.C) {
	// The stub is on PATH, but the repository has no .github/.sqlfluff
	// and no config is passed, so run must abort before linting.
	s.chdir(c, s.makeRepo(c, false))
	code := run(nil)
	c.Check(code, tc.Equals, 2)
}

func (s *runSuite) TestMissingSQLFluffExitsTwo(c *tc.C) {
	// An empty PATH guarantees no sqlfluff, real or stubbed, is found.
	s.setenv(c, "PATH", c.MkDir())
	code := run(nil)
	c.Check(code, tc.Equals, 2)
}

func (s *runSuite) TestCleanRepoExitsZero(c *tc.C) {
	c.Assert(os.WriteFile(filepath.Join(s.repo, "fixture.go"), []byte(fixtureGo), 0o644), tc.ErrorIsNil)
	s.setenv(c, "STUB_CLEAN", "1")
	code := run(nil)
	c.Check(code, tc.Equals, 0)
}

func (s *runSuite) TestViolationExitsOne(c *tc.C) {
	c.Assert(os.WriteFile(filepath.Join(s.repo, "fixture.go"), []byte(fixtureGo), 0o644), tc.ErrorIsNil)
	code, stdout := captureStdout(c, func() int { return run(nil) })
	c.Check(code, tc.Equals, 1)
	line, col := fixtureViolationPos()
	c.Check(strings.Contains(stdout, "fixture.go:"+strconv.Itoa(line)+":"+strconv.Itoa(col)+": LT01 stub spacing violation"),
		tc.IsTrue, tc.Commentf("stdout: %s", stdout))
}

func (s *runSuite) TestReportOnlyViolationExitsZero(c *tc.C) {
	c.Assert(os.WriteFile(filepath.Join(s.repo, "fixture.go"), []byte(fixtureGo), 0o644), tc.ErrorIsNil)
	code := run([]string{"--report-only"})
	c.Check(code, tc.Equals, 0)
}

func (s *runSuite) TestMalformedGoSourceExitsTwo(c *tc.C) {
	// The linter is strict: a Go file that does not parse aborts the run
	// rather than being skipped. This pins that behaviour deliberately.
	repo := s.makeRepo(c, true)
	s.chdir(c, repo)
	c.Assert(os.WriteFile(filepath.Join(repo, "broken.go"), []byte("package p\n\nfunc {\n"), 0o644), tc.ErrorIsNil)
	code := run(nil)
	c.Check(code, tc.Equals, 2)
}

func (s *runSuite) TestGitHubActionsAutoAnnotation(c *tc.C) {
	c.Assert(os.WriteFile(filepath.Join(s.repo, "fixture.go"), []byte(fixtureGo), 0o644), tc.ErrorIsNil)
	s.setenv(c, "GITHUB_ACTIONS", "true")
	line, col := fixtureViolationPos()
	code, stdout := captureStdout(c, func() int { return run(nil) })
	c.Check(code, tc.Equals, 1)
	want := "::error file=fixture.go,line=" + strconv.Itoa(line) + ",col=" + strconv.Itoa(col) + "::LT01 stub spacing violation"
	c.Check(strings.Contains(stdout, want), tc.IsTrue, tc.Commentf("stdout: %s", stdout))
}
