#!/bin/bash


{
    # this block disables xtrace for lines from this file,
    # while keeping it enabled for user-land reactive code
    shopt -s expand_aliases
    alias _suppress_xtrace='{ local old_opts="$-"; set $X_OPT; } 2> /dev/null'
    alias _restore_xtrace='_restore_opt $old_opts x'
    function _restore_opt() {
        # restore the given opt to its previous state
        local prev_opts="$1"
        local opt_to_restore="$2"
        if [[ "$prev_opts" == *$opt_to_restore* ]]; then
            # opt was previously enabled, to re-enable it
            set -$opt_to_restore
        else
            # opt was previously disabled, to re-disable it
            set +$opt_to_restore
        fi
    }
    if [[ "$CHARMS_REACTIVE_TRACE" != 'true' ]]; then
        X_OPT="+x"
    else
        # we are debugging charms.reactive.sh, so enable xtrace internally
        X_OPT="-x"
    fi
    # save options as of import
    import_opts="$-"
    set +x
} 2> /dev/null  # suppress xtrace during import (braces to hide entire block from xtracing)

export PATH=$PATH:$CHARM_DIR/bin
export PYTHONPATH=${PYTHONPATH-}:$CHARM_DIR/lib

if [[ "$0" == "$BASH_SOURCE" ]]; then
    echo 'This file contains helpers for developing reactive charms in Bash.'
    echo
    echo 'Usage:'
    echo
    echo '    source `which reactive.sh`'
    echo
    echo '    @when db.ready cache.ready'
    echo '    function write_configuration() {'
    echo '        db_dsn=$(relation_call --flag db.ready uri)'
    echo '        cache_uri=$(relation_call --flag cache.ready uri)'
    echo '        chlp render_template db_dsn=$db_dsn cache_uri=$cache_uri'
    echo '    }'
    echo
    echo '    @when_not db.ready cache.ready'
    echo '    function report_blocked() {'
    echo '        status-set blocked "Waiting for db and/or cache"'
    echo '    }'
    echo
    echo '    reactive_handler_main'
    echo
    echo 'Helpers available:'
    echo
    echo '  @hook PATTERN                Run decorated function if hook matches'
    echo '  @when STATES                 Run decorated function if states are active'
    echo '  @when_not STATES             Run decorated function if states not active'
    echo '  @when_file_changed FILENAME  Run decorated function if file(s) changed'
    echo '  @only_once                   Run decorated function only once'
    echo '  set_flag                     Set / activate a flag'
    echo '  clear_flag                   Remove a flag'
    echo '  relation_call                Call a method on a relation by flag or relation name'
    echo '  all_flags_set                Check all flags are active'
    echo '  any_flags_set                Check any flags are active'
    echo '  name_relation_get            Get a relation value by relation name (vs ID)'
    exit 0
fi

REACTIVE_ACTION="${1-'--test'}"
REACTIVE_ARGS="${2-}"

declare -A REACTIVE_HANDLERS
declare -A REACTIVE_TESTS

function _get_decorated() {
    # find the name of the "decorated" function, given
    # that there may be more decorators between us and it
    filename=${BASH_SOURCE[2]}
    lineno=$((BASH_LINENO[1]+1))
    last_decorator=yes
    while sed "${lineno}q;d" $filename | grep -qE "^\s*@"; do
        # there are other decorators between us and the function
        last_decorator=no
        ((lineno++))
    done
    func=$(sed "${lineno}q;d" $filename | sed "s/\(function\)\? *\(.*\) *() *{/\2/")
    if [[ "$old_opts" == *x* && $last_decorator == yes ]]; then
        # pseudo-xtrace function definition when xtrace is active, since
        # the @decorator line will be logged anyway (can't suppress it)
        >&2 echo "+ function $func() { ... }"
    fi
}

function @decorator() {
    _suppress_xtrace
    _get_decorated
    handler_id="$filename:$lineno:$func"
    # this "decorates" a function using source-code introspection, registering
    # the "decorated" function as a reactive handler with a set of preconditions

    # register handler_ids
    if [ ! ${REACTIVE_HANDLERS[$func]+_} ]; then
        REACTIVE_HANDLERS[$func]="$handler_id"
    fi

    # register tests
    test_name="$1"; shift
    test_args=$(printf '"%s" ' "$@")
    if [ ! ${REACTIVE_TESTS[$func]+_} ]; then
        REACTIVE_TESTS[$func]="'$func' '$handler_id' '$test_name' '$test_args'"
    else
        REACTIVE_TESTS[$func]="${REACTIVE_TESTS[$func]} '$test_name' '$test_args'"
    fi
    _restore_xtrace
}

function reactive_handler_main() {
    _suppress_xtrace
    if [[ "$REACTIVE_ACTION" == "--test" ]]; then
        >&2 echo "Running reactive_handler_main for $(basename $0) (test)"
        to_invoke=$(charms.reactive test "${REACTIVE_TESTS[@]}")
        if [[ -n "$to_invoke" ]]; then
            echo $to_invoke
            >&2 echo "Will invoke: $to_invoke"
            >&2 echo "End reactive_handler_main (test)"
            exit 0
        else
            >&2 echo "Nothing to invoke"
            >&2 echo "End reactive_handler_main (test)"
            exit 1
        fi
    elif [[ "$REACTIVE_ACTION" == "--invoke" ]]; then
        >&2 echo "Running reactive_handler_main for $(basename $0) (invoke)"
        invoked=()
        for handler in ${REACTIVE_ARGS//,/ }; do
            >&2 echo "Invoking bash reactive handler: $handler"
            eval "_restore_xtrace; $handler; _suppress_xtrace"
            invoked+=("${REACTIVE_HANDLERS[$handler]}")
        done
        charms.reactive mark_invoked "${invoked[@]}"
        >&2 echo "End reactive_handler_main (invoke)"
    fi
    _restore_xtrace
}

# some helpers and syntactic sugar
alias @hook='@decorator hook'
alias @when='@decorator when'
alias @when_all='@decorator when_all'
alias @when_any='@decorator when_any'
alias @when_not='@decorator when_not'
alias @when_none='@decorator when_none'
alias @when_not_all='@decorator when_not_all'
alias @when_file_changed='@decorator when_file_changed'
alias @only_once='@decorator only_once'
alias set_flag='charms.reactive set_flag'
alias clear_flag='charms.reactive clear_flag'
alias relation_call='charms.reactive relation_call'  # Call a method on a named relation
alias all_flags_set='charms.reactive all_flags_set'
alias any_flags_set='charms.reactive any_flags_set'

# DEPRECATED
alias set_state='charms.reactive set_flag'
alias remove_state='charms.reactive clear_flag'
alias all_states='charms.reactive all_flags_set'
alias any_states='charms.reactive any_flags_set'

# DEPRECATED: Use `relation_call --flag <flag> <method>` instead
function state_relation_call() {
    _suppress_xtrace
    # Call a method on the relation implementation associated with the
    # given active state.
    flag=$1
    method=$2
    shift; shift
    charms.reactive relation_call --flag $flag $method "$@"
    _restore_xtrace
}

function name_relation_get() {
    _suppress_xtrace
    # Helper function around relation-get that accepts a relation name in place
    # of a relation ID.
    #
    # Usage: name_relation_get <relation_name> <unit_name> [<key>] [--format=<format>]
    # Note: The --format option must come at the end, if present
    relation_name="$1"
    unit_name="$2"
    key="$3"
    if [[ -z "$key" || "$key" == --* ]]; then
        key="-"
        shift; shift
    else
        shift; shift; shift
    fi
    relid="$(charms.reactive relation_id "$relation_name" "$unit_name")"
    relation-get -r "$relid" "$key" "$unit_name" "$@"
    _restore_xtrace
}

# re-enable xtrace for user code, if it was enabled
_restore_opt $import_opts x
