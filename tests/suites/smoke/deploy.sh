run_deploy() {
    echo

    file="${TEST_DIR}/smoke-test-deploy.txt"

    ensure "smoke-test-deploy" "${file}"

    CHK=$(cat "${file}" | grep -i "ERROR" || true)
    if [ -n "${CHK}" ]; then
        printf "\\nFound some issues"
        cat "${file}" | xargs echo -I % "\\n%"
        exit 1
    fi

    destroy_model "smoke-test-deploy"
}

test_deploy() {
    if [ -n "${SKIP_SMOKE_DEPLOY:-}" ]; then
        echo "==> SKIP: Asked to skip smoke deploy tests"
        return
    fi

    (
        set -e

        # Check that deploy runs on LXD
        run "deploy"
    )
}
