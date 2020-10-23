run_simplestream_metadata() {
    VERSION=$(jujud version)
    JUJUD_VERSION=$(jujud_version)
    echo "===> Using jujud version ${JUJUD_VERSION}"

    add_clean_func "remove_bootstrap_tools"
    add_bootstrap_tools "${VERSION}"

    add_clean_func "remove_bootstrap_metadata"
    juju metadata generate-agents \
        --clean \
        --prevent-fallback \
        -d "./tests/suites/bootstrap/streams/"

    add_clean_func "kill_server"
    start_server "./tests/suites/bootstrap/streams/tools"

    ip_address=$(ip -4 -o addr show scope global | awk '{gsub(/\/.*/,"",$4); print $4}' | head -n 1)

    name="test-bootstrap-stream"

    file="${TEST_DIR}/test-bootstrap-stream.log"
    juju bootstrap "lxd" "${name}" \
        --config agent-metadata-url="http://${ip_address}:8000/" \
        --config test-mode=true \
        --agent-version="${JUJUD_VERSION}" 2>&1 | OUTPUT "${file}"
    echo "${name}" >> "${TEST_DIR}/jujus"

    juju deploy ./tests/suites/bootstrap/charms/ubuntu
    wait_for "ubuntu" "$(idle_condition "ubuntu")"
}

test_bootstrap_simplestream() {
    if [ -n "$(skip 'test_bootstrap_simplestream')" ]; then
        echo "==> SKIP: Asked to skip stream tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_simplestream_metadata"
    )
}

add_bootstrap_tools() {
    local version jujud_path

    version=${1}

    jujud_path=$(which jujud)
    cp "${jujud_path}" "${TEST_DIR}"
    cd "${TEST_DIR}" || exit

    tar -zcvf "juju-${version}.tgz" jujud >/dev/null
    cd "${CURRENT_DIR}/.." || exit

    mkdir -p "./tests/suites/bootstrap/streams/tools/released/"
    mv "${TEST_DIR}/juju-${version}.tgz" "./tests/suites/bootstrap/streams/tools/released"
}

remove_bootstrap_tools() {
    cd "${CURRENT_DIR}/.." || exit

    echo "==> Removing tools"
    rm -rf ./tests/suites/bootstrap/streams/tools/released || true
    echo "==> Removed tools"
}

remove_bootstrap_metadata() {
    cd "${CURRENT_DIR}/.." || exit

    echo "==> Removing metadata"
    rm -rf ./tests/suites/bootstrap/streams/tools/streams || true
    echo "==> Removed metadata"
}
