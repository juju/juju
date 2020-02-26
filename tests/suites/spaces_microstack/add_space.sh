run_add_space() {
    echo

    file="${TEST_DIR}/test-add-space.txt"

    ensure "spaces-add-space" "${file}"

    juju add-space space-1 192.168.222.0/24
    juju add-machine --constraints "spaces=space-1"
}

test_add_space() {
    if [ "$(skip 'test_add_space')" ]; then
        echo "==> TEST SKIPPED: add space"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_add_space" "$@"
    )
}
