run_stream() {
    VERSION=$(jujud version)
    JUJUD_VERSION=$(echo "${VERSION}" | cut -d '-' -f 1)
    echo "===> Using jujud version ${JUJUD_VERSION}"

    FORCE_VERSION="$(force_version "${VERSION}")"
    FORCE_JUJUD_VERSION="$(echo "${FORCE_VERSION}" | cut -d '-' -f 1)"
    echo "===> Force version ${FORCE_JUJUD_VERSION}"

    add_clean_func "remove_tools"
    add_tools "${VERSION}"
    add_force_tools "${FORCE_VERSION}"
  
    add_clean_func "kill_server"
    start_server "${VERSION}" "${JUJUD_VERSION}" "${FORCE_VERSION}" "${FORCE_JUJUD_VERSION}"

    sleep 5

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

    juju upgrade-controller --agent-version="${FORCE_JUJUD_VERSION}" 2>&1 | OUTPUT "${file}"
    wait_for_controller_version "${FORCE_JUJUD_VERSION}"
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
    local version jujud_version series arch

    version=${1}

    jujud_version=$(echo "${version}" | cut -d '-' -f 1)
    series=$(echo "${version}" | cut -d '-' -f 2)
    arch=$(echo "${version}" | cut -d '-' -f 3)

    MAJOR=$(echo "${jujud_version}" | cut -d '.' -f 1)
    MINOR=$(echo "${jujud_version}" | cut -d '.' -f 2)
    PATCH=$(echo "${jujud_version}" | cut -d '.' -f 3)

    echo "${MAJOR}.${MINOR}.$((${PATCH}+1))-${series}-${arch}"
}

add_tools() {
    local version jujud_path

    version=${1}

    jujud_path=$(which jujud)
    cp "${jujud_path}" "${TEST_DIR}"
    cd "${TEST_DIR}" || exit

    tar -zcvf "${version}".tar.gz jujud >/dev/null
    cd "${CURRENT_DIR}/.." || exit

    mv "${TEST_DIR}"/"${version}".tar.gz ./tests/suites/upgrade/streams/tools/agent/
}

add_force_tools() {
    local version jujud_version jujud_path

    version=${1}

    jujud_version=$(echo "${version}" | cut -d '-' -f 1)

    jujud_path=$(which jujud)
    cp "${jujud_path}" "${TEST_DIR}"
    cd "${TEST_DIR}" || exit

    echo "${jujud_version}" > "FORCE_VERSION"

    tar -zcvf "${version}".tar.gz jujud FORCE_VERSION >/dev/null
    cd "${CURRENT_DIR}/.." || exit

    mv "${TEST_DIR}"/"${version}".tar.gz ./tests/suites/upgrade/streams/tools/agent/
}

remove_tools() {
    cd "${CURRENT_DIR}/.." || exit

    echo "==> Removing tools"
    rm -f ./tests/suites/upgrade/streams/tools/agent/*.tar.gz || true
    echo "==> Removed tools"
}

start_server() {
    local version jujud_version force_version force_jujud_version

    version=${1}
    jujud_version=${2}
    force_version=${3}
    force_jujud_version=${4}

    go run ./tests/suites/upgrade/server/main.go \
        --release="released,focal-20.04-amd64,${jujud_version},agent/${version}.tar.gz" \
        --release="released,bionic-18.04-amd64,${jujud_version},agent/${version}.tar.gz" \
        --release="released,focal-20.04-amd64,${force_jujud_version},agent/${force_version}.tar.gz" \
        --release="released,bionic-18.04-amd64,${force_jujud_version},agent/${force_version}.tar.gz" \
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

wait_for_controller_version() {
    local version

    version=${1}

    attempt=0
    # shellcheck disable=SC2046,SC2143
    until [ $(juju machines -m controller --format=json | jq -r '.machines | .["0"] | .["juju-status"] | .version' | grep "${version}") ]; do
        echo "[+] (attempt ${attempt}) polling machines"
        sleep "${SHORT_TIMEOUT}"
        attempt=$((attempt+1))
    done

    if [ "${attempt}" -gt 0 ]; then
        echo "[+] $(green 'Completed polling machines')"
        sleep "${SHORT_TIMEOUT}"
    fi
}
