// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	stderrors "errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/agent/tools"
	corecharm "launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/charm/hooks"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/fslock"
	"launchpad.net/juju-core/worker/uniter/charm"
	"launchpad.net/juju-core/worker/uniter/hook"
	"launchpad.net/juju-core/worker/uniter/jujuc"
	"launchpad.net/juju-core/worker/uniter/relation"
)

var log = loggo.GetLogger("juju.worker.uniter")

// Uniter implements the capabilities of the unit agent. It is not intended to
// implement the actual *behaviour* of the unit agent; that responsibility is
// delegated to Mode values, which are expected to react to events and direct
// the uniter's responses to them.
type Uniter struct {
	tomb          tomb.Tomb
	st            *state.State
	f             *filter
	unit          *state.Unit
	service       *state.Service
	relationers   map[int]*Relationer
	relationHooks chan hook.Info
	uuid          string

	dataDir      string
	baseDir      string
	toolsDir     string
	relationsDir string
	charm        *charm.GitDir
	bundles      *charm.BundlesDir
	deployer     *charm.Deployer
	s            *State
	sf           *StateFile
	rand         *rand.Rand
	hookLock     *fslock.Lock

	ranConfigChanged bool
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

func (u *Uniter) loop(name string) (err error) {
	if err = u.init(name); err != nil {
		return err
	}
	log.Infof("unit %q started", u.unit)

	// Start filtering state change events for consumption by modes.
	u.f, err = newFilter(u.st, name)
	if err != nil {
		return err
	}
	defer watcher.Stop(u.f, &u.tomb)
	go func() {
		u.tomb.Kill(u.f.Wait())
	}()

	// Announce our presence to the world.
	pinger, err := u.unit.SetAgentAlive()
	if err != nil {
		return err
	}
	defer watcher.Stop(pinger, &u.tomb)

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
	log.Infof("unit %q shutting down: %s", u.unit, err)
	return err
}

func (u *Uniter) setupLocks() (err error) {
	lockDir := filepath.Join(u.dataDir, "locks")
	u.hookLock, err = fslock.NewLock(lockDir, "uniter-hook-execution")
	if err != nil {
		return err
	}
	if message := u.hookLock.Message(); u.hookLock.IsLocked() && message != "" {
		// Look to see if it was us that held the lock before.  If it was, we
		// should be safe enough to break it, as it is likely that we died
		// before unlocking, and have been restarted by upstart.
		parts := strings.SplitN(message, ":", 2)
		if len(parts) > 1 && parts[0] == u.unit.Name() {
			if err := u.hookLock.BreakLock(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (u *Uniter) init(name string) (err error) {
	defer utils.ErrorContextf(&err, "failed to initialize uniter for unit %q", name)
	u.unit, err = u.st.Unit(name)
	if err != nil {
		return err
	}
	if err = u.setupLocks(); err != nil {
		return err
	}
	ename := u.unit.Tag()
	u.toolsDir = tools.ToolsDir(u.dataDir, ename)
	if err := EnsureJujucSymlinks(u.toolsDir); err != nil {
		return err
	}
	u.baseDir = filepath.Join(u.dataDir, "agents", ename)
	u.relationsDir = filepath.Join(u.baseDir, "state", "relations")
	if err := os.MkdirAll(u.relationsDir, 0755); err != nil {
		return err
	}
	u.service, err = u.st.Service(u.unit.ServiceName())
	if err != nil {
		return err
	}
	var env *state.Environment
	env, err = u.st.Environment()
	if err != nil {
		return err
	}
	u.uuid = env.UUID()
	u.relationers = map[int]*Relationer{}
	u.relationHooks = make(chan hook.Info)
	u.charm = charm.NewGitDir(filepath.Join(u.baseDir, "charm"))
	u.bundles = charm.NewBundlesDir(filepath.Join(u.baseDir, "state", "bundles"))
	u.deployer = charm.NewDeployer(filepath.Join(u.baseDir, "state", "deployer"))
	u.sf = NewStateFile(filepath.Join(u.baseDir, "state", "uniter"))
	u.rand = rand.New(rand.NewSource(time.Now().Unix()))
	return nil
}

func (u *Uniter) Kill() {
	u.tomb.Kill(nil)
}

func (u *Uniter) Wait() error {
	return u.tomb.Wait()
}

func (u *Uniter) Stop() error {
	u.tomb.Kill(nil)
	return u.Wait()
}

func (u *Uniter) String() string {
	return "uniter for " + u.unit.Name()
}

func (u *Uniter) Dead() <-chan struct{} {
	return u.tomb.Dead()
}

// writeState saves uniter state with the supplied values, and infers the appropriate
// value of Started.
func (u *Uniter) writeState(op Op, step OpStep, hi *hook.Info, url *corecharm.URL) error {
	s := State{
		Started:  op == RunHook && hi.Kind == hooks.Start || u.s != nil && u.s.Started,
		Op:       op,
		OpStep:   step,
		Hook:     hi,
		CharmURL: url,
	}
	if err := u.sf.Write(s.Started, s.Op, s.OpStep, s.Hook, s.CharmURL); err != nil {
		return err
	}
	u.s = &s
	return nil
}

// deploy deploys the supplied charm URL, and sets follow-up hook operation state
// as indicated by reason.
func (u *Uniter) deploy(curl *corecharm.URL, reason Op) error {
	if reason != Install && reason != Upgrade {
		panic(fmt.Errorf("%q is not a deploy operation", reason))
	}
	var hi *hook.Info
	if u.s != nil && (u.s.Op == RunHook || u.s.Op == Upgrade) {
		// If this upgrade interrupts a RunHook, we need to preserve the hook
		// info so that we can return to the appropriate error state. However,
		// if we're resuming (or have force-interrupted) an Upgrade, we also
		// need to preserve whatever hook info was preserved when we initially
		// started upgrading, to ensure we still return to the correct state.
		hi = u.s.Hook
	}
	if u.s == nil || u.s.OpStep != Done {
		// Get the new charm bundle before announcing intention to use it.
		log.Infof("fetching charm %q", curl)
		sch, err := u.st.Charm(curl)
		if err != nil {
			return err
		}
		bun, err := u.bundles.Read(sch, u.tomb.Dying())
		if err != nil {
			return err
		}
		if err = u.deployer.Stage(bun, curl); err != nil {
			return err
		}

		// Set the new charm URL - this returns when the operation is complete,
		// at which point we can refresh the local copy of the unit to get a
		// version with the correct charm URL, and can go ahead and deploy
		// the charm proper.
		if err := u.f.SetCharm(curl); err != nil {
			return err
		}
		if err := u.unit.Refresh(); err != nil {
			return err
		}
		log.Infof("deploying charm %q", curl)
		if err = u.writeState(reason, Pending, hi, curl); err != nil {
			return err
		}
		if err = u.deployer.Deploy(u.charm); err != nil {
			return err
		}
		if err = u.writeState(reason, Done, hi, curl); err != nil {
			return err
		}
	}
	log.Infof("charm %q is deployed", curl)
	status := Queued
	if hi != nil {
		// If a hook operation was interrupted, restore it.
		status = Pending
	} else {
		// Otherwise, queue the relevant post-deploy hook.
		hi = &hook.Info{}
		switch reason {
		case Install:
			hi.Kind = hooks.Install
		case Upgrade:
			hi.Kind = hooks.UpgradeCharm
		}
	}
	return u.writeState(RunHook, status, hi, nil)
}

// errHookFailed indicates that a hook failed to execute, but that the Uniter's
// operation is not affected by the error.
var errHookFailed = stderrors.New("hook execution failed")

// runHook executes the supplied hook.Info in an appropriate hook context. If
// the hook itself fails to execute, it returns errHookFailed.
func (u *Uniter) runHook(hi hook.Info) (err error) {
	// Prepare context.
	if err = hi.Validate(); err != nil {
		return err
	}

	hookName := string(hi.Kind)
	relationId := -1
	if hi.Kind.IsRelation() {
		relationId = hi.RelationId
		if hookName, err = u.relationers[relationId].PrepareHook(hi); err != nil {
			return err
		}
	}
	hctxId := fmt.Sprintf("%s:%s:%d", u.unit.Name(), hookName, u.rand.Int63())
	// We want to make sure we don't block forever when locking, but take the
	// tomb into account.
	checkTomb := func() error {
		select {
		case <-u.tomb.Dying():
			return tomb.ErrDying
		default:
			// no-op to fall through to return.
		}
		return nil
	}
	lockMessage := fmt.Sprintf("%s: running hook %q", u.unit.Name(), hookName)
	if err = u.hookLock.LockWithFunc(lockMessage, checkTomb); err != nil {
		return err
	}
	defer u.hookLock.Unlock()

	ctxRelations := map[int]*ContextRelation{}
	for id, r := range u.relationers {
		ctxRelations[id] = r.Context()
	}
	apiAddrs, err := u.st.APIAddresses()
	if err != nil {
		return err
	}
	hctx := NewHookContext(u.unit, hctxId, u.uuid, relationId, hi.RemoteUnit,
		ctxRelations, apiAddrs)

	// Prepare server.
	getCmd := func(ctxId, cmdName string) (cmd.Command, error) {
		// TODO: switch to long-running server with single context;
		// use nonce in place of context id.
		if ctxId != hctxId {
			return nil, fmt.Errorf("expected context id %q, got %q", hctxId, ctxId)
		}
		return jujuc.NewCommand(hctx, cmdName)
	}
	socketPath := filepath.Join(u.baseDir, "agent.socket")
	srv, err := jujuc.NewServer(getCmd, socketPath)
	if err != nil {
		return err
	}
	go srv.Run()
	defer srv.Close()

	// Run the hook.
	if err := u.writeState(RunHook, Pending, &hi, nil); err != nil {
		return err
	}
	log.Infof("running %q hook", hookName)
	if err := hctx.RunHook(hookName, u.charm.Path(), u.toolsDir, socketPath); err != nil {
		log.Errorf("hook failed: %s", err)
		return errHookFailed
	}
	if err := u.writeState(RunHook, Done, &hi, nil); err != nil {
		return err
	}
	log.Infof("ran %q hook", hookName)
	return u.commitHook(hi)
}

// commitHook ensures that state is consistent with the supplied hook, and
// that the fact of the hook's completion is persisted.
func (u *Uniter) commitHook(hi hook.Info) error {
	log.Infof("committing %q hook", hi.Kind)
	if hi.Kind.IsRelation() {
		if err := u.relationers[hi.RelationId].CommitHook(hi); err != nil {
			return err
		}
		if hi.Kind == hooks.RelationBroken {
			delete(u.relationers, hi.RelationId)
		}
	}
	if err := u.charm.Snapshotf("Completed %q hook.", hi.Kind); err != nil {
		return err
	}
	if hi.Kind == hooks.ConfigChanged {
		u.ranConfigChanged = true
	}
	if err := u.writeState(Continue, Pending, &hi, nil); err != nil {
		return err
	}
	log.Infof("committed %q hook", hi.Kind)
	return nil
}

// restoreRelations reconciles the supplied relation state dirs with the
// remote state of the corresponding relations.
func (u *Uniter) restoreRelations() error {
	// TODO(dimitern): Get these from state, not from disk.
	dirs, err := relation.ReadAllStateDirs(u.relationsDir)
	if err != nil {
		return err
	}
	for id, dir := range dirs {
		remove := false
		rel, err := u.st.Relation(id)
		if errors.IsNotFoundError(err) {
			remove = true
		} else if err != nil {
			return err
		}
		if err = u.addRelation(rel, dir); err == state.ErrCannotEnterScope {
			remove = true
		} else if err != nil {
			return err
		}
		if remove {
			// If the previous execution was interrupted in the process of
			// joining or departing the relation, the directory will be empty
			// and the state is sane.
			if err := dir.Remove(); err != nil {
				return fmt.Errorf("cannot synchronize relation state: %v", err)
			}
		}
	}
	return nil
}

// updateRelations responds to changes in the life states of the relations
// with the supplied ids. If any id corresponds to an alive relation not
// known to the unit, the uniter will join that relation and return its
// relationer in the added list.
func (u *Uniter) updateRelations(ids []int) (added []*Relationer, err error) {
	for _, id := range ids {
		if r, found := u.relationers[id]; found {
			rel := r.ru.Relation()
			if err := rel.Refresh(); err != nil {
				return nil, fmt.Errorf("cannot update relation %q: %v", rel, err)
			}
			if rel.Life() == state.Dying {
				if err := r.SetDying(); err != nil {
					return nil, err
				} else if r.IsImplicit() {
					delete(u.relationers, id)
				}
			}
			continue
		}
		// Relations that are not alive are simply skipped, because they
		// were not previously known anyway.
		rel, err := u.st.Relation(id)
		if err != nil {
			if errors.IsNotFoundError(err) {
				continue
			}
			return nil, err
		}
		if rel.Life() != state.Alive {
			continue
		}
		// Make sure we ignore relations not implemented by the unit's charm
		ch, err := corecharm.ReadDir(u.charm.Path())
		if err != nil {
			return nil, err
		}
		if ep, err := rel.Endpoint(u.unit.ServiceName()); err != nil {
			return nil, err
		} else if !ep.ImplementedBy(ch) {
			log.Warningf("skipping relation with unknown endpoint %q", ep)
			continue
		}
		dir, err := relation.ReadStateDir(u.relationsDir, id)
		if err != nil {
			return nil, err
		}
		err = u.addRelation(rel, dir)
		if err == nil {
			added = append(added, u.relationers[id])
			continue
		}
		e := dir.Remove()
		if err != state.ErrCannotEnterScope {
			return nil, err
		}
		if e != nil {
			return nil, e
		}
	}
	if u.unit.IsPrincipal() {
		return added, nil
	}
	// If no Alive relations remain between a subordinate unit's service
	// and its principal's service, the subordinate must become Dying.
	keepAlive := false
	for _, r := range u.relationers {
		scope := r.ru.Endpoint().Scope
		if scope == corecharm.ScopeContainer && !r.dying {
			keepAlive = true
			break
		}
	}
	if !keepAlive {
		if err := u.unit.Destroy(); err != nil {
			return nil, err
		}
	}
	return added, nil
}

// addRelation causes the unit agent to join the supplied relation, and to
// store persistent state in the supplied dir.
func (u *Uniter) addRelation(rel *state.Relation, dir *relation.StateDir) error {
	log.Infof("joining relation %q", rel)
	ru, err := rel.Unit(u.unit)
	if err != nil {
		return err
	}
	r := NewRelationer(ru, dir, u.relationHooks)
	w := u.unit.Watch()
	defer watcher.Stop(w, &u.tomb)
	for {
		select {
		case <-u.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-w.Changes():
			if !ok {
				return watcher.MustErr(w)
			}
			if err := r.Join(); err == state.ErrCannotEnterScopeYet {
				log.Infof("cannot enter scope for relation %q; waiting for subordinate to be removed", rel)
				continue
			} else if err != nil {
				return err
			}
			log.Infof("joined relation %q", rel)
			u.relationers[rel.Id()] = r
			return nil
		}
	}
}
