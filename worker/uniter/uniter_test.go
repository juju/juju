package uniter_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	stdtesting "testing"
	"time"
)

func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

type UniterSuite struct {
	testing.JujuConnSuite
	coretesting.HTTPSuite
	dataDir string
	oldPath string
}

var _ = Suite(&UniterSuite{})

func (s *UniterSuite) SetUpSuite(c *C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.HTTPSuite.SetUpSuite(c)
	s.dataDir = c.MkDir()
	toolsDir := environs.AgentToolsDir(s.dataDir, "unit-u-0")
	err := os.MkdirAll(toolsDir, 0755)
	c.Assert(err, IsNil)
	cmd := exec.Command("go", "build", "launchpad.net/juju-core/cmd/jujuc")
	cmd.Dir = toolsDir
	out, err := cmd.CombinedOutput()
	c.Logf(string(out))
	c.Assert(err, IsNil)
	s.oldPath = os.Getenv("PATH")
	os.Setenv("PATH", toolsDir+":"+s.oldPath)
}

func (s *UniterSuite) TearDownSuite(c *C) {
	os.Setenv("PATH", s.oldPath)
}

func (s *UniterSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.HTTPSuite.SetUpTest(c)
}

func (s *UniterSuite) TearDownTest(c *C) {
	s.HTTPSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *UniterSuite) Reset(c *C) {
	s.JujuConnSuite.Reset(c)
}

type uniterTest struct {
	summary string
	steps   []stepper
}

func ut(summary string, steps ...stepper) uniterTest {
	return uniterTest{summary, steps}
}

type stepper interface {
	step(c *C, ctx *context)
}

type context struct {
	id      int
	path    string
	dataDir string
	st      *state.State
	charms  coretesting.ResponseMap
	hooks   []string
	svc     *state.Service
	unit    *state.Unit
	uniter  *uniter.Uniter
}

var goodHook = `
#!/bin/bash
juju-log UniterSuite-%d %s
exit 0
`[1:]

var badHook = `
#!/bin/bash
juju-log UniterSuite-%d fail-%s
exit 1
`[1:]

func (ctx *context) writeHook(c *C, path string, good bool) {
	hook := badHook
	if good {
		hook = goodHook
	}
	content := fmt.Sprintf(hook, ctx.id, filepath.Base(path))
	err := ioutil.WriteFile(path, []byte(content), 0755)
	c.Assert(err, IsNil)
}

func (ctx *context) matchLogHooks(c *C) (bool, bool) {
	hookPattern := fmt.Sprintf(`^.* UniterSuite-%d ([a-z-]+)$`, ctx.id)
	hookRegexp := regexp.MustCompile(hookPattern)
	var actual []string
	for _, line := range strings.Split(c.GetTestLog(), "\n") {
		if parts := hookRegexp.FindStringSubmatch(line); parts != nil {
			actual = append(actual, parts[1])
		}
	}
	c.Logf("actual: %#v", actual)
	if len(actual) < len(ctx.hooks) {
		return false, false
	}
	for i, e := range ctx.hooks {
		if actual[i] != e {
			return false, false
		}
	}
	return true, len(actual) > len(ctx.hooks)
}

var uniterTests = []uniterTest{
	// Check conditions that can cause the uniter to fail to start.
	ut(
		"unable to create directories",
		writeFile{"state", 0644},
		startUniter{`failed to create uniter for unit "u/0": .*state must be a directory`},
	), ut(
		"unknown unit",
		startUniter{`failed to create uniter for unit "u/0": cannot get unit .*`},
	),
	// Check error conditions during unit bootstrap phase.
	ut(
		"charm cannot be downloaded",
		createCharm{},
		custom{func(c *C, ctx *context) {
			coretesting.Server.Response(404, nil, nil)
		}},
		createUniter{},
		waitUniterDead{`ModeInstalling: failed to download charm .* 404 Not Found`},
	), ut(
		"charm cannot be installed",
		writeFile{"charm", 0644},
		createCharm{},
		serveCharm{},
		createUniter{},
		waitUniterDead{`ModeInstalling: failed to write charm to .*`},
	), ut(
		"install hook fail and resolve",
		startupError{"install"},
		verifyWaiting{},

		resolveError{state.ResolvedNoHooks},
		waitUnit{status: state.UnitStarted},
		waitHooks{"start", "config-changed"},
	), ut(
		"install hook fail and retry",
		startupError{"install"},
		verifyWaiting{},

		resolveError{state.ResolvedRetryHooks},
		waitUnit{status: state.UnitError, info: `hook failed: "install"`},
		waitHooks{"fail-install"},
		fixHook{"install"},
		verifyWaiting{},

		resolveError{state.ResolvedRetryHooks},
		waitUnit{status: state.UnitStarted},
		waitHooks{"install", "start", "config-changed"},
	), ut(
		"start hook fail and resolve",
		startupError{"start"},
		verifyWaiting{},

		resolveError{state.ResolvedNoHooks},
		waitUnit{status: state.UnitStarted},
		waitHooks{"config-changed"},
		verifyRunning{},
	), ut(
		"start hook fail and retry",
		startupError{"start"},
		verifyWaiting{},

		resolveError{state.ResolvedRetryHooks},
		waitUnit{status: state.UnitError, info: `hook failed: "start"`},
		waitHooks{"fail-start"},
		verifyWaiting{},

		fixHook{"start"},
		resolveError{state.ResolvedRetryHooks},
		waitUnit{status: state.UnitStarted},
		waitHooks{"start", "config-changed"},
		verifyRunning{},
	), ut(
		"config-changed hook fail and resolve",
		startupError{"config-changed"},
		verifyWaiting{},

		// Note: we'll run another config-changed as soon as we hit the
		// started state, so the broken hook would actually prevent us
		// from advancing at all if we didn't fix it.
		fixHook{"config-changed"},
		resolveError{state.ResolvedNoHooks},
		waitUnit{status: state.UnitStarted},
		waitHooks{"config-changed"},
		// If we'd accidentally retried that hook, somehow, we would get
		// an extra config-changed as we entered started; see that we don't.
		waitHooks{},
		verifyRunning{},
	), ut(
		"config-changed hook fail and retry",
		startupError{"config-changed"},
		verifyWaiting{},

		resolveError{state.ResolvedRetryHooks},
		waitUnit{status: state.UnitError, info: `hook failed: "config-changed"`},
		waitHooks{"fail-config-changed"},
		verifyWaiting{},

		fixHook{"config-changed"},
		resolveError{state.ResolvedRetryHooks},
		waitUnit{status: state.UnitStarted},
		// Note: the second config-changed hook is automatically run as we
		// enter started. IMO the simplicity and clarity of that approach
		// outweigh this slight inelegance.
		waitHooks{"config-changed", "config-changed"},
		verifyRunning{},
	), ut(
		"steady state config change",
		quickStart{},
		changeConfig{},
		waitUnit{status: state.UnitStarted},
		waitHooks{"config-changed"},
		verifyRunning{},
	),
}

func (s *UniterSuite) TestUniter(c *C) {
	unitDir := filepath.Join(s.dataDir, "units", "u-0")
	for i, t := range uniterTests {
		if i != 0 {
			s.Reset(c)
			coretesting.Server.Flush()
			err := os.RemoveAll(unitDir)
			c.Assert(err, IsNil)
		}
		c.Logf("\ntest %d: %s\n", i, t.summary)
		ctx := &context{
			id:      i,
			path:    unitDir,
			dataDir: s.dataDir,
			st:      s.State,
			charms:  coretesting.ResponseMap{},
		}
		for i, s := range t.steps {
			c.Logf("step %d", i)
			step(c, ctx, s)
		}
		if ctx.uniter != nil {
			err := ctx.uniter.Stop()
			c.Assert(err, IsNil)
		}
	}
}

func step(c *C, ctx *context, s stepper) {
	c.Logf("%#v", s)
	s.step(c, ctx)
}

type createCharm struct {
	revision int
	badHooks []string
}

func (s createCharm) step(c *C, ctx *context) {
	base := coretesting.Charms.ClonedDirPath(c.MkDir(), "dummy", "series")
	for _, name := range []string{"install", "start", "config-changed", "upgrade-charm"} {
		path := filepath.Join(base, "hooks", name)
		good := true
		for _, bad := range s.badHooks {
			if name == bad {
				good = false
			}
		}
		ctx.writeHook(c, path, good)
	}
	dir, err := charm.ReadDir(base)
	c.Assert(err, IsNil)
	err = dir.SetDiskRevision(s.revision)
	c.Assert(err, IsNil)
	buf := &bytes.Buffer{}
	err = dir.BundleTo(buf)
	c.Assert(err, IsNil)
	body := buf.Bytes()
	hasher := sha256.New()
	_, err = io.Copy(hasher, buf)
	c.Assert(err, IsNil)
	hash := hex.EncodeToString(hasher.Sum(nil))
	key := fmt.Sprintf("/charms/%d", s.revision)
	hurl, err := url.Parse(coretesting.Server.URL + key)
	c.Assert(err, IsNil)
	ctx.charms[key] = coretesting.Response{200, nil, body}
	_, err = ctx.st.AddCharm(dir, curl(s.revision), hurl, hash)
	c.Assert(err, IsNil)
}

type serveCharm struct{}

func (s serveCharm) step(c *C, ctx *context) {
	coretesting.Server.ResponseMap(1, ctx.charms)
}

type createUniter struct{}

func (s createUniter) step(c *C, ctx *context) {
	cfg, err := config.New(map[string]interface{}{
		"name":            "testenv",
		"type":            "dummy",
		"default-series":  "abominable",
		"authorized-keys": "we-are-the-keys",
	})
	c.Assert(err, IsNil)
	err = ctx.st.SetEnvironConfig(cfg)
	c.Assert(err, IsNil)
	sch, err := ctx.st.Charm(curl(0))
	c.Assert(err, IsNil)
	svc, err := ctx.st.AddService("u", sch)
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)
	ctx.svc = svc
	ctx.unit = unit
	step(c, ctx, startUniter{})

	// Poll for correct address settings (consequence of "dummy" env type).
	timeout := time.After(1 * time.Second)
	for {
		select {
		case <-timeout:
			c.Fatalf("timed out waiting for unit addresses")
		case <-time.After(50 * time.Millisecond):
			private, err := unit.PrivateAddress()
			if err != nil || private != "private.dummy.address.example.com" {
				continue
			}
			public, err := unit.PublicAddress()
			if err != nil || public != "public.dummy.address.example.com" {
				continue
			}
			return
		}
	}
}

type startUniter struct {
	err string
}

func (s startUniter) step(c *C, ctx *context) {
	if ctx.uniter != nil {
		panic("don't start two uniters!")
	}
	u, err := uniter.NewUniter(ctx.st, "u/0", ctx.dataDir)
	if s.err == "" {
		c.Assert(err, IsNil)
		ctx.uniter = u
	} else {
		c.Assert(u, IsNil)
		c.Assert(err, ErrorMatches, s.err)
	}
}

type waitUniterDead struct {
	err string
}

func (s waitUniterDead) step(c *C, ctx *context) {
	u := ctx.uniter
	ctx.uniter = nil
	select {
	case <-u.Dying():
		err := u.Wait()
		c.Assert(err, ErrorMatches, s.err)
	case <-time.After(1000 * time.Millisecond):
		c.Fatalf("uniter still alive")
	}
}

type stopUniter struct {
	err string
}

func (s stopUniter) step(c *C, ctx *context) {
	u := ctx.uniter
	ctx.uniter = nil
	err := u.Stop()
	if s.err == "" {
		c.Assert(err, IsNil)
	} else {
		c.Assert(err, ErrorMatches, s.err)
	}
}

type verifyWaiting struct{}

func (s verifyWaiting) step(c *C, ctx *context) {
	step(c, ctx, stopUniter{})
	step(c, ctx, startUniter{})
	step(c, ctx, waitHooks{})
}

type verifyRunning struct{}

func (s verifyRunning) step(c *C, ctx *context) {
	step(c, ctx, stopUniter{})
	step(c, ctx, startUniter{})
	step(c, ctx, waitHooks{"config-changed"})
}

type startupError struct {
	badHook string
}

func (s startupError) step(c *C, ctx *context) {
	step(c, ctx, createCharm{badHooks: []string{s.badHook}})
	step(c, ctx, serveCharm{})
	step(c, ctx, createUniter{})
	step(c, ctx, waitUnit{
		status: state.UnitError,
		info:   fmt.Sprintf(`hook failed: %q`, s.badHook),
	})
	for _, hook := range []string{"install", "start", "config-changed"} {
		if hook == s.badHook {
			step(c, ctx, waitHooks{"fail-" + hook})
			break
		}
		step(c, ctx, waitHooks{hook})
	}
	step(c, ctx, verifyCharm{})
}

type quickStart struct{}

func (s quickStart) step(c *C, ctx *context) {
	step(c, ctx, createCharm{})
	step(c, ctx, serveCharm{})
	step(c, ctx, createUniter{})
	step(c, ctx, waitUnit{status: state.UnitStarted})
	step(c, ctx, waitHooks{"install", "start", "config-changed"})
	step(c, ctx, verifyCharm{})
}

type resolveError struct {
	resolved state.ResolvedMode
}

func (s resolveError) step(c *C, ctx *context) {
	err := ctx.unit.SetResolved(s.resolved)
	c.Assert(err, IsNil)
}

type waitUnit struct {
	status   state.UnitStatus
	info     string
	charm    int
	resolved state.ResolvedMode
}

func (s waitUnit) step(c *C, ctx *context) {
	timeout := time.After(3000 * time.Millisecond)
	// Upgrade/resolved checks are easy...
	resolved := ctx.unit.WatchResolved()
	defer stop(c, resolved)
	resolvedOk := false
	for !resolvedOk {
		select {
		case ch := <-resolved.Changes():
			resolvedOk = ch == s.resolved
			if !resolvedOk {
				c.Logf("%#v", ch)
			}
		case <-timeout:
			c.Fatalf("never reached desired state")
		}
	}

	// ...but we have no status/charm watchers, so just poll.
	for {
		select {
		case <-time.After(200 * time.Millisecond):
			status, info, err := ctx.unit.Status()
			c.Assert(err, IsNil)
			if status != s.status {
				c.Logf("wrong status: %s", status)
				continue
			}
			if info != s.info {
				c.Logf("wrong info: %s", info)
				continue
			}
			ch, err := ctx.unit.Charm()
			if err != nil {
				c.Logf("no charm")
				continue
			}
			if *ch.URL() != *curl(s.charm) {
				c.Logf("wrong charm: %s", ch.URL())
				continue
			}
			return
		case <-timeout:
			c.Fatalf("never reached desired status")
		}
	}
}

type waitHooks []string

func (s waitHooks) step(c *C, ctx *context) {
	if len(s) == 0 {
		// Give unwanted hooks a moment to run...
		time.Sleep(200 * time.Millisecond)
	}
	ctx.hooks = append(ctx.hooks, s...)
	c.Logf("waiting for hooks: %#v", ctx.hooks)
	match, overshoot := ctx.matchLogHooks(c)
	if overshoot && len(s) == 0 {
		c.Fatalf("ran more hooks than expected")
	}
	if match {
		return
	}
	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-time.After(200 * time.Millisecond):
			if match, _ = ctx.matchLogHooks(c); match {
				return
			}
		case <-timeout:
			c.Fatalf("never got expected hooks")
		}
	}
}

type fixHook struct {
	name string
}

func (s fixHook) step(c *C, ctx *context) {
	path := filepath.Join(ctx.path, "charm", "hooks", s.name)
	ctx.writeHook(c, path, true)
}

type changeConfig struct{}

func (s changeConfig) step(c *C, ctx *context) {
	node, err := ctx.svc.Config()
	c.Assert(err, IsNil)
	prev, found := node.Get("skill-level")
	if !found {
		prev = 0
	}
	node.Set("skill-level", prev.(int)+1)
	_, err = node.Write()
	c.Assert(err, IsNil)
}

type upgradeCharm struct {
	revision int
	forced   bool
}

func (s upgradeCharm) step(c *C, ctx *context) {
	sch, err := ctx.st.Charm(curl(s.revision))
	c.Assert(err, IsNil)
	err = ctx.svc.SetCharm(sch, s.forced)
	c.Assert(err, IsNil)
}

type verifyCharm struct {
	revision int
}

func (s verifyCharm) step(c *C, ctx *context) {
	path := filepath.Join(ctx.path, "charm", "revision")
	content, err := ioutil.ReadFile(path)
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, strconv.Itoa(s.revision))
	ch, err := ctx.unit.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.URL(), DeepEquals, curl(s.revision))
}

type writeFile struct {
	path string
	mode os.FileMode
}

func (s writeFile) step(c *C, ctx *context) {
	path := filepath.Join(ctx.path, s.path)
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(path, nil, s.mode)
	c.Assert(err, IsNil)
}

type chmod struct {
	path string
	mode os.FileMode
}

func (s chmod) step(c *C, ctx *context) {
	path := filepath.Join(ctx.path, s.path)
	err := os.Chmod(path, s.mode)
	c.Assert(err, IsNil)
}

type custom struct {
	f func(*C, *context)
}

func (s custom) step(c *C, ctx *context) {
	s.f(c, ctx)
}

func curl(revision int) *charm.URL {
	return charm.MustParseURL("cs:series/dummy").WithRevision(revision)
}
