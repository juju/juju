package storage

import (
	"fmt"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v4/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/worker/uniter/hook"
)

var logger = loggo.GetLogger("juju.worker.uniter.storage")

type storage struct {
	st         *uniter.State
	hooks      chan hook.Info
	abort      <-chan struct{}
	dying      bool
	hookSender hook.Sender

	changes chan hook.SourceChange
	mu      sync.Mutex
	changed set.Strings
}

func NewStorage(st *uniter.State, tag names.UnitTag, abort <-chan struct{}) (*storage, error) {
	s := &storage{
		st:      st,
		hooks:   make(chan hook.Info),
		abort:   abort,
		changes: make(chan hook.SourceChange, 1),
		changed: make(set.Strings),
	}
	return s, nil
}

func (s *storage) Hooks() <-chan hook.Info {
	return s.hooks
}

func (s *storage) PrepareHook(hi hook.Info) (string, error) {
	logger.Debugf("PrepareHook(%+v)", hi)
	// TODO(axw) add utility function to juju/names for extracting storage
	// name from ID.
	name := hi.StorageId[:strings.IndexRune(hi.StorageId, '/')]
	return fmt.Sprintf("%s-%s", name, hi.Kind), nil
}

func (s *storage) CommitHook(hi hook.Info) error {
	logger.Debugf("CommitHook(%+v)", hi)
	return nil
}

func (s *storage) Update(ids []string) error {
	logger.Debugf("Update(%q)", ids)
	s.mu.Lock()
	for _, id := range ids {
		s.changed.Add(id)
	}
	s.mu.Unlock()

	select {
	case <-s.abort:
		return tomb.ErrDying
	case s.changes <- func() error { return nil }:
	default: // s.changes already signaled
	}
	return nil
}

func (s *storage) StartHooks() {
	if s.hookSender != nil {
		panic("hooks already started")
	}
	logger.Debugf("starting storage hooks")
	s.hookSender = hook.NewSender(s.hooks, s)
}

func (s *storage) StopHooks() error {
	logger.Debugf("stopping storage hooks")
	if s.hookSender == nil {
		return nil
	}
	return s.hookSender.Stop()
}

// SetDying informs the storage hook source to deliver only hooks to notify
// charms of impending storage departure, and thence destruction/detachment.
func (s *storage) SetDying() error {
	if err := s.StopHooks(); err != nil {
		return errors.Annotate(err, "stopping storage hooks")
	}
	s.dying = true
	s.StartHooks()
	return nil
}

// Changes is part of the hook.Source interface.
func (s *storage) Changes() <-chan hook.SourceChange {
	return s.changes
}

// Stop is part of the hook.Source interface.
func (s *storage) Stop() error {
	logger.Debugf("Stop")
	select {
	case _, ok := <-s.changes:
		if ok {
			return nil
		}
	}
	close(s.changes)
	return nil
}

// Next is part of the hook.Source interface.
func (s *storage) Next() hook.Info {
	s.mu.Lock()
	values := s.changed.SortedValues()
	s.mu.Unlock()
	head := values[0]

	// TODO(axw) when we track storage lifecycle, decide the hook
	// kind from the transitions:
	//     unknown->alive: storage-attached
	//     alive->dying:   storage-detaching
	//     dying->dead:    storage-detached
	hi := hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: head,
	}
	logger.Debugf("Next returning %+v", hi)
	return hi
}

// Pop is part of the hook.Source interface.
func (s *storage) Pop() {
	logger.Debugf("Pop")
	s.mu.Lock()
	defer s.mu.Unlock()
	head := s.changed.SortedValues()[0]
	s.changed.Remove(head)
}

// Empty is part of the hook.Source interface.
func (s *storage) Empty() bool {
	s.mu.Lock()
	empty := len(s.changed) == 0
	s.mu.Unlock()
	logger.Debugf("Empty: %v", empty)
	return empty
}
