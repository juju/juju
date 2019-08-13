test_smoke() {
    if [ -n "${SKIP_SMOKE:-}" ]; then
        echo "==> SKIP: Asked to skip smoke tests"
        return
    fi

    test_build
    test_deploy
}
