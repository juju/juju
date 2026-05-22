---- MODULE MigrationTransitionPhases ----
\* Copyright 2026 Canonical Ltd.
\* Licensed under the AGPLv3, see LICENCE file for details.

EXTENDS Naturals

\* Model of core/migration/phase.go transition behaviour.
\* Phase names intentionally mirror phaseNames in the Go package.
Phases ==
    {
        "UNKNOWN",
        "NONE",
        "QUIESCE",
        "IMPORT",
        "PROCESSRELATIONS",
        "VALIDATION",
        "SUCCESS",
        "LOGTRANSFER",
        "REAP",
        "REAPFAILED",
        "DONE",
        "ABORT",
        "ABORTDONE"
    }

InitialPhase == "QUIESCE"

\* Transition relation matching validTransitions from phase.go.
AllowedNext ==
    [p \in Phases |->
        CASE p = "QUIESCE" -> {"IMPORT", "ABORT"}
            [] p = "IMPORT" -> {"PROCESSRELATIONS", "ABORT"}
            [] p = "PROCESSRELATIONS" -> {"VALIDATION", "ABORT"}
            [] p = "VALIDATION" -> {"SUCCESS", "ABORT"}
            [] p = "SUCCESS" -> {"LOGTRANSFER"}
            [] p = "LOGTRANSFER" -> {"REAP"}
            [] p = "REAP" -> {"DONE", "REAPFAILED"}
            [] p = "ABORT" -> {"ABORTDONE"}
            [] OTHER -> {}
    ]

CanTransition(from, to) == to \in AllowedNext[from]

TerminalPhases == {p \in Phases : AllowedNext[p] = {}}

RunningPhases ==
    {"QUIESCE", "IMPORT", "PROCESSRELATIONS", "VALIDATION", "SUCCESS"}

PostSuccessPhases == {"LOGTRANSFER", "REAP", "REAPFAILED", "DONE"}

\* Longest valid successful path from InitialPhase has 7 transitions.
MaxTransitions == 7

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
    TerminalPhases = {"UNKNOWN", "NONE", "REAPFAILED", "DONE", "ABORTDONE"}

ASSUME RunningPhasesMatchGo ==
    RunningPhases =
        {"QUIESCE", "IMPORT", "PROCESSRELATIONS", "VALIDATION", "SUCCESS"}

ASSUME PostSuccessPhasesMatchGo ==
    /\ PostSuccessPhases = {"LOGTRANSFER", "REAP", "REAPFAILED", "DONE"}
    /\ PostSuccessPhases \cap {"ABORT", "ABORTDONE"} = {}

ASSUME SpecialPhasesMatchGo ==
    /\ AllowedNext["UNKNOWN"] = {}
    /\ AllowedNext["NONE"] = {}
    /\ \A p \in Phases:
        /\ ~CanTransition(p, "UNKNOWN")
        /\ ~CanTransition(p, "NONE")

StepBoundInvariant == steps <= MaxTransitions

TransitionAction ==
    (phase' = phase) \/ CanTransition(phase, phase')

TransitionSafety == [][TransitionAction]_vars

EventuallyTerminal == <> (phase \in TerminalPhases)

=============================================================================
