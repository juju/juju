run_copyright() {
    OUT=$(find . -name '*.go' | grep -v -E "(./vendor|./acceptancetests|./provider/azure/internal|./cloudconfig)" | sort | xargs grep -L -E '// (Copyright|Code generated)' || true)
    LINES=$(echo "${OUT}" | wc -w)
    if [ "$LINES" != 0 ]; then
        echo ""
        echo "$(red 'Found some issues:')"
        echo "\\nThe following files are missing copyright headers"
        echo "${OUT}"
        exit 1
    fi
}

test_copyright() {
    if [ "$(skip 'test_copyright')" ]; then
        echo "==> TEST SKIPPED: static copyright analysis"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        # Check for copyright notices
        run "run_copyright"
    )
}
