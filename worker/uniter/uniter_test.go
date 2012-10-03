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
	"launchpad.net/juju-core/worker"
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
	coretesting.MgoTestPackage(t)
}

type UniterSuite struct {
	testing.JujuConnSuite
	coretesting.HTTPSuite
	dataDir  string
	oldLcAll string
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
	s.oldLcAll = os.Getenv("LC_ALL")
	os.Setenv("LC_ALL", "en_US")
}

func (s *UniterSuite) TearDownSuite(c *C) {
	os.Setenv("LC_ALL", s.oldLcAll)
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

func (ctx *context) run(c *C, steps []stepper) {
	defer func() {
		if ctx.uniter != nil {
			err := ctx.uniter.Stop()
			c.Assert(err, IsNil)
		}
	}()
	for i, s := range steps {
		c.Logf("step %d", i)
		step(c, ctx, s)
	}
}

var goodHook = `
#!/bin/bash
juju-log UniterSuite-%d %s
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
	hookPattern := fmt.Sprintf(`^.* JUJU u/0: UniterSuite-%d ([a-z-]+)$`, ctx.id)
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
		"unable to create state dir",
		writeFile{"state", 0644},
		createCharm{},
		createServiceAndUnit{},
		startUniter{},
		waitUniterDead{`failed to initialize uniter for unit "u/0": .*state must be a directory`},
	), ut(
		"unknown unit",
		startUniter{},
		waitUniterDead{`failed to initialize uniter for unit "u/0": unit "u/0" not found`},
	),

	// Check error conditions during unit bootstrap phase.
	ut(
		"insane deployment",
		createCharm{},
		serveCharm{},
		writeFile{"charm", 0644},
		createUniter{},
		waitUniterDead{`ModeInstalling cs:series/dummy-0: charm deployment failed: ".*charm" is not a directory`},
	), ut(
		"charm cannot be downloaded",
		createCharm{},
		custom{func(c *C, ctx *context) {
			coretesting.Server.Response(404, nil, nil)
		}},
		createUniter{},
		waitUniterDead{`ModeInstalling cs:series/dummy-0: failed to download charm .* 404 Not Found`},
	), ut(
		"install hook fail and resolve",
		startupError{"install"},
		verifyWaiting{},

		resolveError{state.ResolvedNoHooks},
		waitUnit{
			status: state.UnitStarted,
		},
		waitHooks{"start", "config-changed"},
	), ut(
		"install hook fail and retry",
		startupError{"install"},
		verifyWaiting{},

		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: state.UnitError,
			info:   `hook failed: "install"`,
		},
		waitHooks{"fail-install"},
		fixHook{"install"},
		verifyWaiting{},

		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: state.UnitStarted,
		},
		waitHooks{"install", "start", "config-changed"},
	), ut(
		"start hook fail and resolve",
		startupError{"start"},
		verifyWaiting{},

		resolveError{state.ResolvedNoHooks},
		waitUnit{
			status: state.UnitStarted,
		},
		waitHooks{"config-changed"},
		verifyRunning{},
	), ut(
		"start hook fail and retry",
		startupError{"start"},
		verifyWaiting{},

		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: state.UnitError,
			info:   `hook failed: "start"`,
		},
		waitHooks{"fail-start"},
		verifyWaiting{},

		fixHook{"start"},
		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: state.UnitStarted,
		},
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
		waitUnit{
			status: state.UnitStarted,
		},
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
		waitUnit{
			status: state.UnitError,
			info:   `hook failed: "config-changed"`,
		},
		waitHooks{"fail-config-changed"},
		verifyWaiting{},

		fixHook{"config-changed"},
		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: state.UnitStarted,
		},
		// Note: the second config-changed hook is automatically run as we
		// re-enter ModeAbide. IMO the simplicity and clarity of that approach
		// outweigh this slight inelegance.
		waitHooks{"config-changed", "config-changed"},
		verifyRunning{},
	),

	// Steady state changes.
	ut(
		"steady state config change",
		quickStart{},
		changeConfig{},
		waitUnit{
			status: state.UnitStarted,
		},
		waitHooks{"config-changed"},
		verifyRunning{},
	),

	// Reaction to entity deaths.
	ut(
		"steady state service dying",
		quickStart{},
		serviceDying,
		waitHooks{"stop"},
		waitUniterDead{},
	), ut(
		"steady state unit dying",
		quickStart{},
		unitDying,
		waitHooks{"stop"},
		waitUniterDead{},
	), ut(
		"steady state unit dead",
		quickStart{},
		unitDead,
		waitHooks{},
		waitUniterDead{},
	), ut(
		"hook error service dying",
		startupError{"start"},
		serviceDying,
		verifyWaiting{},
		fixHook{"start"},
		resolveError{state.ResolvedRetryHooks},
		waitHooks{"start", "config-changed", "stop"},
		waitUniterDead{},
	), ut(
		"hook error unit dying",
		startupError{"start"},
		unitDying,
		verifyWaiting{},
		fixHook{"start"},
		resolveError{state.ResolvedRetryHooks},
		waitHooks{"start", "config-changed", "stop"},
		waitUniterDead{},
	), ut(
		"hook error unit dead",
		startupError{"start"},
		unitDead,
		waitHooks{},
		waitUniterDead{},
	),

	// Upgrade scenarios.
	ut(
		"steady state upgrade",
		quickStart{},
		createCharm{revision: 1},
		upgradeCharm{revision: 1},
		waitUnit{
			status: state.UnitStarted,
		},
		waitHooks{"upgrade-charm", "config-changed"},
		verifyCharm{revision: 1},
		verifyRunning{},
	), ut(
		"steady state forced upgrade (identical behaviour)",
		quickStart{},
		createCharm{revision: 1},
		upgradeCharm{revision: 1, forced: true},
		waitUnit{
			status: state.UnitStarted,
			charm:  1,
		},
		waitHooks{"upgrade-charm", "config-changed"},
		verifyCharm{revision: 1},
		verifyRunning{},
	), ut(
		"steady state upgrade hook fail and resolve",
		quickStart{},
		createCharm{revision: 1, badHooks: []string{"upgrade-charm"}},
		upgradeCharm{revision: 1},
		waitUnit{
			status: state.UnitError,
			info:   `hook failed: "upgrade-charm"`,
			charm:  1,
		},
		waitHooks{"fail-upgrade-charm"},
		verifyCharm{revision: 1},
		verifyWaiting{},

		resolveError{state.ResolvedNoHooks},
		waitUnit{
			status: state.UnitStarted,
			charm:  1,
		},
		waitHooks{"config-changed"},
		verifyRunning{},
	), ut(
		"steady state upgrade hook fail and retry",
		quickStart{},
		createCharm{revision: 1, badHooks: []string{"upgrade-charm"}},
		upgradeCharm{revision: 1},
		waitUnit{
			status: state.UnitError,
			info:   `hook failed: "upgrade-charm"`,
			charm:  1,
		},
		waitHooks{"fail-upgrade-charm"},
		verifyCharm{revision: 1},
		verifyWaiting{},

		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: state.UnitError,
			info:   `hook failed: "upgrade-charm"`,
			charm:  1,
		},
		waitHooks{"fail-upgrade-charm"},
		verifyWaiting{},

		fixHook{"upgrade-charm"},
		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: state.UnitStarted,
			charm:  1,
		},
		waitHooks{"upgrade-charm", "config-changed"},
		verifyRunning{},
	), ut(
		"error state unforced upgrade (ignored until started state)",
		startupError{"start"},
		createCharm{revision: 1},
		upgradeCharm{revision: 1},
		waitUnit{
			status: state.UnitError,
			info:   `hook failed: "start"`,
		},
		waitHooks{},
		verifyCharm{},
		verifyWaiting{},

		resolveError{state.ResolvedNoHooks},
		waitUnit{
			status: state.UnitStarted,
			charm:  1,
		},
		waitHooks{"config-changed", "upgrade-charm", "config-changed"},
		verifyCharm{revision: 1},
		verifyRunning{},
	), ut(
		"error state forced upgrade",
		startupError{"start"},
		createCharm{revision: 1},
		upgradeCharm{revision: 1, forced: true},
		waitUnit{
			status: state.UnitError,
			info:   `hook failed: "start"`,
			charm:  1,
		},
		waitHooks{},
		verifyCharm{revision: 1},
		verifyWaiting{},

		resolveError{state.ResolvedNoHooks},
		waitUnit{
			status: state.UnitStarted,
			charm:  1,
		},
		waitHooks{"config-changed"},
		verifyRunning{},
	), ut(
		"upgrade: conflicting files",
		startUpgradeError{},

		// NOTE: this is just dumbly committing the conflicts, but AFAICT this
		// is the only reasonable solution; if the user tells us it's resolved
		// we have to take their word for it.
		resolveError{state.ResolvedNoHooks},
		waitHooks{"upgrade-charm", "config-changed"},
		waitUnit{
			status: state.UnitStarted,
			charm:  1,
		},
		verifyCharm{revision: 1},
	), ut(
		`upgrade: conflicting directories`,
		createCharm{
			customize: func(c *C, path string) {
				err := os.Mkdir(filepath.Join(path, "data"), 0755)
				c.Assert(err, IsNil)
				start := filepath.Join(path, "hooks", "start")
				f, err := os.OpenFile(start, os.O_WRONLY|os.O_APPEND, 0755)
				c.Assert(err, IsNil)
				defer f.Close()
				_, err = f.Write([]byte("echo DATA > data/newfile"))
				c.Assert(err, IsNil)
			},
		},
		serveCharm{},
		createUniter{},
		waitUnit{
			status: state.UnitStarted,
		},
		waitHooks{"install", "start", "config-changed"},
		verifyCharm{},

		createCharm{
			revision: 1,
			customize: func(c *C, path string) {
				data := filepath.Join(path, "data")
				err := ioutil.WriteFile(data, []byte("<nelson>ha ha</nelson>"), 0644)
				c.Assert(err, IsNil)
			},
		},
		serveCharm{},
		upgradeCharm{revision: 1},
		waitUnit{
			status: state.UnitError,
			info:   "upgrade failed",
		},
		verifyWaiting{},
		verifyCharm{dirty: true},

		resolveError{state.ResolvedNoHooks},
		waitHooks{"upgrade-charm", "config-changed"},
		waitUnit{
			status: state.UnitStarted,
			charm:  1,
		},
		verifyCharm{revision: 1},
	), ut(
		"upgrade conflict resolved with forced upgrade",
		startUpgradeError{},
		createCharm{
			revision: 2,
			customize: func(c *C, path string) {
				otherdata := filepath.Join(path, "otherdata")
				err := ioutil.WriteFile(otherdata, []byte("blah"), 0644)
				c.Assert(err, IsNil)
			},
		},
		serveCharm{},
		upgradeCharm{revision: 2, forced: true},
		waitUnit{
			status: state.UnitStarted,
			charm:  2,
		},
		verifyCharm{revision: 2},
		custom{func(c *C, ctx *context) {
			// otherdata should exist (in v2)
			otherdata, err := ioutil.ReadFile(filepath.Join(ctx.path, "charm", "otherdata"))
			c.Assert(err, IsNil)
			c.Assert(string(otherdata), Equals, "blah")

			// ignore should not (only in v1)
			_, err = os.Stat(filepath.Join(ctx.path, "charm", "ignore"))
			c.Assert(os.IsNotExist(err), Equals, true)

			// data should contain what was written in the start hook
			data, err := ioutil.ReadFile(filepath.Join(ctx.path, "charm", "data"))
			c.Assert(err, IsNil)
			c.Assert(string(data), Equals, "STARTDATA\n")
		}},
	), ut(
		"upgrade conflict service dying",
		startUpgradeError{},
		serviceDying,
		verifyWaiting{},
		resolveError{state.ResolvedNoHooks},
		waitHooks{"upgrade-charm", "config-changed", "stop"},
		waitUniterDead{},
	), ut(
		"upgrade conflict unit dying",
		startUpgradeError{},
		unitDying,
		verifyWaiting{},
		resolveError{state.ResolvedNoHooks},
		waitHooks{"upgrade-charm", "config-changed", "stop"},
		waitUniterDead{},
	), ut(
		"upgrade conflict unit dead",
		startUpgradeError{},
		unitDead,
		waitHooks{},
		waitUniterDead{},
	),
}

func (s *UniterSuite) TestUniter(c *C) {
	unitDir := filepath.Join(s.dataDir, "agents", "unit-u-0")
	for i, t := range uniterTests {
		if i != 0 {
			s.Reset(c)
			coretesting.Server.Flush()
			err := os.RemoveAll(unitDir)
			c.Assert(err, IsNil)
		}
		c.Logf("\ntest %d: %s\n", i, t.summary)
		ctx := &context{
			st:      s.State,
			id:      i,
			path:    unitDir,
			dataDir: s.dataDir,
			charms:  coretesting.ResponseMap{},
		}
		ctx.run(c, t.steps)
	}
}

func step(c *C, ctx *context, s stepper) {
	c.Logf("%#v", s)
	s.step(c, ctx)
}

type createCharm struct {
	revision  int
	badHooks  []string
	customize func(*C, string)
}

var charmHooks = []string{"install", "start", "config-changed", "upgrade-charm", "stop"}

func (s createCharm) step(c *C, ctx *context) {
	base := coretesting.Charms.ClonedDirPath(c.MkDir(), "series", "dummy")
	for _, name := range charmHooks {
		path := filepath.Join(base, "hooks", name)
		good := true
		for _, bad := range s.badHooks {
			if name == bad {
				good = false
			}
		}
		ctx.writeHook(c, path, good)
	}
	if s.customize != nil {
		s.customize(c, base)
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

func (serveCharm) step(c *C, ctx *context) {
	coretesting.Server.ResponseMap(1, ctx.charms)
}

type createServiceAndUnit struct{}

func (createServiceAndUnit) step(c *C, ctx *context) {
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
}

type createUniter struct{}

func (createUniter) step(c *C, ctx *context) {
	step(c, ctx, createServiceAndUnit{})
	step(c, ctx, startUniter{})
	timeout := time.After(1 * time.Second)
	for {
		select {
		case <-timeout:
			c.Fatalf("timed out waiting for unit addresses")
		case <-time.After(50 * time.Millisecond):
			err := ctx.unit.Refresh()
			if err != nil {
				c.Fatalf("unit refresh failed: %v", err)
			}
			private, err := ctx.unit.PrivateAddress()
			if err != nil || private != "private.dummy.address.example.com" {
				continue
			}
			public, err := ctx.unit.PublicAddress()
			if err != nil || public != "public.dummy.address.example.com" {
				continue
			}
			return
		}
	}
}

type startUniter struct{}

func (s startUniter) step(c *C, ctx *context) {
	if ctx.uniter != nil {
		panic("don't start two uniters!")
	}
	ctx.uniter = uniter.NewUniter(ctx.st, "u/0", ctx.dataDir)
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
		if s.err == "" {
			c.Assert(err, Equals, worker.ErrDead)
			err = ctx.unit.Refresh()
			c.Assert(err, IsNil)
			c.Assert(ctx.unit.Life(), Equals, state.Dead)
		} else {
			c.Assert(err, ErrorMatches, s.err)
		}
	case <-time.After(5 * time.Second):
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
	timeout := time.After(5 * time.Second)
	for {
		ctx.st.StartSync()
		select {
		case <-time.After(50 * time.Millisecond):
			err := ctx.unit.Refresh()
			if err != nil {
				c.Fatalf("cannot refresh unit: %v", err)
			}
			resolved := ctx.unit.Resolved()
			if resolved != s.resolved {
				c.Logf("want resolved mode %q, got %q; still waiting", s.resolved, resolved)
				continue
			}
			ch, err := ctx.unit.Charm()
			if err != nil || *ch.URL() != *curl(s.charm) {
				var got string
				if ch != nil {
					got = ch.URL().String()
				}
				c.Logf("want unit charm %q, got %q; still waiting", curl(s.charm), got)
				continue
			}
			status, info, err := ctx.unit.Status()
			c.Assert(err, IsNil)
			if status != s.status {
				c.Logf("want unit status %q, got %q; still waiting", s.status, status)
				continue
			}
			if info != s.info {
				c.Logf("want unit status info %q, got %q; still waiting", s.info, info)
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
		ctx.st.StartSync()
		time.Sleep(100 * time.Millisecond)
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
	timeout := time.After(5 * time.Second)
	for {
		ctx.st.StartSync()
		select {
		case <-time.After(50 * time.Millisecond):
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
	serveCharm{}.step(c, ctx)
}

type verifyCharm struct {
	revision int
	dirty    bool
}

func (s verifyCharm) step(c *C, ctx *context) {
	if !s.dirty {
		path := filepath.Join(ctx.path, "charm", "revision")
		content, err := ioutil.ReadFile(path)
		c.Assert(err, IsNil)
		c.Assert(string(content), Equals, strconv.Itoa(s.revision))
		err = ctx.unit.Refresh()
		c.Assert(err, IsNil)
		ch, err := ctx.unit.Charm()
		c.Assert(err, IsNil)
		c.Assert(ch.URL(), DeepEquals, curl(s.revision))
	}

	// Even if the charm itself has been updated correctly, it is possible that
	// a hook has run and is being committed by git; which will cause all manner
	// of bad stuff to happen when we try to get the status below. There's no
	// general way to guarantee that this is not happening, but the following
	// voodoo sleep has been observed to be sufficient in practice.
	time.Sleep(500 * time.Millisecond)

	cmd := exec.Command("git", "status")
	cmd.Dir = filepath.Join(ctx.path, "charm")
	out, err := cmd.CombinedOutput()
	c.Assert(err, IsNil)

	cmp := Equals
	if s.dirty {
		cmp = Not(Equals)
	}
	c.Assert(string(out), cmp, "# On branch master\nnothing to commit (working directory clean)\n")
}

type startUpgradeError struct{}

func (s startUpgradeError) step(c *C, ctx *context) {
	steps := []stepper{
		createCharm{
			customize: func(c *C, path string) {
				start := filepath.Join(path, "hooks", "start")
				f, err := os.OpenFile(start, os.O_WRONLY|os.O_APPEND, 0755)
				c.Assert(err, IsNil)
				defer f.Close()
				_, err = f.Write([]byte("echo STARTDATA > data"))
				c.Assert(err, IsNil)
			},
		},
		serveCharm{},
		createUniter{},
		waitUnit{
			status: state.UnitStarted,
		},
		waitHooks{"install", "start", "config-changed"},
		verifyCharm{},

		createCharm{
			revision: 1,
			customize: func(c *C, path string) {
				data := filepath.Join(path, "data")
				err := ioutil.WriteFile(data, []byte("<nelson>ha ha</nelson>"), 0644)
				c.Assert(err, IsNil)
				ignore := filepath.Join(path, "ignore")
				err = ioutil.WriteFile(ignore, []byte("anything"), 0644)
				c.Assert(err, IsNil)
			},
		},
		serveCharm{},
		upgradeCharm{revision: 1},
		waitUnit{
			status: state.UnitError,
			info:   "upgrade failed",
		},
		verifyWaiting{},
		verifyCharm{dirty: true},
	}
	for _, s_ := range steps {
		step(c, ctx, s_)
	}
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

var serviceDying = custom{func(c *C, ctx *context) {
	c.Assert(ctx.svc.EnsureDying(), IsNil)
}}

var unitDying = custom{func(c *C, ctx *context) {
	c.Assert(ctx.unit.EnsureDying(), IsNil)
}}

var unitDead = custom{func(c *C, ctx *context) {
	c.Assert(ctx.unit.EnsureDead(), IsNil)
}}

func curl(revision int) *charm.URL {
	return charm.MustParseURL("cs:series/dummy").WithRevision(revision)
}
