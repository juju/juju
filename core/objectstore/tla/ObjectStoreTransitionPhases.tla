---- MODULE ObjectStoreTransitionPhases ----
\* Copyright 2026 Canonical Ltd.
\* Licensed under the AGPLv3, see LICENCE file for details.

EXTENDS Naturals

\* Model of core/objectstore/phase.go transition behaviour.
Phases == {"unknown", "draining", "error", "completed"}

InitialPhase == "unknown"

\* Transition relation matching TransitionTo in phase.go.
AllowedNext ==
    [p \in Phases |->
        CASE p = "unknown" -> {"draining"}
            [] p = "draining" -> {"error", "completed"}
            [] OTHER -> {}
    ]

CanTransition(from, to) == to \in AllowedNext[from]

TerminalPhases == {p \in Phases : AllowedNext[p] = {}}

NotStartedPhases == {"unknown"}

DrainingPhases == {"draining"}

\* Longest valid path from InitialPhase has 2 transitions.
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

Spec ==
    /\ Init
    /\ [][Next]_vars
    /\ WF_vars(Advance)

TypeInvariant ==
    /\ phase \in Phases
    /\ steps \in Nat

ASSUME TerminalPhasesMatchGo ==
    TerminalPhases = {"error", "completed"}

ASSUME NotStartedPhasesMatchGo ==
    NotStartedPhases = {"unknown"}

ASSUME DrainingPhasesMatchGo ==
    DrainingPhases = {"draining"}

ASSUME AllowedTransitionsMatchGo ==
    /\ AllowedNext["unknown"] = {"draining"}
    /\ AllowedNext["draining"] = {"error", "completed"}
    /\ AllowedNext["error"] = {}
    /\ AllowedNext["completed"] = {}

StepBoundInvariant == steps <= MaxTransitions

TransitionAction ==
    (phase' = phase) \/ CanTransition(phase, phase')

TransitionSafety == [][TransitionAction]_vars

EventuallyTerminal == <> (phase \in TerminalPhases)

=============================================================================
