package uniter

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/presence"
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
// delegated to Mode values, which are expected to use the capabilities of the
// uniter to react appropriately to changes in the system.
type Uniter struct {
	tomb    tomb.Tomb
	st      *state.State
	unit    *state.Unit
	service *state.Service
	pinger  *presence.Pinger

	baseDir  string
	charm    *charm.GitDir
	bundles  *charm.BundlesDir
	deployer *charm.Deployer
	sf       *StateFile
	rand     *rand.Rand
}

// NewUniter creates a new Uniter which will install, run, and upgrade a
// charm on behalf of the named unit, by executing hooks and operations
// provoked by changes in st.
func NewUniter(st *state.State, name string, dataDir string) (u *Uniter, err error) {
	defer trivial.ErrorContextf(&err, "failed to create uniter for unit %q", name)
	unit, err := st.Unit(name)
	if err != nil {
		return nil, err
	}
	baseDir, err := ensureFs(dataDir, unit)
	if err != nil {
		return nil, err
	}
	service, err := st.Service(unit.ServiceName())
	if err != nil {
		return nil, err
	}
	pinger, err := unit.SetAgentAlive()
	if err != nil {
		return nil, err
	}
	u = &Uniter{
		st:       st,
		unit:     unit,
		service:  service,
		pinger:   pinger,
		baseDir:  baseDir,
		charm:    charm.NewGitDir(filepath.Join(baseDir, "charm")),
		bundles:  charm.NewBundlesDir(filepath.Join(baseDir, "state", "bundles")),
		deployer: charm.NewDeployer(filepath.Join(baseDir, "state", "deployer")),
		sf:       NewStateFile(filepath.Join(baseDir, "state", "uniter")),
		rand:     rand.New(rand.NewSource(time.Now().Unix())),
	}
	go u.loop()
	return u, nil
}

func (u *Uniter) loop() {
	var err error
	mode := ModeInit
	for mode != nil {
		mode, err = mode(u)
	}
	log.Printf("uniter shutting down: %s", err)
	u.tomb.Kill(err)
	u.tomb.Kill(u.pinger.Stop())
	u.tomb.Done()
}

func (u *Uniter) Stop() error {
	u.tomb.Kill(nil)
	return u.Wait()
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
	log.Printf("running hook %q", hookName)
	if err := hctx.RunHook(hookName, u.charm.Path(), socketPath); err != nil {
		log.Printf("hook failed: %s", err)
		return errHookFailed
	}
	log.Printf("hook succeeded")
	return u.commitHook(hi)
}

// commitHook ensures that state is consistent with the supplied hook, and
// that the fact of the hook's completion is persisted.
func (u *Uniter) commitHook(hi hook.Info) error {
	if err := u.sf.Write(RunHook, Done, &hi, nil); err != nil {
		return err
	}
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
	log.Printf("hook complete")
	return nil
}

// ensureFs ensures that files and directories required by the named uniter
// exist inside dataDir. It returns the path to the directory within which
// the uniter must store its data.
func ensureFs(dataDir string, unit *state.Unit) (string, error) {
	// TODO: do this OAOO at packaging time?
	if err := EnsureJujucSymlinks(dataDir, unit.PathKey()); err != nil {
		return "", err
	}
	path := filepath.Join(dataDir, "agents", unit.PathKey())
	if err := trivial.EnsureDir(filepath.Join(path, "state")); err != nil {
		return "", err
	}
	return path, nil
}
