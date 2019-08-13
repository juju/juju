#!/bin/sh -e
[ -n "${GOPATH:-}" ] && export "PATH=${GOPATH}/bin:${PATH}"

# Always ignore SC2230 ('which' is non-standard. Use builtin 'command -v' instead.)
export SHELLCHECK_OPTS="-e SC2230 -e SC2039 -e SC2028 -e SC2002"

import_subdir_files() {
    test "$1"
    local file
    for file in "$1"/*.sh; do
        # shellcheck disable=SC1090
        . "$file"
    done
}

import_subdir_files includes

echo "==> Checking for dependencies"
check_dependencies curl jq shellcheck juju

if [ "${USER:-'root'}" = "root" ]; then
    echo "The testsuite must not be run as root." >&2
    exit 1
fi


cleanup() {
    # Allow for failures and stop tracing everything
    set +ex

    # Allow for inspection
    if [ -n "${TEST_INSPECT:-}" ]; then
        if [ "${TEST_RESULT}" != "success" ]; then
            echo "==> TEST DONE: ${TEST_CURRENT_DESCRIPTION}"
        fi
        echo "==> Test result: ${TEST_RESULT}"
        echo "Tests Completed (${TEST_RESULT}): hit enter to continue"

        # shellcheck disable=SC2034
        read -r nothing
    fi

    echo "==> Cleaning up"

    cleanup_jujus

    echo ""
    echo ""
    if [ "$TEST_RESULT" != "success" ]; then
        echo "==> TEST DONE: ${TEST_CURRENT_DESCRIPTION}"
    fi
    rm -rf "${TEST_DIR}"
    echo "==> Tests Removed: ${TEST_DIR}"
    echo "==> Test result: ${TEST_RESULT}"
}

TEST_CURRENT=setup
TEST_RESULT=failure

trap cleanup EXIT HUP INT TERM

# Setup test directory
TEST_DIR=$(mktemp -d tmp.XXX | xargs -I % echo "$(pwd)/%")

run_test() {
    TEST_CURRENT=${1}
    TEST_CURRENT_DESCRIPTION=${2:-${1}}
    TEST_CURRENT_NAME=${TEST_CURRENT#"test_"}

    if [ -n "${4}" ]; then
        TEST_CURRENT=${4}
    fi

    import_subdir_files "suites/${TEST_CURRENT_NAME}"

    echo "==> TEST BEGIN: ${TEST_CURRENT_DESCRIPTION}"
    START_TIME=$(date +%s)
    ${TEST_CURRENT}
    END_TIME=$(date +%s)

    echo "==> TEST DONE: ${TEST_CURRENT_DESCRIPTION} ($((END_TIME-START_TIME))s)"
}

# allow for running a specific set of tests
if [ "$#" -gt 0 ]; then
    run_test "test_${1}" "" "$@"
    TEST_RESULT=success
    exit
fi

run_test test_static_analysis "running static analysis"
run_test test_smoke "running smoke tests"
run_test test_cmr_bundles "running cmr bundle acceptance tests"

TEST_RESULT=success
