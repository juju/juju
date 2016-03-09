// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package legacy contains state-watcher-tuned worker harnesses; the canonical
implementations are in the watcher package, but aren't type-compatible with
original-style watchers -- such as those returned from state methods -- which
we still have a couple of uses for (and the certupdater use might even be
legitimate).
*/
package legacy
