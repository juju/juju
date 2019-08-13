bootstrap() {
    local provider name

    provider=${1}
    shift

    name=${1}
    shift

    output=${1}
    shift

    OUT=$(juju controllers --format=json | jq '.controllers | keys' | grep "${name}" || true)
    if [ -n "${OUT}" ]; then
        echo "${name} already exists. Use the following to clean up the environment:"
        echo "    juju destroy-controller --destroy-all-models -y ${name}"
        exit 1
    fi

    echo "====> Bootstrapping juju"
    if [ -n "${output}" ]; then
        juju bootstrap "${provider}" "${name}" "$@" > "${output}" 2>&1
    else
        juju bootstrap "${provider}" "${name}" "$@"
    fi
    echo "${name}" >> "${TEST_DIR}/jujus"

    echo "====> Bootstrapped juju"
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
            destroy "${juju_name}"
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
