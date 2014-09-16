#!/bin/bash
# juju-core.bash_completion.sh: dynamic bash completion for juju cmdline,
# from parsed (and cached) juju status output.
#
# Author: JuanJo Ciarlante <jjo@canonical.com>
# Copyright 2013+, Canonical Ltd.
# License: GPLv3
#

# Print (return) all machines
_juju_machines_from_file() {
python -c '
import json, sys; j=json.load(sys.stdin)
print "\n".join(j["machines"].keys());' < ${1?}
}

# Print (return) all units, each optionally postfixed by $2 (eg. 'myservice/0:')
_juju_units_from_file() {
python -c '
trail="'${2}'"
import json, sys; j=json.load(sys.stdin)
all_units=[]
for k,v in j["services"].items():
    if v.get("units"):
        all_units.extend(v.get("units",{}).keys())
print "\n".join([unit + trail for unit in all_units])
' < ${1?}
}

# Print (return) all services
_juju_services_from_file() {
python -c '
import json, sys; j=json.load(sys.stdin)
print "\n".join(j["services"].keys());' < ${1?}
}

# Print (return) both services and units, currently used for juju status completion
_juju_services_and_units_from_file() {
    _juju_services_from_file "$@"
    _juju_units_from_file "$@"
}

# Print (return) all juju commands
_juju_list_commands() {
    juju help commands 2>/dev/null | awk '{print $1}'
}

# Print (return) flags for juju action, shamelessly excluding
# -e/--environment for cleaner completion for common usage cases
# (e.g. juju ssh <TAB>, etc)
_juju_flags_for() {
    test -z "${1}" && return 0
    juju help ${1} 2>/dev/null |egrep -o --  '(^|-)-[a-z-]+'|egrep -v -- '^(-e|--environment)'|sort -u
}

# Print (return) guessed completion function for cmd.
# Guessing is done by parsing 1st line of juju help <cmd>,
# see case switch below.
_juju_completion_func_for_cmd() {
    local action=${1} cword=${2}
    # if cword==1 or action==help, use _juju_list_commands
    if [ "${cword}" -eq 1 -o "${action}" = help ]; then
        echo _juju_list_commands
        return 0
    fi
    # parse 1st line of juju help <cmd>, to guess the completion function
    case $(juju help ${action} 2>/dev/null| head -1) in
        # special case for ssh, scp which have 'service' in 1st line of help:
        *\<unit*|*juju?ssh*|*juju?scp*)    echo _juju_units_from_file;;
        *\<service*)    echo _juju_services_from_file;;
        *\<machine*)    echo _juju_machines_from_file;;
        *pattern*)      echo _juju_services_and_units_from_file;; # e.g. status
        ?*)     echo true ;;  # help ok, existing command, no more expansion
        *)      echo false;;  # failed, not a command
    esac
}

# Print (return) filename from juju status cached output (if not expired),
# create cache dirs if needed
# - setups caching dir if non-existent
# - caches juju status output, $cache_mins minutes max
_juju_get_status_filename() {
    local cache_mins=60     # ttl=60 mins
    local cache_dir=$HOME/.cache/juju
    local juju_status_file=${cache_dir}/juju-status-${JUJU_ENV:-default}
    # setup caching dir under ~/.cache/juju
    test -d ${cache_dir} || install -d ${cache_dir} -m 700
    # if can't find a fresh (age < $cache_mins) saved file, with a ~reasonable size ...
    if [[ -z $(find "${juju_status_file}" -mmin -${cache_mins} -a -size +32c 2> /dev/null) ]]; then
        # ... create it
        juju status --format=json > "${juju_status_file}".tmp && \
            mv "${juju_status_file}".tmp "${juju_status_file}"
        rm -f "${juju_status_file}".tmp
    fi
    if [ -r "${juju_status_file}" ]; then
        echo "${juju_status_file}"
    else
        return 1
    fi
}
# Main completion function wrap:
# calls passed completion function, also adding flags for cmd
_juju_complete_with_func() {
    local action="${1}" func=${2?}
    local cur

    # scp is special, as we want ':' appended to unit names,
    # and filename completion also.
    local postfix_str= compgen_xtra=
    if [ "${action}" = "scp" ]; then
        local orig_comp_wordbreaks="${COMP_WORDBREAKS}"
        COMP_WORDBREAKS="${COMP_WORDBREAKS/:/}"
        postfix_str=':'
        compgen_xtra='-A file'
        compopt -o nospace
    fi
    juju_status_file=
    # if func name ends with 'from_file', set juju_status_file
    [[ ${func} =~ .*from_file ]] &&  juju_status_file=$(_juju_get_status_filename)
    # build COMPREPLY from passed function stdout, and _juju_flags_for $action
    cur="${COMP_WORDS[COMP_CWORD]}"
    COMPREPLY=( $( compgen ${compgen_xtra} -W "$(${func} ${juju_status_file} ${postfix_str}) $(_juju_flags_for "${action}")" -- ${cur} ))
    if [ "${action}" = "scp" ]; then
        COMP_WORDBREAKS="${orig_comp_wordbreaks}"
        compopt +o nospace
    fi
    return 0
}

# Not used here, available to the user for quick cache removal
_juju_completion_cache_rm() {
    rm -fv $HOME/.cache/juju/juju-status-${JUJU_ENV:-default}
}

# main completion function entry point
_juju() {
    local action parsing_func
    action="${COMP_WORDS[1]}"
    COMPREPLY=()
    parsing_func=$(_juju_completion_func_for_cmd "${action}" ${COMP_CWORD})
    test -z "${parsing_func}" && return 0
    _juju_complete_with_func "${action}" "${parsing_func}"
    return $?
}
complete -F _juju juju
# vim: ai et sw=2 ts=2
