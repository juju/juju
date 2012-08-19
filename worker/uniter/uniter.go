package uniter

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/worker/uniter/charm"
	"launchpad.net/juju-core/worker/uniter/hook"
	"launchpad.net/tomb"
	"math/rand"
	"path/filepath"
	"strings"
	"time"
)

// Uniter implements the capabilities of the unit agent. It is not intended to
// implement the actual *behaviour* of the unit agent; that responsibility is
// delegated to Mode values, which are expected to use the capabilities of the
// uniter to react appropriately to changes in the system.
type Uniter struct {
	tomb    tomb.Tomb
	path    string
	hook    *hook.StateFile
	charm   *charm.StateFile
	bundles *charm.BundlesDir
	rand    *rand.Rand
	unit    *state.Unit
	service *state.Service
	pinger  *presence.Pinger
}

// NewUniter creates a new Uniter which will install, run, and upgrade a
// charm on behalf of the named unit, by executing hooks and operations
// provoked by changes in st.
func NewUniter(st *state.State, name string) (*Uniter, error) {
	path, err := ensureFs(name)
	if err != nil {
		return nil, err
	}
	unit, err := st.Unit(name)
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
	statePath := func(name string) string {
		return filepath.Join(path, "state", name)
	}
	u := &Uniter{
		path:    path,
		hook:    hook.NewStateFile(statePath("hook")),
		charm:   charm.NewStateFile(statePath("charm")),
		bundles: charm.NewBundlesDir(statePath("bundles")),
		rand:    rand.New(rand.NewSource(time.Now().Unix())),
		unit:    unit,
		service: service,
		pinger:  pinger,
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
	u.tomb.Kill(err)
	u.tomb.Kill(u.pinger.Stop())
	u.tomb.Done()
}

// Stop stops the uniter, and returns any error encountered when running or
// shutting down.
func (u *Uniter) Stop() error {
	u.tomb.Kill(nil)
	return u.tomb.Wait()
}

// Err returns the error that caused the uniter to shut down, or
// tomb.ErrStillAlive if the uniter is still running.
func (u *Uniter) Err() error {
	return u.tomb.Err()
}

// changeCharm writes the service's current charm into the unit's charm
// directory. It returns true if the charm was written successfully, and
// false if the supplied charm was already in place. Before the charm
// directory is changed, the charm status will be set to reason.
func (u *Uniter) changeCharm(reason charm.Status) (bool, error) {
	sch, err := u.service.Charm()
	if err != nil {
		return false, err
	}
	if ucurl, err := u.unit.CharmURL(); err != nil {
		if _, ok := err.(*state.NotFoundError); !ok {
			return false, err
		}
	} else if *ucurl == *sch.URL() {
		// The required bundle has already been unpacked into the charm dir.
		return false, nil
	}
	bun, err := u.bundles.Read(sch, &u.tomb)
	if err != nil {
		return false, err
	}
	if err = u.charm.Write(reason); err != nil {
		return false, err
	}
	if err = bun.ExpandTo(u.charmPath()); err != nil {
		return false, err
	}
	// Update the unit's charm url to match the new reality.
	if err = u.unit.SetCharmURL(sch.URL()); err != nil {
		return false, err
	}
	return true, nil
}

// errHookFailed indicates that a hook failed to execute, but that the Uniter's
// operation is not affected by the error.
var errHookFailed = errors.New("hook execution failed")

// runHook executes the supplied hook.Info in an appropriate hook context. If
// the hook itself fails to execute, it returns errHookFailed.
func (u *Uniter) runHook(hi hook.Info) error {
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
	socketPath := filepath.Join(u.path, "agent.socket")
	srv, err := server.NewServer(hctx.CmdGetter(), socketPath)
	if err != nil {
		return err
	}
	go srv.Run()
	defer srv.Close()
	if err := u.hook.Write(hi, hook.StatusStarted); err != nil {
		return err
	}
	if err := hctx.RunHook(hookName, u.charmPath(), socketPath); err != nil {
		return errHookFailed
	}
	if err := u.hook.Write(hi, hook.StatusSucceeded); err != nil {
		return err
	}
	return u.commitHook(hi)
}

// commitHook ensures that state is consistent with the supplied hook, and
// that the fact of the hook's completion is persisted.
func (u *Uniter) commitHook(hi hook.Info) error {
	if err := u.syncState(hi); err != nil {
		return err
	}
	return u.hook.Write(hi, hook.StatusCommitted)
}

// syncState ensures that state changes implied by completion of the
// operation associated with the supplied hook have been persisted.
// It does not record the fact of the hook's execution.
func (u *Uniter) syncState(hi hook.Info) error {
	if hi.Kind.IsRelation() {
		panic("relation hooks are not yet supported")
		// TODO: commit relation state changes.
	}
	if hi.Kind == hook.UpgradeCharm {
		if err := u.unit.ClearNeedsUpgrade(); err != nil {
			return err
		}
	}
	if hi.Kind.IsCharmChange() {
		if err := u.charm.Write(charm.Installed); err != nil {
			return err
		}
	}
	return nil
}

// charmPath returns the path to the unit's charm directory.
func (u *Uniter) charmPath() string {
	return filepath.Join(u.path, "charm")
}

// ensureFs ensures that files and directories required by the named uniter
// exist. It returns the path to the directory within which the uniter must
// store its data.
func ensureFs(name string) (string, error) {
	// TODO: do this OAOO at packaging time.
	if err := EnsureJujucSymlinks(name); err != nil {
		return "", err
	}
	path := filepath.Join(environs.VarDir, "units", strings.Replace(name, "/", "-", 1))
	if err := trivial.EnsureDir(filepath.Join(path, "state")); err != nil {
		return "", err
	}
	return path, nil
}
