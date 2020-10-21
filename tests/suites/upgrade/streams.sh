run_simplestream_metadata() {
    VERSION=$(jujud version)
    JUJUD_VERSION=$(jujud_version)
    STABLE_VERSION=$(last_stable_version "${JUJUD_VERSION}")
    echo "===> Using jujud version ${JUJUD_VERSION}"

    FOCAL_VERSION=$(series_version "${VERSION}" "focal")
    BIONIC_VERSION=$(series_version "${VERSION}" "bionic")

    OUT=$(snap install juju --classic --channel="${STABLE_VERSION}/stable" || echo "FALLBACK")
    if [ "${OUT}" == "FALLBACK" ]; then
        snap refresh juju --channel="${STABLE_VERSION}/stable"
    fi

    add_clean_func "remove_upgrade_tools"
    add_upgrade_tools "${FOCAL_VERSION}"
    add_upgrade_tools "${BIONIC_VERSION}"

    add_clean_func "remove_upgrade_metadata"
    juju metadata generate-agents \
        --clean \
        --prevent-fallback \
        -d "./tests/suites/upgrade/streams/"

    add_clean_func "kill_server"
    start_server "./tests/suites/upgrade/streams/tools"

    ip_address=$(ip -4 -o addr show scope global | awk '{gsub(/\/.*/,"",$4); print $4}' | head -n 1)

    name="test-upgrade-stream"

    file="${TEST_DIR}/test-upgrade-stream.log"
    /snap/bin/juju bootstrap "lxd" "${name}" \
        --config agent-metadata-url="http://${ip_address}:8000/" \
        --config test-mode=true 2>&1 | OUTPUT "${file}"
    echo "${name}" >> "${TEST_DIR}/jujus"

    juju deploy ./tests/suites/upgrade/charms/ubuntu
    wait_for "ubuntu" "$(idle_condition "ubuntu")"

    CURRENT=$(juju machines -m controller --format=json | jq -r '.machines | .["0"] | .["juju-status"] | .version')
    echo "==> Current juju version ${CURRENT}"

    juju upgrade-controller --agent-version="${JUJUD_VERSION}"

    attempt=0
    while true; do
        UPDATED=$(juju machines -m controller --format=json | jq -r '.machines | .["0"] | .["juju-status"] | .version' || echo "${CURRENT}")
        if [ "$CURRENT" != "$UPDATED" ]; then
            break
        fi
        echo "[+] (attempt ${attempt}) polling machines"
        sleep 10
        attempt=$((attempt+1))
        if [ "$attempt" -eq 48 ]; then
            echo "Upgrade controller timed out"
            exit 1
        fi
    done

    juju upgrade-charm ubuntu --path=./tests/suites/upgrade/charms/ubuntu

    sleep 10
    wait_for "ubuntu" "$(idle_condition "ubuntu")"
}

test_upgrade_simplestream() {
    if [ -n "$(skip 'test_upgrade_simplestream')" ]; then
        echo "==> SKIP: Asked to skip stream tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_simplestream_metadata"
    )
}

last_stable_version() {
    local version major minor patch parts

    version="${1}"

    # shellcheck disable=SC2116
    version=$(echo "${version%-*}")

    major=$(echo "${version}" | cut -d '.' -f 1)
    minor=$(echo "${version}" | cut -d '.' -f 2)
    patch=$(echo "${version}" | cut -d '.' -f 3)

    parts=$(echo "${version}" | awk -F. '{print NF-1}')
    if [ "${parts}" -eq 2 ]; then
        if [ "${patch}" -eq 0 ]; then
            minor=$((minor-1))
        fi
        echo "${major}.${minor}"
        return
    fi

    minor=$((minor-1))
    echo "${major}.${minor}"
}

series_version() {
    local version series arch

    version="${1}"
    series="${2}"

    arch=$(echo "${version}" | sed 's:.*-::')

    # shellcheck disable=SC2116
    version=$(echo "${version%-*}")
    # shellcheck disable=SC2116
    version=$(echo "${version%-*}")

    echo "${version}-${series}-${arch}"
}

add_upgrade_tools() {
    local version jujud_path

    version=${1}

    jujud_path=$(which jujud)
    cp "${jujud_path}" "${TEST_DIR}"
    cd "${TEST_DIR}" || exit

    tar -zcvf "juju-${version}.tgz" jujud >/dev/null
    cd "${CURRENT_DIR}/.." || exit

    mkdir -p "./tests/suites/upgrade/streams/tools/released/"
    mv "${TEST_DIR}/juju-${version}.tgz" "./tests/suites/upgrade/streams/tools/released"
}

remove_upgrade_tools() {
    cd "${CURRENT_DIR}/.." || exit

    echo "==> Removing tools"
    rm -rf ./tests/suites/upgrade/streams/tools/released || true
    echo "==> Removed tools"
}

remove_upgrade_metadata() {
    cd "${CURRENT_DIR}/.." || exit

    echo "==> Removing metadata"
    rm -rf ./tests/suites/upgrade/streams/tools/streams || true
    echo "==> Removed metadata"
}
