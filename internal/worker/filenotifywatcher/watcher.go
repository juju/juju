// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

import (
	"context"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
)

const (
	defaultWatcherPath = "/var/lib/juju/locks"
)

// FileWatcher is an interface that allows a worker to watch a file for changes.
type FileWatcher interface {
	worker.Worker
	// Changes returns a channel that will receive a value whenever the
	// watched file changes.
	Changes() <-chan bool
}

// INotifyWatcher is an interface that allows a worker to watch a file for
// changes using inotify.
type INotifyWatcher interface {
	// Watch adds the given file or directory (non-recursively) to the watch.
	Watch(path string) error

	// Events returns the next event.
	Events() <-chan fsnotify.Event

	// Errors returns the next error.
	Errors() <-chan error

	// Close removes all watches and closes the events channel.
	Close() error
}

type option struct {
	path      string
	logger    logger.Logger
	watcherFn func() (INotifyWatcher, error)
}

type Option func(*option)

// WithPath is an option for NewWatcher that specifies the path to watch.
func WithPath(path string) Option {
	return func(o *option) {
		o.path = path
	}
}

// WithLogger is an option for NewWatcher that specifies the logger to use.
func WithLogger(logger logger.Logger) Option {
	return func(o *option) {
		o.logger = logger
	}
}

// WithINotifyWatcherFn is an option for NewWatcher that specifies the inotify
// watcher to use.
func WithINotifyWatcherFn(watcherFn func() (INotifyWatcher, error)) Option {
	return func(o *option) {
		o.watcherFn = watcherFn
	}
}

func newOption() *option {
	return &option{
		path:      defaultWatcherPath,
		logger:    internallogger.GetLogger("juju.worker.filenotifywatcher"),
		watcherFn: newWatcher,
	}
}

// NewInotifyWatcher returns a new INotifyWatcher.
var NewINotifyWatcher = newWatcher

type Watcher struct {
	catacomb catacomb.Catacomb

	fileName string
	changes  chan bool

	watchPath string
	watcher   INotifyWatcher

	logger logger.Logger
}

// NewWatcher returns a new FileWatcher that watches the given fileName in the
// given path.
func NewWatcher(fileName string, opts ...Option) (FileWatcher, error) {
	o := newOption()
	for _, opt := range opts {
		opt(o)
	}

	// Ensure that we create the watch path.
	if _, err := os.Stat(o.path); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(o.path, 0755); err != nil {
			o.logger.Infof(context.Background(), "failed watching file %q in path %q: %v", fileName, o.path, err)
			return newNoopFileWatcher(), nil
		}
	}

	watcher, err := o.watcherFn()
	if err != nil {
		return nil, errors.Annotatef(err, "creating watcher for file %q in path %q", fileName, o.path)
	}

	if err := watcher.Watch(o.path); err != nil {
		// As this is only used for debugging, we don't want to fail if we can't
		// watch the folder.
		o.logger.Infof(context.Background(), "failed watching file %q in path %q: %v", fileName, o.path, err)
		_ = watcher.Close()
		return newNoopFileWatcher(), nil
	}

	w := &Watcher{
		fileName:  fileName,
		changes:   make(chan bool),
		watcher:   watcher,
		watchPath: filepath.Join(o.path, fileName),
		logger:    o.logger,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "file-watcher",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *Watcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Watcher) Wait() error {
	return w.catacomb.Wait()
}

// Changes returns the changes for the given fileName.
func (w *Watcher) Changes() <-chan bool {
	return w.changes
}

func (w *Watcher) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	defer func() {
		_ = w.watcher.Close()
		close(w.changes)
	}()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case event := <-w.watcher.Events():
			if w.logger.IsLevelEnabled(logger.TRACE) {
				w.logger.Tracef(ctx, "inotify event for %v", event)
			}
			// Ignore events for other files in the directory.
			if event.Name != w.watchPath {
				continue
			}
			// If the event is not a create or delete event, ignore it.
			opType := parseOpType(event.Op)
			if opType == unknown {
				continue
			}

			if w.logger.IsLevelEnabled(logger.TRACE) {
				w.logger.Tracef(ctx, "dispatch event for fileName %q: %v", w.fileName, event)
			}

			w.changes <- opType == created

		case err := <-w.watcher.Errors():
			w.logger.Errorf(ctx, "error watching fileName %q with %v", w.fileName, err)
		}
	}
}

func (w *Watcher) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

// opType normalizes the fsnotify op type, to known types.
type opType int

const (
	unknown opType = iota
	created
	deleted
)

// parseOpType returns the op type for the given op.
// It expects that created and deleted can never be set at the same time.
func parseOpType(m fsnotify.Op) opType {
	if m.Has(fsnotify.Create) {
		return created
	}
	if m.Has(fsnotify.Remove) {
		return deleted
	}
	return unknown
}

type noopFileWatcher struct {
	tomb tomb.Tomb
	ch   chan bool
}

func newNoopFileWatcher() *noopFileWatcher {
	w := &noopFileWatcher{
		ch: make(chan bool),
	}

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})

	return w
}

func (w *noopFileWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *noopFileWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *noopFileWatcher) Changes() <-chan bool {
	return w.ch
}
