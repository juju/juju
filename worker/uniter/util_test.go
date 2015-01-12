// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/rpc"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	gt "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	"github.com/juju/utils"
	utilexec "github.com/juju/utils/exec"
	"github.com/juju/utils/fslock"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v4"
	goyaml "gopkg.in/yaml.v1"

	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/charm"
)

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = coretesting.LongWait

// Assign the unit to a provisioned machine with dummy addresses set.
func assertAssignUnit(c *gc.C, st *state.State, u *state.Unit) {
	err := u.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := st.Machine(mid)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("i-exist", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetAddresses(network.Address{
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
		Value: "private.address.example.com",
	}, network.Address{
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
		Value: "public.address.example.com",
	})
	c.Assert(err, jc.ErrorIsNil)
}

type context struct {
	uuid          string
	path          string
	dataDir       string
	s             *UniterSuite
	st            *state.State
	api           *apiuniter.State
	charms        map[string][]byte
	hooks         []string
	sch           *state.Charm
	svc           *state.Service
	unit          *state.Unit
	uniter        *uniter.Uniter
	relatedSvc    *state.Service
	relation      *state.Relation
	relationUnits map[string]*state.RelationUnit
	subordinate   *state.Unit
	ticker        *uniter.ManualTicker

	mu             sync.Mutex
	hooksCompleted []string
}

var _ uniter.UniterExecutionObserver = (*context)(nil)

func (ctx *context) HookCompleted(hookName string) {
	ctx.mu.Lock()
	ctx.hooksCompleted = append(ctx.hooksCompleted, hookName)
	ctx.mu.Unlock()
}

func (ctx *context) HookFailed(hookName string) {
	ctx.mu.Lock()
	ctx.hooksCompleted = append(ctx.hooksCompleted, "fail-"+hookName)
	ctx.mu.Unlock()
}

func (ctx *context) run(c *gc.C, steps []stepper) {
	defer func() {
		if ctx.uniter != nil {
			err := ctx.uniter.Stop()
			c.Assert(err, jc.ErrorIsNil)
		}
	}()
	for i, s := range steps {
		c.Logf("step %d:\n", i)
		step(c, ctx, s)
	}
}

func (ctx *context) apiLogin(c *gc.C) {
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	st := ctx.s.OpenAPIAs(c, ctx.unit.Tag(), password)
	c.Assert(st, gc.NotNil)
	c.Logf("API: login as %q successful", ctx.unit.Tag())
	ctx.api, err = st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.api, gc.NotNil)
}

var goodHook = `
#!/bin/bash --norc
juju-log $JUJU_ENV_UUID %s $JUJU_REMOTE_UNIT
`[1:]

var badHook = `
#!/bin/bash --norc
juju-log $JUJU_ENV_UUID fail-%s $JUJU_REMOTE_UNIT
exit 1
`[1:]

var rebootHook = `
#!/bin/bash --norc
juju-reboot
`[1:]

var badRebootHook = `
#!/bin/bash --norc
juju-reboot
exit 1
`[1:]

var rebootNowHook = `
#!/bin/bash --norc

if [ -f "i_have_risen" ]
then
    exit 0
fi
touch i_have_risen
juju-reboot --now
`[1:]

func (ctx *context) writeExplicitHook(c *gc.C, path string, contents string) {
	err := ioutil.WriteFile(path, []byte(contents), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (ctx *context) writeHook(c *gc.C, path string, good bool) {
	hook := badHook
	if good {
		hook = goodHook
	}
	content := fmt.Sprintf(hook, filepath.Base(path))
	ctx.writeExplicitHook(c, path, content)
}

func (ctx *context) writeActions(c *gc.C, path string, names []string) {
	for _, name := range names {
		ctx.writeAction(c, path, name)
	}
}

func (ctx *context) writeMetricsYaml(c *gc.C, path string) {
	metricsYamlPath := filepath.Join(path, "metrics.yaml")
	var metricsYamlFull []byte = []byte(`
metrics:
  pings:
    type: gauge
    description: sample metric
`)
	err := ioutil.WriteFile(metricsYamlPath, []byte(metricsYamlFull), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (ctx *context) writeAction(c *gc.C, path, name string) {
	var actions = map[string]string{
		"action-log": `
#!/bin/bash --norc
juju-log $JUJU_ENV_UUID action-log
`[1:],
		"snapshot": `
#!/bin/bash --norc
action-set outfile.name="snapshot-01.tar" outfile.size="10.3GB"
action-set outfile.size.magnitude="10.3" outfile.size.units="GB"
action-set completion.status="yes" completion.time="5m"
action-set completion="yes"
`[1:],
		"action-log-fail": `
#!/bin/bash --norc
action-fail "I'm afraid I can't let you do that, Dave."
action-set foo="still works"
`[1:],
		"action-log-fail-error": `
#!/bin/bash --norc
action-fail too many arguments
action-set foo="still works"
action-fail "A real message"
`[1:],
		"action-reboot": `
#!/bin/bash --norc
juju-reboot || action-set reboot-delayed="good"
juju-reboot --now || action-set reboot-now="good"
`[1:],
	}

	actionPath := filepath.Join(path, "actions", name)
	action := actions[name]
	err := ioutil.WriteFile(actionPath, []byte(action), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (ctx *context) writeActionsYaml(c *gc.C, path string, names ...string) {
	var actionsYaml = map[string]string{
		"base": "",
		"snapshot": `
snapshot:
   description: Take a snapshot of the database.
   params:
      outfile:
         description: "The file to write out to."
         type: string
   required: ["outfile"]
`[1:],
		"action-log": `
action-log:
`[1:],
		"action-log-fail": `
action-log-fail:
`[1:],
		"action-log-fail-error": `
action-log-fail-error:
`[1:],
		"action-reboot": `
action-reboot:
`[1:],
	}
	actionsYamlPath := filepath.Join(path, "actions.yaml")
	var actionsYamlFull string
	// Build an appropriate actions.yaml
	if names[0] != "base" {
		names = append([]string{"base"}, names...)
	}
	for _, name := range names {
		actionsYamlFull = strings.Join(
			[]string{actionsYamlFull, actionsYaml[name]}, "\n")
	}
	err := ioutil.WriteFile(actionsYamlPath, []byte(actionsYamlFull), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (ctx *context) matchHooks(c *gc.C) (match bool, overshoot bool) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	c.Logf("ctx.hooksCompleted: %#v", ctx.hooksCompleted)
	if len(ctx.hooksCompleted) < len(ctx.hooks) {
		return false, false
	}
	for i, e := range ctx.hooks {
		if ctx.hooksCompleted[i] != e {
			return false, false
		}
	}
	return true, len(ctx.hooksCompleted) > len(ctx.hooks)
}

type uniterTest struct {
	summary string
	steps   []stepper
}

func ut(summary string, steps ...stepper) uniterTest {
	return uniterTest{summary, steps}
}

type stepper interface {
	step(c *gc.C, ctx *context)
}

func step(c *gc.C, ctx *context, s stepper) {
	c.Logf("%#v", s)
	s.step(c, ctx)
}

type ensureStateWorker struct{}

func (s ensureStateWorker) step(c *gc.C, ctx *context) {
	addresses, err := ctx.st.Addresses()
	if err != nil || len(addresses) == 0 {
		addStateServerMachine(c, ctx.st)
	}
	addresses, err = ctx.st.APIAddressesFromMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.HasLen, 1)
}

func addStateServerMachine(c *gc.C, st *state.State) {
	// The AddStateServerMachine call will update the API host ports
	// to made-up addresses. We need valid addresses so that the uniter
	// can download charms from the API server.
	apiHostPorts, err := st.APIHostPorts()
	c.Assert(err, gc.IsNil)
	testing.AddStateServerMachine(c, st)
	err = st.SetAPIHostPorts(apiHostPorts)
	c.Assert(err, gc.IsNil)
}

type createCharm struct {
	revision  int
	badHooks  []string
	customize func(*gc.C, *context, string)
}

var charmHooks = []string{
	"install", "start", "config-changed", "upgrade-charm", "stop",
	"db-relation-joined", "db-relation-changed", "db-relation-departed",
	"db-relation-broken", "meter-status-changed", "collect-metrics",
}

func (s createCharm) step(c *gc.C, ctx *context) {
	base := testcharms.Repo.ClonedDirPath(c.MkDir(), "wordpress")
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
		s.customize(c, ctx, base)
	}
	dir, err := corecharm.ReadCharmDir(base)
	c.Assert(err, jc.ErrorIsNil)
	err = dir.SetDiskRevision(s.revision)
	c.Assert(err, jc.ErrorIsNil)
	step(c, ctx, addCharm{dir, curl(s.revision)})
}

type addCharm struct {
	dir  *corecharm.CharmDir
	curl *corecharm.URL
}

func (s addCharm) step(c *gc.C, ctx *context) {
	var buf bytes.Buffer
	err := s.dir.ArchiveTo(&buf)
	c.Assert(err, jc.ErrorIsNil)
	body := buf.Bytes()
	hash, _, err := utils.ReadSHA256(&buf)
	c.Assert(err, jc.ErrorIsNil)

	storagePath := fmt.Sprintf("/charms/%s/%d", s.dir.Meta().Name, s.dir.Revision())
	ctx.charms[storagePath] = body
	ctx.sch, err = ctx.st.AddCharm(s.dir, s.curl, storagePath, hash)
	c.Assert(err, jc.ErrorIsNil)
}

type serveCharm struct{}

func (s serveCharm) step(c *gc.C, ctx *context) {
	storage := storage.NewStorage(ctx.st.EnvironUUID(), ctx.st.MongoSession())
	for storagePath, data := range ctx.charms {
		err := storage.Put(storagePath, bytes.NewReader(data), int64(len(data)))
		c.Assert(err, jc.ErrorIsNil)
		delete(ctx.charms, storagePath)
	}
}

type createServiceAndUnit struct {
	serviceName string
}

func (csau createServiceAndUnit) step(c *gc.C, ctx *context) {
	if csau.serviceName == "" {
		csau.serviceName = "u"
	}
	sch, err := ctx.st.Charm(curl(0))
	c.Assert(err, jc.ErrorIsNil)
	svc := ctx.s.AddTestingService(c, csau.serviceName, sch)
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// Assign the unit to a provisioned machine to match expected state.
	assertAssignUnit(c, ctx.st, unit)
	ctx.svc = svc
	ctx.unit = unit

	ctx.apiLogin(c)
}

type createUniter struct{}

func (createUniter) step(c *gc.C, ctx *context) {
	step(c, ctx, ensureStateWorker{})
	step(c, ctx, createServiceAndUnit{})
	step(c, ctx, startUniter{})
	step(c, ctx, waitAddresses{})
}

type waitAddresses struct{}

func (waitAddresses) step(c *gc.C, ctx *context) {
	timeout := time.After(worstCase)
	for {
		select {
		case <-timeout:
			c.Fatalf("timed out waiting for unit addresses")
		case <-time.After(coretesting.ShortWait):
			err := ctx.unit.Refresh()
			if err != nil {
				c.Fatalf("unit refresh failed: %v", err)
			}
			// GZ 2013-07-10: Hardcoded values from dummy environ
			//                special cased here, questionable.
			private, _ := ctx.unit.PrivateAddress()
			if private != "private.address.example.com" {
				continue
			}
			public, _ := ctx.unit.PublicAddress()
			if public != "public.address.example.com" {
				continue
			}
			return
		}
	}
}

type startUniter struct {
	unitTag string
}

func (s startUniter) step(c *gc.C, ctx *context) {
	if s.unitTag == "" {
		s.unitTag = "unit-u-0"
	}
	if ctx.uniter != nil {
		panic("don't start two uniters!")
	}
	if ctx.api == nil {
		panic("API connection not established")
	}
	tag, err := names.ParseUnitTag(s.unitTag)
	if err != nil {
		panic(err.Error())
	}
	locksDir := filepath.Join(ctx.dataDir, "locks")
	lock, err := fslock.NewLock(locksDir, "uniter-hook-execution")
	c.Assert(err, jc.ErrorIsNil)
	ctx.uniter = uniter.NewUniter(ctx.api, tag, ctx.dataDir, lock)
	uniter.SetUniterObserver(ctx.uniter, ctx)
}

type waitUniterDead struct {
	err string
}

func (s waitUniterDead) step(c *gc.C, ctx *context) {
	if s.err != "" {
		err := s.waitDead(c, ctx)
		c.Assert(err, gc.ErrorMatches, s.err)
		return
	}
	// In the default case, we're waiting for worker.ErrTerminateAgent, but
	// the path to that error can be tricky. If the unit becomes Dead at an
	// inconvenient time, unrelated calls can fail -- as they should -- but
	// not be detected as worker.ErrTerminateAgent. In this case, we restart
	// the uniter and check that it fails as expected when starting up; this
	// mimics the behaviour of the unit agent and verifies that the UA will,
	// eventually, see the correct error and respond appropriately.
	err := s.waitDead(c, ctx)
	if err != worker.ErrTerminateAgent {
		step(c, ctx, startUniter{})
		err = s.waitDead(c, ctx)
	}
	c.Assert(err, gc.Equals, worker.ErrTerminateAgent)
	err = ctx.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.unit.Life(), gc.Equals, state.Dead)
}

func (s waitUniterDead) waitDead(c *gc.C, ctx *context) error {
	u := ctx.uniter
	ctx.uniter = nil
	timeout := time.After(worstCase)
	for {
		// The repeated StartSync is to ensure timely completion of this method
		// in the case(s) where a state change causes a uniter action which
		// causes a state change which causes a uniter action, in which case we
		// need more than one sync. At the moment there's only one situation
		// that causes this -- setting the unit's service to Dying -- but it's
		// not an intrinsically insane pattern of action (and helps to simplify
		// the filter code) so this test seems like a small price to pay.
		ctx.s.BackingState.StartSync()
		select {
		case <-u.Dead():
			return u.Wait()
		case <-time.After(coretesting.ShortWait):
			continue
		case <-timeout:
			c.Fatalf("uniter still alive")
		}
	}
}

type stopUniter struct {
	err string
}

func (s stopUniter) step(c *gc.C, ctx *context) {
	u := ctx.uniter
	ctx.uniter = nil
	err := u.Stop()
	if s.err == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, s.err)
	}
}

type verifyWaiting struct{}

func (s verifyWaiting) step(c *gc.C, ctx *context) {
	step(c, ctx, stopUniter{})
	step(c, ctx, startUniter{})
	step(c, ctx, waitHooks{})
}

type verifyRunning struct{}

func (s verifyRunning) step(c *gc.C, ctx *context) {
	step(c, ctx, stopUniter{})
	step(c, ctx, startUniter{})
	step(c, ctx, waitHooks{"config-changed"})
}

type startupErrorWithCustomCharm struct {
	badHook   string
	customize func(*gc.C, *context, string)
}

func (s startupErrorWithCustomCharm) step(c *gc.C, ctx *context) {
	step(c, ctx, createCharm{
		badHooks:  []string{s.badHook},
		customize: s.customize,
	})
	step(c, ctx, serveCharm{})
	step(c, ctx, createUniter{})
	step(c, ctx, waitUnit{
		status: params.StatusError,
		info:   fmt.Sprintf(`hook failed: %q`, s.badHook),
	})
	for _, hook := range []string{"install", "config-changed", "start"} {
		if hook == s.badHook {
			step(c, ctx, waitHooks{"fail-" + hook})
			break
		}
		step(c, ctx, waitHooks{hook})
	}
	step(c, ctx, verifyCharm{})
}

type startupError struct {
	badHook string
}

func (s startupError) step(c *gc.C, ctx *context) {
	step(c, ctx, createCharm{badHooks: []string{s.badHook}})
	step(c, ctx, serveCharm{})
	step(c, ctx, createUniter{})
	step(c, ctx, waitUnit{
		status: params.StatusError,
		info:   fmt.Sprintf(`hook failed: %q`, s.badHook),
	})
	for _, hook := range []string{"install", "config-changed", "start"} {
		if hook == s.badHook {
			step(c, ctx, waitHooks{"fail-" + hook})
			break
		}
		step(c, ctx, waitHooks{hook})
	}
	step(c, ctx, verifyCharm{})
}

type quickStart struct{}

func (s quickStart) step(c *gc.C, ctx *context) {
	step(c, ctx, createCharm{})
	step(c, ctx, serveCharm{})
	step(c, ctx, createUniter{})
	step(c, ctx, waitUnit{status: params.StatusActive})
	step(c, ctx, waitHooks{"install", "config-changed", "start"})
	step(c, ctx, verifyCharm{})
}

type quickStartRelation struct{}

func (s quickStartRelation) step(c *gc.C, ctx *context) {
	step(c, ctx, quickStart{})
	step(c, ctx, addRelation{})
	step(c, ctx, addRelationUnit{})
	step(c, ctx, waitHooks{"db-relation-joined mysql/0 db:0", "db-relation-changed mysql/0 db:0"})
	step(c, ctx, verifyRunning{})
}

type startupRelationError struct {
	badHook string
}

func (s startupRelationError) step(c *gc.C, ctx *context) {
	step(c, ctx, createCharm{badHooks: []string{s.badHook}})
	step(c, ctx, serveCharm{})
	step(c, ctx, createUniter{})
	step(c, ctx, waitUnit{status: params.StatusActive})
	step(c, ctx, waitHooks{"install", "config-changed", "start"})
	step(c, ctx, verifyCharm{})
	step(c, ctx, addRelation{})
	step(c, ctx, addRelationUnit{})
}

type resolveError struct {
	resolved state.ResolvedMode
}

func (s resolveError) step(c *gc.C, ctx *context) {
	err := ctx.unit.SetResolved(s.resolved)
	c.Assert(err, jc.ErrorIsNil)
}

type waitUnit struct {
	status   params.Status
	info     string
	data     map[string]interface{}
	charm    int
	resolved state.ResolvedMode
}

func (s waitUnit) step(c *gc.C, ctx *context) {
	timeout := time.After(worstCase)
	for {
		ctx.s.BackingState.StartSync()
		select {
		case <-time.After(coretesting.ShortWait):
			err := ctx.unit.Refresh()
			if err != nil {
				c.Fatalf("cannot refresh unit: %v", err)
			}
			resolved := ctx.unit.Resolved()
			if resolved != s.resolved {
				c.Logf("want resolved mode %q, got %q; still waiting", s.resolved, resolved)
				continue
			}
			url, ok := ctx.unit.CharmURL()
			if !ok || *url != *curl(s.charm) {
				var got string
				if ok {
					got = url.String()
				}
				c.Logf("want unit charm %q, got %q; still waiting", curl(s.charm), got)
				continue
			}
			status, info, data, err := ctx.unit.Status()
			c.Assert(err, jc.ErrorIsNil)
			if string(status) != string(s.status) {
				c.Logf("want unit status %q, got %q; still waiting", s.status, status)
				continue
			}
			if info != s.info {
				c.Logf("want unit status info %q, got %q; still waiting", s.info, info)
				continue
			}
			if s.data != nil {
				if len(data) != len(s.data) {
					c.Logf("want %d unit status data value(s), got %d; still waiting", len(s.data), len(data))
					continue
				}
				for key, value := range s.data {
					if data[key] != value {
						c.Logf("want unit status data value %q for key %q, got %q; still waiting",
							value, key, data[key])
						continue
					}
				}
			}
			return
		case <-timeout:
			c.Fatalf("never reached desired status")
		}
	}
}

type waitHooks []string

func (s waitHooks) step(c *gc.C, ctx *context) {
	if len(s) == 0 {
		// Give unwanted hooks a moment to run...
		ctx.s.BackingState.StartSync()
		time.Sleep(coretesting.ShortWait)
	}
	ctx.hooks = append(ctx.hooks, s...)
	c.Logf("waiting for hooks: %#v", ctx.hooks)
	match, overshoot := ctx.matchHooks(c)
	if overshoot && len(s) == 0 {
		c.Fatalf("ran more hooks than expected")
	}
	if match {
		return
	}
	timeout := time.After(worstCase)
	for {
		ctx.s.BackingState.StartSync()
		select {
		case <-time.After(coretesting.ShortWait):
			if match, _ = ctx.matchHooks(c); match {
				return
			}
		case <-timeout:
			c.Fatalf("never got expected hooks")
		}
	}
}

type actionResult struct {
	name    string
	results map[string]interface{}
	status  string
	message string
}

type waitActionResults struct {
	expectedResults []actionResult
}

func (s waitActionResults) step(c *gc.C, ctx *context) {
	resultsWatcher := ctx.st.WatchActionResults()
	defer func() {
		c.Assert(resultsWatcher.Stop(), gc.IsNil)
	}()
	timeout := time.After(worstCase)
	for {
		ctx.s.BackingState.StartSync()
		select {
		case <-time.After(coretesting.ShortWait):
			continue
		case <-timeout:
			c.Fatalf("timed out waiting for action results")
		case changes, ok := <-resultsWatcher.Changes():
			c.Logf("Got changes: %#v", changes)
			c.Assert(ok, jc.IsTrue)
			stateActionResults, err := ctx.unit.CompletedActions()
			c.Assert(err, jc.ErrorIsNil)
			if len(stateActionResults) != len(s.expectedResults) {
				continue
			}
			actualResults := make([]actionResult, len(stateActionResults))
			for i, result := range stateActionResults {
				results, message := result.Results()
				actualResults[i] = actionResult{
					name:    result.Name(),
					results: results,
					status:  string(result.Status()),
					message: message,
				}
			}
			assertActionResultsMatch(c, actualResults, s.expectedResults)
			return
		}
	}
}

func assertActionResultsMatch(c *gc.C, actualIn []actionResult, expectIn []actionResult) {
	matches := 0
	desiredMatches := len(actualIn)
	c.Assert(len(actualIn), gc.Equals, len(expectIn))
findMatch:
	for _, expectedItem := range expectIn {
		// find expectedItem in actualIn
		for j, actualItem := range actualIn {
			// If we find a match, remove both items from their
			// respective slices, increment match count, and restart.
			if reflect.DeepEqual(actualItem, expectedItem) {
				actualIn = append(actualIn[:j], actualIn[j+1:]...)
				matches++
				continue findMatch
			}
		}
		// if we finish the whole thing without finding a match, we failed.
		c.Assert(actualIn, jc.DeepEquals, expectIn)
	}

	c.Assert(matches, gc.Equals, desiredMatches)
}

type verifyNoActionResults struct{}

func (s verifyNoActionResults) step(c *gc.C, ctx *context) {
	time.Sleep(coretesting.ShortWait)
	result, err := ctx.unit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 0)
}

type fixHook struct {
	name string
}

func (s fixHook) step(c *gc.C, ctx *context) {
	path := filepath.Join(ctx.path, "charm", "hooks", s.name)
	ctx.writeHook(c, path, true)
}

type changeMeterStatus struct {
	code string
	info string
}

func (s changeMeterStatus) step(c *gc.C, ctx *context) {
	err := ctx.unit.SetMeterStatus(s.code, s.info)
	c.Assert(err, jc.ErrorIsNil)
}

type metricsTick struct{}

func (s metricsTick) step(c *gc.C, ctx *context) {
	err := ctx.ticker.Tick()
	c.Assert(err, jc.ErrorIsNil)
}

type changeConfig map[string]interface{}

func (s changeConfig) step(c *gc.C, ctx *context) {
	err := ctx.svc.UpdateConfigSettings(corecharm.Settings(s))
	c.Assert(err, jc.ErrorIsNil)
}

type addAction struct {
	name   string
	params map[string]interface{}
}

func (s addAction) step(c *gc.C, ctx *context) {
	_, err := ctx.st.EnqueueAction(ctx.unit.Tag(), s.name, s.params)
	// _, err := ctx.unit.AddAction(s.name, s.params)
	c.Assert(err, jc.ErrorIsNil)
}

type upgradeCharm struct {
	revision int
	forced   bool
}

func (s upgradeCharm) step(c *gc.C, ctx *context) {
	curl := curl(s.revision)
	sch, err := ctx.st.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.svc.SetCharm(sch, s.forced)
	c.Assert(err, jc.ErrorIsNil)
	serveCharm{}.step(c, ctx)
}

type verifyCharm struct {
	revision          int
	attemptedRevision int
	checkFiles        ft.Entries
}

func (s verifyCharm) step(c *gc.C, ctx *context) {
	s.checkFiles.Check(c, filepath.Join(ctx.path, "charm"))
	path := filepath.Join(ctx.path, "charm", "revision")
	content, err := ioutil.ReadFile(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, strconv.Itoa(s.revision))
	checkRevision := s.revision
	if s.attemptedRevision > checkRevision {
		checkRevision = s.attemptedRevision
	}
	err = ctx.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	url, ok := ctx.unit.CharmURL()
	c.Assert(ok, jc.IsTrue)
	c.Assert(url, gc.DeepEquals, curl(checkRevision))
}

type startUpgradeError struct{}

func (s startUpgradeError) step(c *gc.C, ctx *context) {
	steps := []stepper{
		createCharm{
			customize: func(c *gc.C, ctx *context, path string) {
				appendHook(c, path, "start", "chmod 555 $CHARM_DIR")
			},
		},
		serveCharm{},
		createUniter{},
		waitUnit{
			status: params.StatusActive,
		},
		waitHooks{"install", "config-changed", "start"},
		verifyCharm{},

		createCharm{revision: 1},
		serveCharm{},
		upgradeCharm{revision: 1},
		waitUnit{
			status: params.StatusError,
			info:   "upgrade failed",
			charm:  1,
		},
		verifyWaiting{},
		verifyCharm{attemptedRevision: 1},
	}
	for _, s_ := range steps {
		step(c, ctx, s_)
	}
}

type verifyWaitingUpgradeError struct {
	revision int
}

func (s verifyWaitingUpgradeError) step(c *gc.C, ctx *context) {
	verifyCharmSteps := []stepper{
		waitUnit{
			status: params.StatusError,
			info:   "upgrade failed",
			charm:  s.revision,
		},
		verifyCharm{attemptedRevision: s.revision},
	}
	verifyWaitingSteps := []stepper{
		stopUniter{},
		custom{func(c *gc.C, ctx *context) {
			// By setting status to Started, and waiting for the restarted uniter
			// to reset the error status, we can avoid a race in which a subsequent
			// fixUpgradeError lands just before the restarting uniter retries the
			// upgrade; and thus puts us in an unexpected state for future steps.
			ctx.unit.SetStatus(state.StatusActive, "", nil)
		}},
		startUniter{},
	}
	allSteps := append(verifyCharmSteps, verifyWaitingSteps...)
	allSteps = append(allSteps, verifyCharmSteps...)
	for _, s_ := range allSteps {
		step(c, ctx, s_)
	}
}

type fixUpgradeError struct{}

func (s fixUpgradeError) step(c *gc.C, ctx *context) {
	charmPath := filepath.Join(ctx.path, "charm")
	err := os.Chmod(charmPath, 0755)
	c.Assert(err, jc.ErrorIsNil)
}

type addRelation struct {
	waitJoin bool
}

func (s addRelation) step(c *gc.C, ctx *context) {
	if ctx.relation != nil {
		panic("don't add two relations!")
	}
	if ctx.relatedSvc == nil {
		ctx.relatedSvc = ctx.s.AddTestingService(c, "mysql", ctx.s.AddTestingCharm(c, "mysql"))
	}
	eps, err := ctx.st.InferEndpoints("u", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	ctx.relation, err = ctx.st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ctx.relationUnits = map[string]*state.RelationUnit{}
	if !s.waitJoin {
		return
	}

	// It's hard to do this properly (watching scope) without perturbing other tests.
	ru, err := ctx.relation.Unit(ctx.unit)
	c.Assert(err, jc.ErrorIsNil)
	timeout := time.After(worstCase)
	for {
		c.Logf("waiting to join relation")
		select {
		case <-timeout:
			c.Fatalf("failed to join relation")
		case <-time.After(coretesting.ShortWait):
			inScope, err := ru.InScope()
			c.Assert(err, jc.ErrorIsNil)
			if inScope {
				return
			}
		}
	}
}

type addRelationUnit struct{}

func (s addRelationUnit) step(c *gc.C, ctx *context) {
	u, err := ctx.relatedSvc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	ru, err := ctx.relation.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx.relationUnits[u.Name()] = ru
}

type changeRelationUnit struct {
	name string
}

func (s changeRelationUnit) step(c *gc.C, ctx *context) {
	settings, err := ctx.relationUnits[s.name].Settings()
	c.Assert(err, jc.ErrorIsNil)
	key := "madness?"
	raw, _ := settings.Get(key)
	val, _ := raw.(string)
	if val == "" {
		val = "this is juju"
	} else {
		val += "u"
	}
	settings.Set(key, val)
	_, err = settings.Write()
	c.Assert(err, jc.ErrorIsNil)
}

type removeRelationUnit struct {
	name string
}

func (s removeRelationUnit) step(c *gc.C, ctx *context) {
	err := ctx.relationUnits[s.name].LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	ctx.relationUnits[s.name] = nil
}

type relationState struct {
	removed bool
	life    state.Life
}

func (s relationState) step(c *gc.C, ctx *context) {
	err := ctx.relation.Refresh()
	if s.removed {
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
		return
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.relation.Life(), gc.Equals, s.life)

}

type addSubordinateRelation struct {
	ifce string
}

func (s addSubordinateRelation) step(c *gc.C, ctx *context) {
	if _, err := ctx.st.Service("logging"); errors.IsNotFound(err) {
		ctx.s.AddTestingService(c, "logging", ctx.s.AddTestingCharm(c, "logging"))
	}
	eps, err := ctx.st.InferEndpoints("logging", "u:"+s.ifce)
	c.Assert(err, jc.ErrorIsNil)
	_, err = ctx.st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
}

type removeSubordinateRelation struct {
	ifce string
}

func (s removeSubordinateRelation) step(c *gc.C, ctx *context) {
	eps, err := ctx.st.InferEndpoints("logging", "u:"+s.ifce)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := ctx.st.EndpointsRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

type waitSubordinateExists struct {
	name string
}

func (s waitSubordinateExists) step(c *gc.C, ctx *context) {
	timeout := time.After(worstCase)
	for {
		ctx.s.BackingState.StartSync()
		select {
		case <-timeout:
			c.Fatalf("subordinate was not created")
		case <-time.After(coretesting.ShortWait):
			var err error
			ctx.subordinate, err = ctx.st.Unit(s.name)
			if errors.IsNotFound(err) {
				continue
			}
			c.Assert(err, jc.ErrorIsNil)
			return
		}
	}
}

type waitSubordinateDying struct{}

func (waitSubordinateDying) step(c *gc.C, ctx *context) {
	timeout := time.After(worstCase)
	for {
		ctx.s.BackingState.StartSync()
		select {
		case <-timeout:
			c.Fatalf("subordinate was not made Dying")
		case <-time.After(coretesting.ShortWait):
			err := ctx.subordinate.Refresh()
			c.Assert(err, jc.ErrorIsNil)
			if ctx.subordinate.Life() != state.Dying {
				continue
			}
		}
		break
	}
}

type removeSubordinate struct{}

func (removeSubordinate) step(c *gc.C, ctx *context) {
	err := ctx.subordinate.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.subordinate.Remove()
	c.Assert(err, jc.ErrorIsNil)
	ctx.subordinate = nil
}

type assertYaml struct {
	path   string
	expect map[string]interface{}
}

func (s assertYaml) step(c *gc.C, ctx *context) {
	data, err := ioutil.ReadFile(filepath.Join(ctx.path, s.path))
	c.Assert(err, jc.ErrorIsNil)
	actual := make(map[string]interface{})
	err = goyaml.Unmarshal(data, &actual)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, gc.DeepEquals, s.expect)
}

type writeFile struct {
	path string
	mode os.FileMode
}

func (s writeFile) step(c *gc.C, ctx *context) {
	path := filepath.Join(ctx.path, s.path)
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(path, nil, s.mode)
	c.Assert(err, jc.ErrorIsNil)
}

type chmod struct {
	path string
	mode os.FileMode
}

func (s chmod) step(c *gc.C, ctx *context) {
	path := filepath.Join(ctx.path, s.path)
	err := os.Chmod(path, s.mode)
	c.Assert(err, jc.ErrorIsNil)
}

type custom struct {
	f func(*gc.C, *context)
}

func (s custom) step(c *gc.C, ctx *context) {
	s.f(c, ctx)
}

var serviceDying = custom{func(c *gc.C, ctx *context) {
	c.Assert(ctx.svc.Destroy(), gc.IsNil)
}}

var relationDying = custom{func(c *gc.C, ctx *context) {
	c.Assert(ctx.relation.Destroy(), gc.IsNil)
}}

var unitDying = custom{func(c *gc.C, ctx *context) {
	c.Assert(ctx.unit.Destroy(), gc.IsNil)
}}

var unitDead = custom{func(c *gc.C, ctx *context) {
	c.Assert(ctx.unit.EnsureDead(), gc.IsNil)
}}

var subordinateDying = custom{func(c *gc.C, ctx *context) {
	c.Assert(ctx.subordinate.Destroy(), gc.IsNil)
}}

func curl(revision int) *corecharm.URL {
	return corecharm.MustParseURL("cs:quantal/wordpress").WithRevision(revision)
}

func appendHook(c *gc.C, charm, name, data string) {
	path := filepath.Join(charm, "hooks", name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0755)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	_, err = f.Write([]byte(data))
	c.Assert(err, jc.ErrorIsNil)
}

func renameRelation(c *gc.C, charmPath, oldName, newName string) {
	path := filepath.Join(charmPath, "metadata.yaml")
	f, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	meta, err := corecharm.ReadMeta(f)
	c.Assert(err, jc.ErrorIsNil)

	replace := func(what map[string]corecharm.Relation) bool {
		for relName, relation := range what {
			if relName == oldName {
				what[newName] = relation
				delete(what, oldName)
				return true
			}
		}
		return false
	}
	replaced := replace(meta.Provides) || replace(meta.Requires) || replace(meta.Peers)
	c.Assert(replaced, gc.Equals, true, gc.Commentf("charm %q does not implement relation %q", charmPath, oldName))

	newmeta, err := goyaml.Marshal(meta)
	c.Assert(err, jc.ErrorIsNil)
	ioutil.WriteFile(path, newmeta, 0644)

	f, err = os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	meta, err = corecharm.ReadMeta(f)
	c.Assert(err, jc.ErrorIsNil)
}

func createHookLock(c *gc.C, dataDir string) *fslock.Lock {
	lockDir := filepath.Join(dataDir, "locks")
	lock, err := fslock.NewLock(lockDir, "uniter-hook-execution")
	c.Assert(err, jc.ErrorIsNil)
	return lock
}

type acquireHookSyncLock struct {
	message string
}

func (s acquireHookSyncLock) step(c *gc.C, ctx *context) {
	lock := createHookLock(c, ctx.dataDir)
	c.Assert(lock.IsLocked(), jc.IsFalse)
	err := lock.Lock(s.message)
	c.Assert(err, jc.ErrorIsNil)
}

var releaseHookSyncLock = custom{func(c *gc.C, ctx *context) {
	lock := createHookLock(c, ctx.dataDir)
	// Force the release.
	err := lock.BreakLock()
	c.Assert(err, jc.ErrorIsNil)
}}

var verifyHookSyncLockUnlocked = custom{func(c *gc.C, ctx *context) {
	lock := createHookLock(c, ctx.dataDir)
	c.Assert(lock.IsLocked(), jc.IsFalse)
}}

var verifyHookSyncLockLocked = custom{func(c *gc.C, ctx *context) {
	lock := createHookLock(c, ctx.dataDir)
	c.Assert(lock.IsLocked(), jc.IsTrue)
}}

type setProxySettings proxy.Settings

func (s setProxySettings) step(c *gc.C, ctx *context) {
	attrs := map[string]interface{}{
		"http-proxy":  s.Http,
		"https-proxy": s.Https,
		"ftp-proxy":   s.Ftp,
		"no-proxy":    s.NoProxy,
	}
	err := ctx.st.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

type relationRunCommands []string

func (cmds relationRunCommands) step(c *gc.C, ctx *context) {
	commands := strings.Join(cmds, "\n")
	args := uniter.RunCommandsArgs{
		Commands:       commands,
		RelationId:     0,
		RemoteUnitName: "",
	}
	result, err := ctx.uniter.RunCommands(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Code, gc.Equals, 0)
	c.Check(string(result.Stdout), gc.Equals, "")
	c.Check(string(result.Stderr), gc.Equals, "")
}

type runCommands []string

func (cmds runCommands) step(c *gc.C, ctx *context) {
	commands := strings.Join(cmds, "\n")
	args := uniter.RunCommandsArgs{
		Commands:       commands,
		RelationId:     -1,
		RemoteUnitName: "",
	}
	result, err := ctx.uniter.RunCommands(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Code, gc.Equals, 0)
	c.Check(string(result.Stdout), gc.Equals, "")
	c.Check(string(result.Stderr), gc.Equals, "")
}

type asyncRunCommands []string

func (cmds asyncRunCommands) step(c *gc.C, ctx *context) {
	commands := strings.Join(cmds, "\n")
	args := uniter.RunCommandsArgs{
		Commands:       commands,
		RelationId:     -1,
		RemoteUnitName: "",
	}

	socketPath := filepath.Join(ctx.path, "run.socket")

	go func() {
		// make sure the socket exists
		client, err := rpc.Dial("unix", socketPath)
		c.Assert(err, jc.ErrorIsNil)
		defer client.Close()

		var result utilexec.ExecResponse
		err = client.Call(uniter.JujuRunEndpoint, args, &result)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result.Code, gc.Equals, 0)
		c.Check(string(result.Stdout), gc.Equals, "")
		c.Check(string(result.Stderr), gc.Equals, "")
	}()
}

type verifyFile struct {
	filename string
	content  string
}

func (verify verifyFile) fileExists() bool {
	_, err := os.Stat(verify.filename)
	return err == nil
}

func (verify verifyFile) checkContent(c *gc.C) {
	content, err := ioutil.ReadFile(verify.filename)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, verify.content)
}

func (verify verifyFile) step(c *gc.C, ctx *context) {
	if verify.fileExists() {
		verify.checkContent(c)
		return
	}
	c.Logf("waiting for file: %s", verify.filename)
	timeout := time.After(worstCase)
	for {
		select {
		case <-time.After(coretesting.ShortWait):
			if verify.fileExists() {
				verify.checkContent(c)
				return
			}
		case <-timeout:
			c.Fatalf("file does not exist")
		}
	}
}

// verify that the file does not exist
type verifyNoFile struct {
	filename string
}

func (verify verifyNoFile) step(c *gc.C, ctx *context) {
	c.Assert(verify.filename, jc.DoesNotExist)
	// Wait a short time and check again.
	time.Sleep(coretesting.ShortWait)
	c.Assert(verify.filename, jc.DoesNotExist)
}

// prepareGitUniter runs a sequence of uniter tests with the manifest deployer
// replacement logic patched out, simulating the effect of running an older
// version of juju that exclusively used a git deployer. This is useful both
// for testing the new deployer-replacement code *and* for running the old
// tests against the new, patched code to check that the tweaks made to
// accommodate the manifest deployer do not change the original behaviour as
// simulated by the patched-out code.
type prepareGitUniter struct {
	prepSteps []stepper
}

func (s prepareGitUniter) step(c *gc.C, ctx *context) {
	c.Assert(ctx.uniter, gc.IsNil, gc.Commentf("please don't try to patch stuff while the uniter's running"))
	newDeployer := func(charmPath, dataPath string, bundles charm.BundleReader) (charm.Deployer, error) {
		return charm.NewGitDeployer(charmPath, dataPath, bundles), nil
	}
	restoreNewDeployer := gt.PatchValue(&charm.NewDeployer, newDeployer)
	defer restoreNewDeployer()

	fixDeployer := func(deployer *charm.Deployer) error {
		return nil
	}
	restoreFixDeployer := gt.PatchValue(&charm.FixDeployer, fixDeployer)
	defer restoreFixDeployer()

	for _, prepStep := range s.prepSteps {
		step(c, ctx, prepStep)
	}
	if ctx.uniter != nil {
		step(c, ctx, stopUniter{})
	}
}

func ugt(summary string, steps ...stepper) uniterTest {
	return ut(summary, prepareGitUniter{steps})
}

type verifyGitCharm struct {
	revision int
	dirty    bool
}

func (s verifyGitCharm) step(c *gc.C, ctx *context) {
	charmPath := filepath.Join(ctx.path, "charm")
	if !s.dirty {
		revisionPath := filepath.Join(charmPath, "revision")
		content, err := ioutil.ReadFile(revisionPath)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(content), gc.Equals, strconv.Itoa(s.revision))
		err = ctx.unit.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		url, ok := ctx.unit.CharmURL()
		c.Assert(ok, jc.IsTrue)
		c.Assert(url, gc.DeepEquals, curl(s.revision))
	}

	// Before we try to check the git status, make sure expected hooks are all
	// complete, to prevent the test and the uniter interfering with each other.
	step(c, ctx, waitHooks{})
	step(c, ctx, waitHooks{})
	cmd := exec.Command("git", "status")
	cmd.Dir = filepath.Join(ctx.path, "charm")
	out, err := cmd.CombinedOutput()
	c.Assert(err, jc.ErrorIsNil)
	cmp := gc.Matches
	if s.dirty {
		cmp = gc.Not(gc.Matches)
	}
	c.Assert(string(out), cmp, "(# )?On branch master\nnothing to commit.*\n")
}

type startGitUpgradeError struct{}

func (s startGitUpgradeError) step(c *gc.C, ctx *context) {
	steps := []stepper{
		createCharm{
			customize: func(c *gc.C, ctx *context, path string) {
				appendHook(c, path, "start", "echo STARTDATA > data")
			},
		},
		serveCharm{},
		createUniter{},
		waitUnit{
			status: params.StatusActive,
		},
		waitHooks{"install", "config-changed", "start"},
		verifyGitCharm{dirty: true},

		createCharm{
			revision: 1,
			customize: func(c *gc.C, ctx *context, path string) {
				ft.File{"data", "<nelson>ha ha</nelson>", 0644}.Create(c, path)
				ft.File{"ignore", "anything", 0644}.Create(c, path)
			},
		},
		serveCharm{},
		upgradeCharm{revision: 1},
		waitUnit{
			status: params.StatusError,
			info:   "upgrade failed",
			charm:  1,
		},
		verifyWaiting{},
		verifyGitCharm{dirty: true},
	}
	for _, s_ := range steps {
		step(c, ctx, s_)
	}
}
