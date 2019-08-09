run_build() {
    OUT=$(make go-build)
    if [ -n "${OUT}" ]; then
        printf "\\nFound some issues"
        echo "\\n${OUT}"
        exit 1
    fi
}

test_build() {
    (
        set -e

        cd ../

        # Check that build runs
        run "build"
    )
}
