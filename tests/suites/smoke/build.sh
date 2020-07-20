run_build() {
    OUT=$(make go-build 2>&1 || true)
    if [ $? -ne 0 ]; then
        echo ""
        echo "$(red 'Found some issues:')"
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

        cd .. || exit

        # Check that build runs
        run "run_build"
    )
}
