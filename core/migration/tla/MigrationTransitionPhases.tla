---- MODULE MigrationTransitionPhases ----
\* Copyright 2026 Canonical Ltd.
\* Licensed under the AGPLv3, see LICENCE file for details.

EXTENDS Naturals

\* Model of core/migration/phase.go validTransitions.
\*
\* UNKNOWN and NONE are code-only sentinels ("uninitialised field" and "no
\* migration has ever been attempted"). They are terminal per IsTerminal() and
\* are included here so the Go drift test can compare the spec against the full
\* phase enum verbatim, but the lifecycle begins at QUIESCE so they are
\* unreachable in any behaviour.
Phases == {"UNKNOWN", "NONE", "QUIESCE", "IMPORT",
           "VALIDATION", "SUCCESS", "LOGTRANSFER", "REAP",
           "REAPFAILED", "DONE", "ABORT", "ABORTDONE"}

InitialPhase == "QUIESCE"

\* Transition relation matching validTransitions in phase.go.
AllowedNext ==
    [p \in Phases |->
        CASE p = "QUIESCE"           -> {"IMPORT", "ABORT"}
          [] p = "IMPORT"            -> {"VALIDATION", "ABORT"}
          [] p = "VALIDATION"        -> {"SUCCESS", "ABORT"}
          [] p = "SUCCESS"           -> {"LOGTRANSFER"}
          [] p = "LOGTRANSFER"       -> {"REAP"}
          [] p = "REAP"              -> {"DONE", "REAPFAILED"}
          [] p = "ABORT"             -> {"ABORTDONE"}
          [] OTHER -> {}
    ]

CanTransition(from, to) == to \in AllowedNext[from]

TerminalPhases == {p \in Phases : AllowedNext[p] = {}}

\* IsRunning() in phase.go.
RunningPhases == {"QUIESCE", "IMPORT", "VALIDATION", "SUCCESS"}

\* IsPostSuccess() in phase.go.
PostSuccessPhases == {"LOGTRANSFER", "REAP", "REAPFAILED", "DONE"}

\* Longest valid path from InitialPhase has 6 transitions:
\* QUIESCE -> IMPORT -> VALIDATION -> SUCCESS -> LOGTRANSFER -> REAP -> DONE
MaxTransitions == 6

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
    RunningPhases = {"QUIESCE", "IMPORT", "VALIDATION", "SUCCESS"}

ASSUME PostSuccessPhasesMatchGo ==
    PostSuccessPhases = {"LOGTRANSFER", "REAP", "REAPFAILED", "DONE"}

ASSUME AllowedTransitionsMatchGo ==
    /\ AllowedNext["QUIESCE"] = {"IMPORT", "ABORT"}
    /\ AllowedNext["IMPORT"] = {"VALIDATION", "ABORT"}
    /\ AllowedNext["VALIDATION"] = {"SUCCESS", "ABORT"}
    /\ AllowedNext["SUCCESS"] = {"LOGTRANSFER"}
    /\ AllowedNext["LOGTRANSFER"] = {"REAP"}
    /\ AllowedNext["REAP"] = {"DONE", "REAPFAILED"}
    /\ AllowedNext["ABORT"] = {"ABORTDONE"}
    /\ AllowedNext["DONE"] = {}
    /\ AllowedNext["REAPFAILED"] = {}
    /\ AllowedNext["ABORTDONE"] = {}
    /\ AllowedNext["UNKNOWN"] = {}
    /\ AllowedNext["NONE"] = {}

StepBoundInvariant == steps <= MaxTransitions

TransitionAction ==
    (phase' = phase) \/ CanTransition(phase, phase')

TransitionSafety == [][TransitionAction]_vars

\* The property this suite exists to prove: once the migration reaches a terminal
\* phase it never leaves it (terminal nodes are absorbing).
TerminalStability == [][ (phase \in TerminalPhases) => (phase' = phase) ]_vars

EventuallyTerminal == <> (phase \in TerminalPhases)

=============================================================================
