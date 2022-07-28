// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package fortress implements a convenient metaphor for an RWLock.

A "fortress" is constructed via a manifold's Start func, and accessed via its
Output func as either a Guard or a Guest. To begin with, it's considered to be
locked, and inaccessible to Guests; when the Guard Unlocks it, the Guests can
Visit it until the Guard calls Lockdown. At that point, new Visits are blocked,
and existing Visits are allowed to complete; the Lockdown returns once all
Guests' Visits have completed.

The original motivating use case was for a component to mediate charm directory
access between the uniter and the metrics collector. The metrics collector must
be free to run its own independent hooks while the uniter is active; but metrics
hooks and charm upgrades cannot be allowed to tread on one another's toes.
*/
package fortress
