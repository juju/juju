# Context parameter in functions

Summary: All functions that require a context should have a `context.Context`
parameter as the first parameter.

## Context

In Go, the `context.Context` type is used to pass request-scoped values,
deadlines, and cancellation signals across API boundaries and between processes.
It is important to pass the context to all functions that require it to ensure
that the context is propagated correctly.

Further more, dqlite requires a context to be passed to all functions that
interact with the database. It is important to pass the context to all functions
that interact with the database to ensure that we have the ability to cancel the
operation if needed.

## Decision

All functions that require a context should have a `context.Context` parameter
as the first parameter. If a method or a function can not locate the context
from either the request or the facade request, it should either be based of a
tomb or a catacomb. If it's not possible to do that, then the function should be
refactored to accept a context as a parameter.

There should be no calls of `context.Background()` without tying it to a source;
either a request, a facade request, a tomb or a catacomb in non-test code. There
should be no calls of `context.TODO()` including non-test code, without a valid
reason that must be documented and approved by the project lead.

To source the context, the following rules should be followed for different
types of component:

### API Requests

Methods that handle http.Requests should use the request's context.

Example:

```go
func (s *Server) Get(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    // Do something with the context
}
```

### Facade Requests

Methods that handle facade requests should add a context parameter as the first parameter.


Example:

```go

func (s *Facade) Get(ctx context.Context, req params.EntityTags) (params.ErrorResults, error) {
    // Do something with the context
}
```

### Workers

Workers that require a context, the context should be tied to either a tomb.Tomb
or a catacomb.Catacomb. This ensures that the context is cancelled when the
worker is stopped.

Example:

```go
type Worker struct {
    tomb tomb.Tomb
}

func newWorker() *Worker {
    w := &Worker{}
    w.tomb.Go(w.run)
    return w
}

func (w *Worker) run() error {
    ctx, cancel := w.scopedContext()
    defer cancel()

    for {
        select {
        case <-w.tomb.Dying():
            return tomb.ErrDying
        default:
            // Do something with the context
        }
    }
}

func (w *Worker) scopedContext() (context.Context, context.CancelFunc) {
    return context.WithCancel(w.tomb.Context(context.Background()))
}
```

### Tests

`go.C` tests should use a context.Background().

Example:

```go
func (*Suite) TestSomething(t *gc.C) {
    ctx := context.Background()
    // Do something with the context
}
```

Standard library `testing.T` should using `T.Context()`.

Example:

```go
func TestSomething(t *testing.T) {
    ctx := t.Context()
    // Do something with the context
}
```

