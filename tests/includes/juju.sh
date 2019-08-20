bootstrap() {
    set -eux
    local provider name model

    case "${BOOTSTRAP_PROVIDER:-}" in
        "aws")
            provider="aws"
            ;;
        *)
            echo "Expected bootstrap provider, falling back to lxd."
            provider="lxd"
    esac

    model=${1}
    shift

    output=${1}
    shift

    rnd=$(cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 8 | head -n 1)
    name=$(echo "ctrl-${rnd}")

    OUT=$(juju models --format=json 2>/dev/null | jq '.models[] | .["short-name"]' | grep "${model}" || true)
    if [ -n "${OUT}" ]; then
        echo "${model} already exists. Use the following to clean up the environment:"
        echo "    juju destroy-model --force -y ${model}"
        exit 1
    fi

    echo "====> Bootstrapping juju"
    if [ -n "${output}" ]; then
        juju bootstrap "${provider}" "${name}" -d "${model}" "$@" > "${output}" 2>&1
    else
        juju bootstrap "${provider}" "${name}" -d "${model}" "$@"
    fi
    echo "${name}" >> "${TEST_DIR}/jujus"
    echo "${model}" >> "${TEST_DIR}/models"

    echo "====> Bootstrapped juju"

    export BOOTSTRAPPED_JUJU_CTRL_NAME="${name}"
}

destroy() {
    local name

    name=${1}
    shift

    # shellcheck disable=SC2034
    OUT=$(juju controllers --format=json | jq '.controllers | keys' | grep "${name}" || true)
    # shellcheck disable=SC2181
    if [ -z "${OUT}" ]; then
        return
    fi

    file="${TEST_DIR}/${name}_destroy.txt"

    echo "====> Destroying juju model ${name}"
    echo "${name}" | xargs -I % juju destroy-model --force -y % > "${file}" 2>&1
    CHK=$(cat "${file}" | grep -i "ERROR" || true)
    if [ -n "${CHK}" ]; then
        printf "\\nFound some issues"
        cat "${file}" | xargs echo -I % "\\n%"
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

    file="${TEST_DIR}/${name}_destroy_controller.txt"

    echo "====> Destroying juju ${name}"
    echo "${name}" | xargs -I % juju destroy-controller --destroy-all-models -y % > "${file}" 2>&1
    CHK=$(cat "${file}" | grep -i "ERROR" || true)
    if [ -n "${CHK}" ]; then
        printf "\\nFound some issues"
        cat "${file}" | xargs echo -I % "\\n%"
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
    fi
}

wait_for() {
    local name query

    name=${1}
    query=${2}

    until [ $(juju status --format=json 2> /dev/null | jq "${query}" | grep "${name}") ]; do
        juju status --relations
        sleep 5
    done
}
