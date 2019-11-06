# wait_for defines the ability to wait for a given condition to happen in a
# juju status output. The output is JSON, so everything that the API server
# knows about should be valid.
# The query argument is a jq query.
#
# ```
# wait_for <model name> <query>
# ```
wait_for() {
    local name query

    name=${1}
    query=${2}

    # shellcheck disable=SC2143
    until [ "$(juju status --format=json 2> /dev/null | jq "${query}" | grep "${name}")" ]; do
        juju status --relations
        sleep 5
    done
}

idle_condition() {
    local name app_index unit_index

    name=${1}
    app_index=${2:-0}
    unit_index=${3:-0}

    echo ".applications | select(.[\"$name\"] | .units | .[\"$name/$unit_index\"] | .[\"juju-status\"] | .current == \"idle\") | keys[$app_index]"
}

# workload_status gets the workload-status object for the unit - use
# .current or .message to select the actual field you need.
workload_status() {
    local app unit

    app=$1
    unit=$2

    echo ".applications[\"$app\"].units[\"$app/$unit\"][\"workload-status\"]"
}

# agent_status gets the juju-status object for the unit - use
# .current or .message to select the actual field you need.
agent_status() {
    local app unit

    app=$1
    unit=$2

    echo ".applications[\"$app\"].units[\"$app/$unit\"][\"juju-status\"]"
}
