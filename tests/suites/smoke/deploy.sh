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
    if [ "$(skip 'test_deploy')" ]; then
        echo "==> TEST SKIPPED: smoke deploy tests"
        return
    fi

    (
        set_verbosity

        # Check that deploy runs on LXD
        run "run_deploy"
    )
}
