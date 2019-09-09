ensure() {
    local model output

    model=${1}
    shift

    output=${1}
    shift

    export BOOTSTRAP_REUSE="true"
    bootstrap "${model}" "${output}"
}

bootstrap() {
    local provider name output model bootstrapped_name

    case "${BOOTSTRAP_PROVIDER:-}" in
        "aws")
            provider="aws"
            ;;
        "lxd")
            provider="lxd"
            ;;
        *)
            echo "Unexpected bootstrap provider."
            exit 1
    esac

    model=${1}
    shift

    output=${1}
    shift

    rnd=$(cat /dev/urandom | tr -dc 'a-z0-9' | fold -w 8 | head -n 1)
    name="ctrl-${rnd}"

    if [ ! -f "${TEST_DIR}/jujus" ]; then
        touch "${TEST_DIR}/jujus"
    fi
    bootstrapped_name=$(grep "." "${TEST_DIR}/jujus" | tail -n 1)
    if [ -z "${bootstrapped_name}" ]; then
        # No bootstrapped juju found, unset the the variable.
        unset BOOTSTRAP_REUSE
    fi

    if [ -n "${BOOTSTRAP_REUSE}" ]; then
        echo "====> Reusing bootstrapped juju"

        OUT=$(juju models --format=json 2>/dev/null | jq '.models[] | .["short-name"]' | grep "${model}" || true)
        if [ -n "${OUT}" ]; then
            echo "${model} already exists. Use the following to clean up the environment:"
            echo "    juju destroy-model --force -y ${model}"
            exit 1
        fi

        add_model "${model}" "${provider}"
        name="${bootstrapped_name}"
    else
        echo "====> Bootstrapping juju"
        juju_bootstrap "${provider}" "${name}" "${model}" "${output}"
    fi

    echo "====> Bootstrapped juju"

    export BOOTSTRAPPED_JUJU_CTRL_NAME="${name}"
}

add_model() {
    local model provider

    model=${1}
    provider=${2}

    OUT=$(juju controllers --format=json | jq '.controllers | .["${bootstrapped_name}"] | .cloud' | grep "${provider}" || true)
    if [ -n "${OUT}" ]; then
        juju add-model "${model}" "${provider}"
    else
        juju add-model "${model}"
    fi
    echo "${model}" >> "${TEST_DIR}/models"
}

juju_bootstrap() {
    local provider name model output

    provider=${1}
    shift

    name=${1}
    shift

    model=${1}
    shift

    output=${1}
    shift

    if [ -n "${output}" ]; then
        juju bootstrap "${provider}" "${name}" -d "${model}" "$@" 2>&1 | add_date >"${output}"
    else
        juju bootstrap "${provider}" "${name}" -d "${model}" "$@"
    fi
    echo "${name}" >> "${TEST_DIR}/jujus"
}

destroy_model() {
    local name

    name=${1}
    shift

    # shellcheck disable=SC2034
    OUT=$(juju models --format=json | jq '.models | .[] | .["short-name"]' | grep "${name}" || true)
    # shellcheck disable=SC2181
    if [ -z "${OUT}" ]; then
        return
    fi

    output="${TEST_DIR}/${name}-destroy.txt"

    echo "====> Destroying juju model ${name}"
    echo "${name}" | xargs -I % juju destroy-model --force -y % 2>&1 | add_date >"${output}"
    CHK=$(cat "${output}" | grep -i "ERROR" || true)
    if [ -n "${CHK}" ]; then
        printf "\\nFound some issues"
        cat "${output}" | xargs echo -I % "\\n%"
        exit 1
    fi
    echo "====> Destroyed juju model ${name}"
}

destroy_controller() {
    local name

    name=${1}
    shift

    # shellcheck disable=SC2034
    OUT=$(juju controllers --format=json | jq '.controllers | keys' | grep "${name}" || true)
    # shellcheck disable=SC2181
    if [ -z "${OUT}" ]; then
        return
    fi

    output="${TEST_DIR}/${name}-destroy-controller.txt"

    echo "====> Destroying juju ${name}"
    echo "${name}" | xargs -I % juju destroy-controller --destroy-all-models -y % 2>&1 | add_date >"${output}"
    CHK=$(cat "${output}" | grep -i "ERROR" || true)
    if [ -n "${CHK}" ]; then
        printf "\\nFound some issues"
        cat "${output}" | xargs echo -I % "\\n%"
        exit 1
    fi
    echo "====> Destroyed juju ${name}"
}

cleanup_jujus() {
    if [ -f "${TEST_DIR}/jujus" ]; then
        echo "====> Cleaning up jujus"

        while read -r juju_name; do
            destroy_controller "${juju_name}"
        done < "${TEST_DIR}/jujus"
        rm -f "${TEST_DIR}/jujus"
    fi
}

wait_for() {
    local name query

    name=${1}
    query=${2}

    # shellcheck disable=SC2046,SC2143
    until [ $(juju status --format=json 2> /dev/null | jq "${query}" | grep "${name}") ]; do
        juju status --relations
        sleep 5
    done
}
