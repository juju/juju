package uniter

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/worker/uniter/charm"
	"launchpad.net/juju-core/worker/uniter/hook"
	"launchpad.net/juju-core/worker/uniter/jujuc"
	"launchpad.net/juju-core/worker/uniter/relation"
	"launchpad.net/tomb"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

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

	dataDir      string
	baseDir      string
	toolsDir     string
	relationsDir string
	charm        *charm.GitDir
	bundles      *charm.BundlesDir
	deployer     *charm.Deployer
	sf           *StateFile
	rand         *rand.Rand

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
	log.Printf("unit %q started", u.unit)

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
	log.Printf("unit %q shutting down: %s", u.unit, err)
	return err
}

func (u *Uniter) init(name string) (err error) {
	defer trivial.ErrorContextf(&err, "failed to initialize uniter for unit %q", name)
	u.unit, err = u.st.Unit(name)
	if err != nil {
		return err
	}
	ename := u.unit.EntityName()
	u.toolsDir = environs.AgentToolsDir(u.dataDir, ename)
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
	u.relationers = map[int]*Relationer{}
	u.relationHooks = make(chan hook.Info)
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
	hctx := &HookContext{
		service:        u.service,
		unit:           u.unit,
		id:             hctxId,
		relationId:     relationId,
		remoteUnitName: hi.RemoteUnit,
		relations:      map[int]*ContextRelation{},
	}
	for id, r := range u.relationers {
		hctx.relations[id] = r.Context()
	}

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
		if err := u.relationers[hi.RelationId].CommitHook(hi); err != nil {
			return err
		}
		if hi.Kind == hook.RelationBroken {
			delete(u.relationers, hi.RelationId)
		}
	}
	if err := u.charm.Snapshotf("Completed %q hook.", hi.Kind); err != nil {
		return err
	}
	if hi.Kind == hook.ConfigChanged {
		u.ranConfigChanged = true
	}
	if err := u.sf.Write(Continue, Pending, &hi, nil); err != nil {
		return err
	}
	log.Printf("committed %q hook", hi.Kind)
	return nil
}

// restoreRelations reconciles the supplied relation state dirs with the
// remote state of the corresponding relations.
func (u *Uniter) restoreRelations() error {
	dirs, err := relation.ReadAllStateDirs(u.relationsDir)
	if err != nil {
		return err
	}
	for id, dir := range dirs {
		// invalid is set to true when the relation state dir refers to a
		// dead or missing relation. So long as the dir is empty, this is
		// a reasonable state: it indicates that the previous execution
		// was interrupted in the process of joining or departing the
		// relation, and that in either case the dir should be removed to
		// bring local state in line with remote.
		invalid := false
		rel, err := u.st.Relation(id)
		if state.IsNotFound(err) {
			invalid = true
		} else if err != nil {
			return err
		}
		if err = u.addRelationer(rel, dir); err == state.ErrScopeDying {
			invalid = true
		} else if err != nil {
			return err
		}
		if invalid {
			if err := dir.Remove(); err != nil {
				return err
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
				return nil, err
			}
			switch rel.Life() {
			case state.Dying:
				if err := r.SetDying(); err != nil {
					return nil, err
				}
			case state.Dead:
				return nil, fmt.Errorf("had reference to dead relation %q", rel)
			}
		} else {
			// If at any point in this block the relation is discovered to be
			// Dead or Dying, or even removed, we simply continue to the next
			// id; we didn't know about this relation before, and can completely
			// ignore its changes. Only once we have entered a relation's scope,
			// by successfully calling addRelationer, do we take on any
			// obligation to deal with that relation in future.
			rel, err := u.st.Relation(id)
			if err != nil {
				if state.IsNotFound(err) {
					continue
				}
				return nil, err
			}
			if rel.Life() != state.Alive {
				continue
			}
			dir, err := relation.ReadStateDir(u.relationsDir, id)
			if err != nil {
				return nil, err
			}
			err = u.addRelationer(rel, dir)
			if err == nil {
				added = append(added, u.relationers[id])
				continue
			}
			e := dir.Remove()
			if err != state.ErrScopeDying {
				return nil, err
			}
			if e != nil {
				return nil, e
			}
		}
	}
	return added, nil
}

// addRelationer causes the unit agent to join the supplied relation, and to
// store persistent state in the supplied dir.
func (u *Uniter) addRelationer(rel *state.Relation, dir *relation.StateDir) error {
	ru, err := rel.Unit(u.unit)
	if err != nil {
		return err
	}
	r := NewRelationer(ru, dir, u.relationHooks)
	if err = r.Join(); err != nil {
		return err
	}
	u.relationers[rel.Id()] = r
	return nil
}

// ensureRelationersDying ensures that counterpart relation units are not being
// watched, and that the only relation hooks generated henceforth will be
// -departed and -broken.
func (u *Uniter) ensureRelationersDying() error {
	for _, r := range u.relationers {
		if err := r.SetDying(); err != nil {
			return err
		}
	}
	return nil
}

// startRelationHooks causes relation hooks to be delivered on the uniter's
// relationHooksChannel.
func (u *Uniter) startRelationHooks() {
	for _, r := range u.relationers {
		r.StartHooks()
	}
}

// stopRelationHooks prevents relation hooks from being delivered on the
// uniter's relationHooksChannel.
func (u *Uniter) stopRelationHooks(err *error) {
	for _, r := range u.relationers {
		if e := r.StopHooks(); e != nil && *err == nil {
			*err = e
		}
	}
}
