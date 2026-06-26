---- MODULE ImportClaimPhases ----
\* Copyright 2026 Canonical Ltd.
\* Licensed under the AGPLv3, see LICENCE file for details.

\* Model of the target-side import-claim protocol. The claim is created at
\* "importing" (the first target-side write) and is destroyed ("deleted") once
\* activation or abort cleanup finalises. The key safety property is the
\* "activation point of no return": there is no activating -> aborting edge
\* (and, symmetrically, no aborting -> activating).

EXTENDS Naturals

\* "deleted" models the absence of the claim row (model UUID free again); it is
\* the terminal node of the protocol.
Phases == {"importing", "activating", "aborting", "deleted"}

InitialPhase == "importing"

AllowedNext ==
    [p \in Phases |->
        CASE p = "importing"  -> {"activating", "aborting"}
          [] p = "activating" -> {"deleted"}   \* never -> aborting (point of no return)
          [] p = "aborting"   -> {"deleted"}   \* never -> activating
          [] OTHER -> {}
    ]

CanTransition(from, to) == to \in AllowedNext[from]

TerminalPhases == {p \in Phases : AllowedNext[p] = {}}

\* Longest valid path from InitialPhase has 2 transitions:
\* importing -> activating -> deleted (and importing -> aborting -> deleted).
MaxTransitions == 2

VARIABLES phase, steps

vars == <<phase, steps>>

Init ==
    /\ phase = InitialPhase
    /\ steps = 0

Advance ==
    /\ phase \notin TerminalPhases
    /\ \E next \in AllowedNext[phase]:
        /\ phase' = next
        /\ steps' = steps + 1

StayTerminal ==
    /\ phase \in TerminalPhases
    /\ UNCHANGED vars

Next == Advance \/ StayTerminal

\* WF_vars(Advance) assumes a driver (master worker / reconciler / operator)
\* keeps acting. It deliberately abstracts away the crash-without-recovery of an
\* "importing" claim, whose recovery is an out-of-band operator/reconciler path
\* rather than a phase transition.
Spec ==
    /\ Init
    /\ [][Next]_vars
    /\ WF_vars(Advance)

TypeInvariant ==
    /\ phase \in Phases
    /\ steps \in Nat

StepBoundInvariant == steps <= MaxTransitions

ASSUME AllowedTransitionsMatchDesign ==
    /\ AllowedNext["importing"] = {"activating", "aborting"}
    /\ AllowedNext["activating"] = {"deleted"}
    /\ AllowedNext["aborting"] = {"deleted"}
    /\ AllowedNext["deleted"] = {}

TransitionAction ==
    (phase' = phase) \/ CanTransition(phase, phase')

TransitionSafety == [][TransitionAction]_vars

\* The activation point of no return: once activation has started, abort cleanup
\* can never take over.
ActivatingNeverAborts == [][ (phase = "activating") => (phase' # "aborting") ]_vars

\* Symmetrically, once abort cleanup has started, activation can never take over.
AbortingNeverActivates == [][ (phase = "aborting") => (phase' # "activating") ]_vars

\* Once the claim is deleted it stays deleted (terminal node is absorbing).
TerminalStability == [][ (phase \in TerminalPhases) => (phase' = phase) ]_vars

EventuallyTerminal == <> (phase \in TerminalPhases)

=============================================================================
