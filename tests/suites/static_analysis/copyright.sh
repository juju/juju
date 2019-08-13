run_copyright() {
    OUT=$(find . -name '*.go' | grep -v -E "(./vendor|./acceptancetests|./provider/azure/internal|./cloudconfig)" | sort | xargs grep -L -E '// (Copyright|Code generated)')
    LINES=$(echo "${OUT}" | wc -w)
    if [ "$LINES" != 0 ]; then
        echo "\\nThe following files are missing copyright headers"
        echo "${OUT}"
        exit 1
    fi
}

test_copyright() {
    if [ -n "${SKIP_STATIC_COPYRIGHT:-}" ]; then
        echo "==> SKIP: Asked to skip static copyright analysis"
        return
    fi

    (
        set -e

        cd ../

        # Check for copyright notices
        run "copyright"
    )
}
