run_charmhub_download() {
    echo
    name="charmhub-download"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    output=$(juju download mysql --series xenial --filepath="${TEST_DIR}/mysql.charm" 2>&1 || true)
    check_contains "${output}" "Fetching charm \"mysql\""

    juju deploy "${TEST_DIR}/mysql.charm" mysql
    juju wait-for application mysql
}

run_charmstore_download() {
    echo
    name="charmstore-download"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    output=$(juju download cs:meshuggah 2>&1 || echo "not found")
    check_contains "${output}" "\"cs:meshuggah\" is not a Charm Hub charm"
}

run_unknown_download() {
    echo
    name="unknown-download"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    output=$(juju download meshuggah 2>&1 || echo "not found")
    check_contains "${output}" "The Charm with the given name was not found in the Store"
}

test_charmhub_download() {
      if [ "$(skip 'test_charmhub_download')" ]; then
        echo "==> TEST SKIPPED: Charm Hub download"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_charmhub_download"
        run "run_charmstore_download"
        run "run_unknown_download"
    )
}