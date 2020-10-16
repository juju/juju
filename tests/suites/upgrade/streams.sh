run_stream() {
    VERSION=$(jujud version)

    add_clean_func "remove_tools"
    add_tools "${VERSION}"

    add_clean_func "remove_templates"
    add_templates "${VERSION}"

    add_clean_func "kill_server"
    start_server

    sleep 5

    ip_address=$(cat "${TEST_DIR}/server.log")
    if [ -z "${ip_address}" ]; then
        echo "IP Address not found"
        exit 1
    fi

    name="test-upgrade"

    file="${TEST_DIR}/test-upgrade.log"
    juju bootstrap "lxd" "${name}" --config agent-metadata-url="http://${ip_address}:8081/tools/" --config test-mode=true --agent-version=2.8.6 2>&1 | OUTPUT "${file}"
    echo "${name}" >> "${TEST_DIR}/jujus"
}

test_stream() {
    if [ -n "$(skip 'test_stream')" ]; then
        echo "==> SKIP: Asked to skip stream tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_stream"
    )
}

add_tools() {
    VERSION=${1}

    JUJUD_PATH=$(which jujud)
    cp "${JUJUD_PATH}" "${TEST_DIR}"
    cd "${TEST_DIR}" || exit

    tar -zcvf "${VERSION}".tar.gz jujud
    cd "${CURRENT_DIR}/.." || exit

    mv "${TEST_DIR}"/"${VERSION}".tar.gz ./tests/suites/upgrade/streams/tools/agent/
}

remove_tools() {
    cd "${CURRENT_DIR}/.." || exit

    echo "==> Removing tools"
    rm -f ./tests/suites/upgrade/streams/tools/agent/*.tar.gz || true
    echo "==> Removed tools"
}

add_templates() {
    VERSION=${1}

    go run ./tests/suites/upgrade/template/main.go \
        ./tests/suites/upgrade/streams/ \
        "${VERSION}"
}

remove_templates() {
    cd "${CURRENT_DIR}/.." || exit

    echo "==> Removing templates"
    rm -f ./tests/suites/upgrade/streams/tools/streams/v1/*.json || true
    echo "==> Removed templates"
}

start_server() {
    go build  -o "${TEST_DIR}/server" ./tests/suites/upgrade/server/main.go
    ("${TEST_DIR}/server" ./tests/suites/upgrade/streams/ >"${TEST_DIR}/server.log" 2>&1) &
    SERVER_PID=$!

    echo "${SERVER_PID}" > "${TEST_DIR}/server.pid"
}

kill_server() {
    if [ ! -f "${TEST_DIR}/server.pid" ]; then
      return
    fi

    pid=$(cat "${TEST_DIR}/server.pid" | head || echo "NOT FOUND")
    if [ "${pid}" == "NOT FOUND" ]; then
        return
    fi

    echo "==> Killing server"
    kill -9 "${pid}" >/dev/null 2>&1 || true
    echo "==> Killed server (PID is $(green "${pid}"))"
}
