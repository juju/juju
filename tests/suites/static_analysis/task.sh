test_static_analysis() {
    if [ -n "${SKIP_STATIC:-}" ]; then
        echo "==> SKIP: Asked to skip static analysis"
        return
    fi

    test_copyright
    test_static_analysis_go
    test_static_analysis_shell
    test_schema
}
