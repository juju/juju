package uniter

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/worker/uniter/charm"
	"launchpad.net/juju-core/worker/uniter/hook"
	"launchpad.net/tomb"
	"math/rand"
	"path/filepath"
	"time"
)

// Uniter implements the capabilities of the unit agent. It is not intended to
// implement the actual *behaviour* of the unit agent; that responsibility is
// delegated to Mode values, which are expected to react to events and direct
// the uniter's responses to them.
type Uniter struct {
	tomb    tomb.Tomb
	st      *state.State
	unit    *state.Unit
	service *state.Service

	dataDir  string
	baseDir  string
	toolsDir string
	charm    *charm.GitDir
	bundles  *charm.BundlesDir
	deployer *charm.Deployer
	sf       *StateFile
	rand     *rand.Rand

	*filter
}

// NewUniter creates a new Uniter which will install, run, and upgrade a
// charm on behalf of the named unit, by executing hooks and operations
// provoked by changes in st.
func NewUniter(st *state.State, name string, dataDir string) *Uniter {
	u := &Uniter{
		st:      st,
		dataDir: dataDir,
	}
	go func() {
		defer u.tomb.Done()
		u.tomb.Kill(u.loop(name))
	}()
	return u
}

func (u *Uniter) loop(name string) error {
	if err := u.init(name); err != nil {
		return err
	}

	// Announce our presence to the world.
	pinger, err := u.unit.SetAgentAlive()
	if err != nil {
		return err
	}
	defer watcher.Stop(pinger, &u.tomb)
	log.Printf("unit %q started pinger", u.unit)

	// Start filtering state change events for consumption by modes.
	u.filter = newFilter(u.unit)
	defer watcher.Stop(u.filter, &u.tomb)
	go func() {
		u.tomb.Kill(u.filter.Wait())
	}()

	// Run modes until we encounter an error.
	mode := ModeInit
	for err == nil {
		select {
		case <-u.tomb.Dying():
			err = tomb.ErrDying
		default:
			mode, err = mode(u)
		}
	}
	log.Printf("unit %q shutting down: %s", u.unit, err)
	return err
}

func (u *Uniter) init(name string) (err error) {
	defer trivial.ErrorContextf(&err, "failed to initialize uniter for unit %q", name)
	u.unit, err = u.st.Unit(name)
	if err != nil {
		return err
	}
	pathKey := u.unit.PathKey()
	u.toolsDir = environs.AgentToolsDir(u.dataDir, pathKey)
	if err := EnsureJujucSymlinks(u.toolsDir); err != nil {
		return err
	}
	u.baseDir = filepath.Join(u.dataDir, "agents", pathKey)
	if err := trivial.EnsureDir(filepath.Join(u.baseDir, "state")); err != nil {
		return err
	}
	u.service, err = u.st.Service(u.unit.ServiceName())
	if err != nil {
		return err
	}
	u.charm = charm.NewGitDir(filepath.Join(u.baseDir, "charm"))
	u.bundles = charm.NewBundlesDir(filepath.Join(u.baseDir, "state", "bundles"))
	u.deployer = charm.NewDeployer(filepath.Join(u.baseDir, "state", "deployer"))
	u.sf = NewStateFile(filepath.Join(u.baseDir, "state", "uniter"))
	u.rand = rand.New(rand.NewSource(time.Now().Unix()))
	return nil
}

func (u *Uniter) Stop() error {
	u.tomb.Kill(nil)
	return u.Wait()
}

func (u *Uniter) String() string {
	return "uniter for " + u.unit.Name()
}

func (u *Uniter) Dying() <-chan struct{} {
	return u.tomb.Dying()
}

func (u *Uniter) Wait() error {
	return u.tomb.Wait()
}

// errNoUpgrade indicates that the uniter should not upgrade its charm.
var errNoUpgrade = errors.New("no upgrades available")

// getUpgrade returns the charm to which the deployment should be upgraded,
// given the supplied service charm information. If no upgrade is appropriate,
// errNoUpgrade is returned.
func (u *Uniter) getUpgrade(ch *charmChange, mustForce bool) (*state.Charm, error) {
	proposed := ch.url
	log.Printf("considering upgrade request: %s", proposed)
	if mustForce && !ch.force {
		log.Printf("request denied: not forced")
		return nil, errNoUpgrade
	}
	select {
	case <-u.unitDying():
		log.Printf("request denied: unit is dying")
		return nil, errNoUpgrade
	default:
	}
	log.Printf("reading current charm")
	s, err := u.sf.Read()
	if err != nil {
		return nil, err
	}
	current := s.CharmURL
	if current == nil {
		current, err = charm.ReadCharmURL(u.charm)
		if err != nil {
			return nil, err
		}
	}
	log.Printf("current charm is %s", current)
	if *current == *proposed {
		log.Printf("request denied: already upgraded")
		return nil, errNoUpgrade
	}
	return u.st.Charm(proposed)
}

// deploy deploys the supplied charm, and sets follow-up hook operation state
// as indicated by reason.
func (u *Uniter) deploy(sch *state.Charm, reason Op) error {
	if reason != Install && reason != Upgrade {
		panic(fmt.Errorf("%q is not a deploy operation", reason))
	}
	s, err := u.sf.Read()
	if err != nil && err != ErrNoStateFile {
		return err
	}
	var hi *hook.Info
	if s != nil && (s.Op == RunHook || s.Op == Upgrade) {
		// If this upgrade interrupts a RunHook, we need to preserve the hook
		// info so that we can return to the appropriate error state. However,
		// if we're resuming (or have force-interrupted) an Upgrade, we also
		// need to preserve whatever hook info was preserved when we initially
		// started upgrading, to ensure we still return to the correct state.
		hi = s.Hook
	}
	url := sch.URL()
	if s == nil || s.OpStep != Done {
		log.Printf("fetching charm %q", url)
		bun, err := u.bundles.Read(sch, u.tomb.Dying())
		if err != nil {
			return err
		}
		if err = u.deployer.Stage(bun, url); err != nil {
			return err
		}
		log.Printf("deploying charm %q", url)
		if err = u.sf.Write(reason, Pending, hi, url); err != nil {
			return err
		}
		if err = u.deployer.Deploy(u.charm); err != nil {
			return err
		}
		if err = u.sf.Write(reason, Done, hi, url); err != nil {
			return err
		}
	}
	log.Printf("charm %q is deployed", url)
	if err := u.unit.SetCharm(sch); err != nil {
		return err
	}
	status := Queued
	if hi != nil {
		// If a hook operation was interrupted, restore it.
		status = Pending
	} else {
		// Otherwise, queue the relevant post-deploy hook.
		hi = &hook.Info{}
		switch reason {
		case Install:
			hi.Kind = hook.Install
		case Upgrade:
			hi.Kind = hook.UpgradeCharm
		}
	}
	return u.sf.Write(RunHook, status, hi, nil)
}

// errHookFailed indicates that a hook failed to execute, but that the Uniter's
// operation is not affected by the error.
var errHookFailed = errors.New("hook execution failed")

// runHook executes the supplied hook.Info in an appropriate hook context. If
// the hook itself fails to execute, it returns errHookFailed.
func (u *Uniter) runHook(hi hook.Info) error {
	// Prepare context.
	hookName := string(hi.Kind)
	if hi.Kind.IsRelation() {
		panic("relation hooks are not yet supported")
		// TODO: update relation context; get hook name.
	}
	hctxId := fmt.Sprintf("%s:%s:%d", u.unit.Name(), hookName, u.rand.Int63())
	hctx := server.HookContext{
		Service:    u.service,
		Unit:       u.unit,
		Id:         hctxId,
		RelationId: -1,
	}

	// Prepare server.
	getCmd := func(ctxId, cmdName string) (cmd.Command, error) {
		// TODO: switch to long-running server with single context;
		// use nonce in place of context id.
		if ctxId != hctxId {
			return nil, fmt.Errorf("expected context id %q, got %q", hctxId, ctxId)
		}
		return hctx.NewCommand(cmdName)
	}
	socketPath := filepath.Join(u.baseDir, "agent.socket")
	srv, err := server.NewServer(getCmd, socketPath)
	if err != nil {
		return err
	}
	go srv.Run()
	defer srv.Close()

	// Run the hook.
	if err := u.sf.Write(RunHook, Pending, &hi, nil); err != nil {
		return err
	}
	log.Printf("running %q hook", hookName)
	if err := hctx.RunHook(hookName, u.charm.Path(), u.toolsDir, socketPath); err != nil {
		log.Printf("hook failed: %s", err)
		return errHookFailed
	}
	if err := u.sf.Write(RunHook, Done, &hi, nil); err != nil {
		return err
	}
	log.Printf("ran %q hook", hookName)
	return u.commitHook(hi)
}

// commitHook ensures that state is consistent with the supplied hook, and
// that the fact of the hook's completion is persisted.
func (u *Uniter) commitHook(hi hook.Info) error {
	log.Printf("committing %q hook", hi.Kind)
	if hi.Kind.IsRelation() {
		panic("relation hooks are not yet supported")
		// TODO: commit relation state changes.
	}
	if err := u.charm.Snapshotf("Completed %q hook.", hi.Kind); err != nil {
		return err
	}
	if err := u.sf.Write(Abide, Pending, &hi, nil); err != nil {
		return err
	}
	log.Printf("committed %q hook", hi.Kind)
	return nil
}

// resolveError abstracts away the commonalities of different resolution
// operations. It does nothing when rm is ResolvedNone; otherwise, it calls
// the supplied resolveFunc and clears the unit's Resolved flag (regardless
// or resolveFunc success or failure). It returns true if rf succeeded and
// the flag was cleared successfully.
func (u *Uniter) resolveError(rm state.ResolvedMode, rf resolveFunc) (success bool, err error) {
	if rm == state.ResolvedNone {
		return false, nil
	}
	switch rm {
	case state.ResolvedRetryHooks:
		err = rf(u, true)
	case state.ResolvedNoHooks:
		err = rf(u, false)
	default:
		panic(fmt.Errorf("unhandled resolved mode %q", rf))
	}
	if e := u.unit.ClearResolved(); e != nil {
		err = e
	}
	return err == nil, nil
}

// resolveFunc values specify the uniter's response to user resolution of
// different kinds of errors.
type resolveFunc func(u *Uniter, retry bool) error

// resolveConflict commits the current state of the charm deployment,
// thereby resolving git conflicts.
func resolveConflict(u *Uniter, _ bool) error {
	log.Printf("resolving upgrade conflict")
	return u.charm.Snapshotf("Upgrade conflict resolved.")
}

// getResolveHook returns a resolveFunc that either runs or commits the
// supplied hook, depending on the value of the retry parameter.
func getResolveHook(hi hook.Info) resolveFunc {
	return func(u *Uniter, retry bool) error {
		log.Printf("resolving hook error")
		if retry {
			return u.runHook(hi)
		}
		return u.commitHook(hi)
	}
}
