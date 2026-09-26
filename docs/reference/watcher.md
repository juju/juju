---
myst:
  html_meta:
    description: "How Juju watchers work: a watcher delivers a signal, not data; one baseline event on creation, then one event per qualifying change."
---

(watchers)=
# Watchers

```{audience} juju-dev
```

A **watcher** is a controller API object that fires when something the
holder cares about changes in the controller database. Watchers are
how everything that acts on the model reacts to change -- agents,
client facades, controller workers -- and nothing polls: the thing
that acts on a record watches it (see the per-entity surfaces, for
example {ref}`application watchers <the-application-watchers>` and
{ref}`machine watchers <machine-watchers>`).

## Signals, not data

The key property: **a watcher delivers a signal, not data.** The
change stream records only that something changed -- not what it
changed to -- because by the time the notification arrives the change
may already be stale (further changes may have happened). Rather than
propagate potentially stale data, the system propagates only the
notification; the consumer always fetches the current authoritative
state. The change-stream worker's own documentation states it:

```
This information all amounts to a notification that something has
happened. The reason no specifics about what exactly has happened are
included is because, as in every eventually consistent system, that
information can easily get stale. To retrieve the latest information,
each subscriber must query the database when they receive the
notification.
```

(`internal/changestream/stream/doc.go`)

Database triggers feed the change stream; the change stream wakes the
watcher; the consumer fetches the current state and reconciles.

## The two kinds

- **NotifyWatcher** fires an empty `struct{}`: pure signal. "Something
  changed. Go find out what."
- **StringsWatcher** fires a slice of the *keys* (IDs) of the records
  that changed: still no data, just enough to know which records to
  re-read.

## The baseline guarantee

Every watcher is required to fire once immediately on creation,
delivering the current state of the watched records before any
deltas. This "at-least-one notification" guarantee is the startup
sync: a freshly started agent needs no initialisation path distinct
from its reconcile loop -- it subscribes, receives its initial event,
and reconciles from there. After the baseline the watcher fires again
on each qualifying change.

## Who holds watchers

The consumers are the {ref}`agents <agent>` and the workers inside
them: an agent is a {ref}`tree of workers <worker>`, each concern
holding the watchers for its domain and reconciling when they fire.
The per-entity pages list the surfaces -- what a watcher fires on,
not who consumes it -- under each entity's "watchers" section, and
the worker loop that consumes them is described in
{ref}`worker <worker>`.
