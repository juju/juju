run_stream() {
    VERSION=$(jujud version)
    JUJUD_VERSION=$(echo "${VERSION}" | cut -d '-' -f 1)
    echo "===> Using jujud version ${JUJUD_VERSION}"

    add_clean_func "remove_tools"
    add_tools "${VERSION}"
  
    add_clean_func "kill_server"
    start_server "${VERSION}" "${JUJUD_VERSION}"

    sleep 5

    ip_address=$(cat "${TEST_DIR}/server.log" | head -n 1)
    if [ -z "${ip_address}" ]; then
        echo "IP Address not found"
        exit 1
    fi

    name="test-bootstrap-stream"

    file="${TEST_DIR}/test-bootstrap-stream.log"
    juju bootstrap "lxd" "${name}" \
        --config agent-metadata-url="http://${ip_address}:8081/" \
        --config test-mode=true \
        --agent-version="${JUJUD_VERSION}" 2>&1 | OUTPUT "${file}"
    echo "${name}" >> "${TEST_DIR}/jujus"

    juju deploy ./tests/suites/bootstrap/charms/ubuntu

    wait_for "ubuntu" "$(idle_condition "ubuntu")"
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
    local version jujud_path

    version=${1}

    jujud_path=$(which jujud)
    cp "${jujud_path}" "${TEST_DIR}"
    cd "${TEST_DIR}" || exit

    tar -zcvf "${version}".tar.gz jujud >/dev/null
    cd "${CURRENT_DIR}/.." || exit

    mv "${TEST_DIR}"/"${version}".tar.gz ./tests/suites/bootstrap/streams/tools/agent/
}

remove_tools() {
    cd "${CURRENT_DIR}/.." || exit

    echo "==> Removing tools"
    rm -f ./tests/suites/bootstrap/streams/tools/agent/*.tar.gz || true
    echo "==> Removed tools"
}

start_server() {
    local version jujud_version

    version=${1}
    jujud_version=${2}

    # We need to build here, if we use `go run` then we end up with the PID of
    # `go run`, but not of the server itself, which is not what we want.
    go build -o "${TEST_DIR}"/server ./tests/streams/server/main.go

    "${TEST_DIR}"/server \
        --release="released,focal-20.04-amd64,${jujud_version},agent/${version}.tar.gz" \
        --release="released,bionic-18.04-amd64,${jujud_version},agent/${version}.tar.gz" \
        ./tests/suites/bootstrap/streams/ >"${TEST_DIR}/server.log" 2>&1 &
    SERVER_PID=$!

    echo "${SERVER_PID}"

    echo "${SERVER_PID}" > "${TEST_DIR}/server.pid"
}

kill_server() {
    if [ ! -f "${TEST_DIR}/server.pid" ]; then
      return
    fi

    pid=$(cat "${TEST_DIR}/server.pid" | head -n 1 || echo "NOT FOUND")
    if [ "${pid}" == "NOT FOUND" ]; then
        return
    fi

    echo "==> Killing server"
    kill -9 "${pid}" >/dev/null 2>&1 || true
    echo "==> Killed server (PID is $(green "${pid}"))"
}
