run_build() {
    OUT=$(make go-build)
    if [ -n "${OUT}" ]; then
        printf "\\nFound some issues"
        echo "\\n${OUT}"
        exit 1
    fi
}

test_build() {
    if [ "$(skip 'test_build')" ]; then
        echo "==> TEST SKIPPED: smoke build tests"
        return
    fi

    (
        set_verbosity

        cd ../

        # Check that build runs
        run "run_build"
    )
}
