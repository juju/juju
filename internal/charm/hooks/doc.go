// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package hooks provides types and constants that define the hooks known to Juju.

// A unit's direct action is entirely defined by its charm's hooks. Hooks
// are executable files in a charm's hooks directory; hooks with particular names
// will be invoked by the Juju unit agent at particular times, and thereby cause
// changes to the world.

// Whenever a hook-worthy event takes place, the unit agent tries to run the hook
// with the appropriate name. If the hook doesn't exist, the agent continues
// without complaint; if it does, it is invoked without arguments in a specific
// environment, and its output is written to the unit agent's log. If it returns
// a non-zero exit code, the agent enters an error state and awaits resolution;
// otherwise it continues to process model changes as before.

// A charm's reactions to hooks should ideally be idempotent. Charm authors don't
// have complete control over the times your hook might be stopped: if the unit agent
// process is killed  for any reason while running a hook, then when it recovers it
// will treat that hook as  having failed -- just as if it had returned a non-zero
// exit code -- and request user  intervention.

// It is unrealistic to expect great sophistication on the part of the average user,
// and as a charm author you should expect that users will attempt to re-execute
// failed hooks before attempting to investigate or understand the situation. You
// should therefore make every effort to ensure your hooks are idempotent when
// aborted and restarted.

// The most sophisticated charms will consider the nature of their operations with
// care, and will be prepared to internally retry any operations they suspect of
// having failed transiently, to ensure that they only request user intervention in
// the most trying circumstances; and will also be careful to log any relevant
// information or advice before signalling the error.

// Every hook is run in the deployed charm directory, in an environment with the
// following characteristics(more details can be found at internal/worker/uniter/runner/context/context.go):
//   * $PATH is prefixed by a directory containing command line tools through
//     which the hooks can interact with juju.
//   * $CHARM_DIR holds the path to the charm directory.
//   * $JUJU_UNIT_NAME holds the name of the local unit.
//   * $JUJU_CONTEXT_ID, $JUJU_AGENT_SOCKET_NETWORK and $JUJU_AGENT_SOCKET_ADDRESS
//     are set (but should not be messed with: the command line tools won't work without them).
//   * $JUJU_API_ADDRESSES holds a space separated list of juju API addresses.
//   * $JUJU_MODEL_NAME holds the human friendly name of the current model.
//   * $JUJU_PRINCIPAL_UNIT holds the name of the principal unit if the current unit is a subordinate.

// All hooks can directly use the jujuc hook tools. Within the context of a single
// hook execution, the hook tools present a sandboxed view of the system with the
// following properties:
//   * Any data retrieved corresponds to the real value of the underlying state at
//     some point in time.
//   * Once state data has been observed within a given hook execution, further
//     requests for the same data will produce the same results, unless that data
//     has been explicitly changed with relation-set.
//   * Data changed by relation-set is only written to global state when the hook
//     completes without error; changes made by a failing hook will be discarded
//     and never observed by any other part of the system.

// It should be noted that, while all hook tools are available to all hooks, the
// relation-* tools are not useful to the install, start, and stop hooks; this is
// because the first two are run before the unit has any opportunity to participate
// in any relations, and the stop hooks will not be run while the unit is still
// participating in one.

package hooks
