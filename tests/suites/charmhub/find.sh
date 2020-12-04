
run_charmhub_find_specific() {
    echo
    name="charmhub-find-specific"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    output=$(juju find ubuntu 2>&1 || true)
    check_not_contains "${output}" "No matching charms for"
    check_contains "${output}" ubuntu

    destroy_model "test-${name}"
}

run_charmhub_find_all() {
    echo
    name="charmhub-find-all"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    output=$(juju find 2>&1 || true)

    # This list is subject to change and could cause failures
    # in the future as we do not have controller over the data.
    # Series appear to be in alphabetical order, using example
    # with LTS only.
    check_contains "${output}" "No search term specified. Here are some interesting charms"
    check_contains "${output}" "bionic,focal,trusty,xenial"
    check_contains "${output}" "kubernetes"
    check_contains "${output}" "openstack-charmers"

    destroy_model "test-${name}"
}

run_charmhub_find_json() {
    echo
    name="charmhub-find-json"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    # There should always be 1 charm with ubuntu in the name,
    # charms should always have at least 1 supported series.
    output=$(juju find ubuntu --format json | jq '.[0].series | length')
    check_gt "${output}" "0"

    destroy_model "test-${name}"
}

run_charmhub_find_not_matching() {
    echo
    name="charmhub-find-not-matching"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    juju find nosuchcharm > "${output}" 2>&1
    check_contains "${output}" "No matching charms for"

    destroy_model "test-${name}"
}

run_charmstore_find() {
    echo
    name="charmstore-find"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    output=$(juju find cs:ubuntu 2>&1 || true)
    check_contains "${output}" "No matching charms for"

    destroy_model "test-${name}"
}

test_charmhub_find() {
      if [ "$(skip 'test_charmhub_find')" ]; then
        echo "==> TEST SKIPPED: Charm Hub find"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_charmhub_find_specific"
        run "run_charmhub_find_all"
        run "run_charmhub_find_json"
        run "run_charmstore_find"
    )
}