---
myst:
  html_meta:
    description: "Juju workers reference: the worker.Worker interface, the relation between agents and their workers, the dependency engine that starts and stops workers, and where worker activity shows up in the logs."
---

(worker)=
# Worker

```{audience} juju-dev
```

In Juju, a **worker** is any type that implements the `worker.Worker`
interface (the `github.com/juju/worker/v5` package): `Kill`, which asks
the worker to stop and returns immediately, and `Wait`, which blocks
until the worker has completed and returns any error it met while
running or stopping. A worker is goroutine-safe: its life is bounded in
time, but the calls into it can come from any goroutine.

The term covers a range: the dependency engine itself, the workers the
engine starts on behalf of the agent's manifolds (the typical usage of
the term), and {ref}`watchers <the-agent-watchers>`.

## Agents and workers

An {ref}`agent <agent>` is a process; it runs workers. One agent runs
many workers at the same time, in parallel, and a
worker may run workers of its own: a worker that manages private child
workers carries them in a catacomb
(`github.com/juju/worker/v5/catacomb`), so an agent's content is a tree
of workers. The distinction matters when reading logs or code: an agent
runs workers; workers run workers.

The agent page renders the worker trees: the {ref}`controller agent's
<controller-agent>` around the embedded {ref}`Dqlite <database>`
database, and the machine cloud's as the execution chain from
{ref}`jujud <jujud>` to the charm.

## The dependency engine

Workers are started and supervised by the dependency engine
(`github.com/juju/worker/v5/dependency`), the library that turns an
agent's manifolds -- each a declaration of what to run, what inputs it
needs, and how to start it -- into a graph of running workers. The
engine starts a manifold's worker without waiting for its inputs and
restarts the worker as its inputs change. It never starts a second
worker for the same manifold: a duplicate start is a fatal error. And
when a worker starts, the engine bounces the workers that depend on it,
so a dependents' restart tracks its inputs' availability.

## The worker's lifecycle

A worker is started, runs, and stops. The engine logs each start and
stop at debug level ("`<worker name>` manifold worker started at
`<time>`", "`<worker name>` manifold worker stopped: `<error>`"), so
the start and stop of every worker is visible in the {ref}`debug log
<command-juju-debug-log>` and in the model's log file, which carries
the logs of all the workers running on the model's behalf (see
{ref}`log <log>`).

A stopped worker returns the error it stopped with. From that error
(passed through the manifold's error filter) the engine decides what
follows: a fatal error stops the whole engine; anything else means the
worker is started again, after a backoff.
