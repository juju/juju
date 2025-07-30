// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"io"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/objectstore"
	coretrace "github.com/juju/juju/core/trace"
)

// controllerWorker is a wrapper around a ObjectStore that adds tracing, without
// exposing the underlying ObjectStore.
type controllerWorker struct {
	catacomb catacomb.Catacomb

	objectStore TrackedObjectStore
	tracer      coretrace.Tracer
}

func newControllerWorker(
	objectStore TrackedObjectStore,
	tracer coretrace.Tracer,
) (*controllerWorker, error) {
	w := &controllerWorker{
		objectStore: objectStore,
		tracer:      tracer,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "tracked-object-store",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{w.objectStore},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// Kill stops the worker.
func (t *controllerWorker) Kill() {
	t.catacomb.Kill(nil)
}

// Wait blocks until the worker has completed.
func (t *controllerWorker) Wait() error {
	return t.catacomb.Wait()
}

// Get returns an io.ReadCloser for data at path, namespaced to the
// model.
func (t *controllerWorker) Get(ctx context.Context, path string) (_ io.ReadCloser, _ int64, err error) {
	ctx, span := coretrace.Start(coretrace.WithTracer(ctx, t.tracer), coretrace.NameFromFunc(),
		coretrace.WithAttributes(coretrace.StringAttr("objectstore.path", path)),
	)
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return t.objectStore.Get(ctx, path)
}

// GetBySHA256 returns an io.ReadCloser for the object with the given SHA256
// hash, namespaced to the model.
func (t *controllerWorker) GetBySHA256(ctx context.Context, sha256 string) (_ io.ReadCloser, _ int64, err error) {
	ctx, span := coretrace.Start(coretrace.WithTracer(ctx, t.tracer), coretrace.NameFromFunc(),
		coretrace.WithAttributes(coretrace.StringAttr("objectstore.sha256", sha256)),
	)
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return t.objectStore.GetBySHA256(ctx, sha256)
}

// GetBySHA256Prefix returns an io.ReadCloser for any object with the a SHA256
// hash starting with a given prefix, namespaced to the model.
func (t *controllerWorker) GetBySHA256Prefix(ctx context.Context, sha256Prefix string) (_ io.ReadCloser, _ int64, err error) {
	ctx, span := coretrace.Start(coretrace.WithTracer(ctx, t.tracer), coretrace.NameFromFunc(),
		coretrace.WithAttributes(coretrace.StringAttr("objectstore.sha256_prefix", sha256Prefix)),
	)
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return t.objectStore.GetBySHA256Prefix(ctx, sha256Prefix)
}

// Put stores data from reader at path, namespaced to the model.
func (t *controllerWorker) Put(ctx context.Context, path string, r io.Reader, length int64) (uuid objectstore.UUID, err error) {
	ctx, span := coretrace.Start(coretrace.WithTracer(ctx, t.tracer), coretrace.NameFromFunc(),
		coretrace.WithAttributes(
			coretrace.StringAttr("objectstore.path", path),
			coretrace.Int64Attr("objectstore.size", length),
		),
	)
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return t.objectStore.Put(ctx, path, r, length)
}

// PutAndCheckHash stores data from reader at path, namespaced to the model.
func (t *controllerWorker) PutAndCheckHash(ctx context.Context, path string, r io.Reader, size int64, sha384 string) (_ objectstore.UUID, err error) {
	ctx, span := coretrace.Start(coretrace.WithTracer(ctx, t.tracer), coretrace.NameFromFunc(),
		coretrace.WithAttributes(
			coretrace.StringAttr("objectstore.path", path),
			coretrace.Int64Attr("objectstore.size", size),
			coretrace.StringAttr("objectstore.sha384", sha384),
		),
	)
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return t.objectStore.PutAndCheckHash(ctx, path, r, size, sha384)
}

// Remove removes data at path, namespaced to the model.
func (t *controllerWorker) Remove(ctx context.Context, path string) (err error) {
	ctx, span := coretrace.Start(coretrace.WithTracer(ctx, t.tracer), coretrace.NameFromFunc(),
		coretrace.WithAttributes(coretrace.StringAttr("objectstore.path", path)),
	)
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := t.objectStore.Remove(ctx, path); err != nil {
		return errors.Annotatef(err, "removing object %q", path)
	}
	return nil
}

// RemoveAll removes all data for the namespaced model. It is destructive and
// should be used with caution. No objects will be retrievable after this call.
// This is expected to be used when the model is being removed or when the
// object store has been drained and is no longer needed.
func (t *controllerWorker) RemoveAll(ctx context.Context) (err error) {
	ctx, span := coretrace.Start(coretrace.WithTracer(ctx, t.tracer), coretrace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := t.objectStore.RemoveAll(ctx); err != nil {
		return errors.Annotatef(err, "removing all objects")
	}
	return nil
}

func (t *controllerWorker) Report() map[string]any {
	report := t.objectStore.Report()
	report["modelUUID"] = database.ControllerNS
	return report
}

func (t *controllerWorker) loop() error {
	<-t.catacomb.Dying()
	return t.catacomb.ErrDying()

}
