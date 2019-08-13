run_build() {
    OUT=$(make go-build)
    if [ -n "${OUT}" ]; then
        printf "\\nFound some issues"
        echo "\\n${OUT}"
        exit 1
    fi
}

test_build() {
    if [ -n "${SKIP_SMOKE_BUILD:-}" ]; then
        echo "==> SKIP: Asked to skip smoke build tests"
        return
    fi

    (
        set -e

        cd ../

        # Check that build runs
        run "build"
    )
}
