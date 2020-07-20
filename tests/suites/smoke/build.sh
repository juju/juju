run_build() {
    make go-build 2>&1
}

test_build() {
    if [ "$(skip 'test_build')" ]; then
        echo "==> TEST SKIPPED: smoke build tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        # Check that build runs
        run "run_build"
    )
}
