run_stream() {
    VERSION=$(jujud version)
    JUJUD_VERSION=$(echo "${VERSION}" | cut -d '-' -f 1)
    echo "===> Using jujud version ${JUJUD_VERSION}"

    FORCE_VERSION=$(force_version "${VERSION}")
    FORCE_JUJUD_VERSION=$(echo "${FORCE_VERSION}" | cut -d '-' -f 1)
    echo "===> Force version ${FORCE_JUJUD_VERSION}"

    #add_clean_func "remove_tools"
    add_tools "${VERSION}"
    add_force_tools "${FORCE_VERSION}"
  
    add_clean_func "kill_server"
    start_server "${VERSION}" "${JUJUD_VERSION}" "${FORCE_VERSION}" "${FORCE_JUJUD_VERSION}"

    sleep 50000

    ip_address=$(cat "${TEST_DIR}/server.log" | head -n 1)
    if [ -z "${ip_address}" ]; then
        echo "IP Address not found"
        exit 1
    fi

    name="test-upgrade"

    file="${TEST_DIR}/test-upgrade.log"
    juju bootstrap "lxd" "${name}" \
        --config agent-metadata-url="http://${ip_address}:8081/" \
        --config test-mode=true \
        --agent-version="${JUJUD_VERSION}" 2>&1 | OUTPUT "${file}"
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

force_version() {
    VERSION=${1}

    JUJUD_VERSION=$(echo "${VERSION}" | cut -d '-' -f 1)
    SERIES=$(echo "${VERSION}" | cut -d '-' -f 2)
    ARCH=$(echo "${VERSION}" | cut -d '-' -f 3)

    MAJOR=$(echo "${JUJUD_VERSION}" | cut -d '.' -f 1)
    MINOR=$(echo "${JUJUD_VERSION}" | cut -d '.' -f 2)
    PATCH=$(echo "${JUJUD_VERSION}" | cut -d '.' -f 3)

    echo "${MAJOR}.${MINOR}.$((${PATCH}+1))-${SERIES}-${ARCH}"
}

add_tools() {
    VERSION=${1}

    JUJUD_PATH=$(which jujud)
    cp "${JUJUD_PATH}" "${TEST_DIR}"
    cd "${TEST_DIR}" || exit

    tar -zcvf "${VERSION}".tar.gz jujud >/dev/null
    cd "${CURRENT_DIR}/.." || exit

    mv "${TEST_DIR}"/"${VERSION}".tar.gz ./tests/suites/upgrade/streams/tools/agent/
}

add_force_tools() {
    VERSION=${1}

    JUJUD_VERSION=$(echo "${VERSION}" | cut -d '-' -f 1)

    JUJUD_PATH=$(which jujud)
    cp "${JUJUD_PATH}" "${TEST_DIR}"
    cd "${TEST_DIR}" || exit

    echo "${JUJUD_VERSION}" > "FORCE_VERSION"

    tar -zcvf "${VERSION}".tar.gz jujud FORCE_VERSION >/dev/null
    cd "${CURRENT_DIR}/.." || exit

    mv "${TEST_DIR}"/"${VERSION}".tar.gz ./tests/suites/upgrade/streams/tools/agent/
}

remove_tools() {
    cd "${CURRENT_DIR}/.." || exit

    echo "==> Removing tools"
    rm -f ./tests/suites/upgrade/streams/tools/agent/*.tar.gz || true
    echo "==> Removed tools"
}

start_server() {
    VERSION=${1}
    JUJUD_VERSION=${2}
    FORCE_VERSION=${3}
    FORCE_JUJUD_VERSION=${4}

    go run ./tests/suites/upgrade/server/main.go \
        --release="released,focal-20.04-amd64,${JUJUD_VERSION},agent/${VERSION}.tar.gz" \
        --release="released,bionic-18.04-amd64,${JUJUD_VERSION},agent/${VERSION}.tar.gz" \
        --release="released,focal-20.04-amd64,${FORCE_JUJUD_VERSION},agent/${FORCE_VERSION}.tar.gz" \
        --release="released,bionic-18.04-amd64,${FORCE_JUJUD_VERSION},agent/${FORCE_VERSION}.tar.gz" \
        ./tests/suites/upgrade/streams/ >"${TEST_DIR}/server.log" 2>&1 &
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
