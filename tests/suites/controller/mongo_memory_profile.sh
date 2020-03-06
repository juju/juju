cat_mongo_service() {
    # shellcheck disable=SC2046
    echo $(juju run -m controller --machine 0 'cat /etc/systemd/system/juju-db.service' | grep "^ExecStart")
}

run_mongo_memory_profile() {
    echo

    file="${TEST_DIR}/mongo_memory_profile.txt"

    ensure "mongo-memory-profile" "${file}"

    check_not_contains "$(cat_mongo_service)" wiredTigerCacheSizeGB

    juju controller-config mongo-memory-profile=low

    sleep 5

    attempt=0
    # shellcheck disable=SC2046,SC2143,SC2091
    until $(check_contains "$(cat_mongo_service)" wiredTigerCacheSizeGB >/dev/null 2>&1); do
        echo "[+] (attempt ${attempt}) polling mongo service"
        cat_mongo_service | sed 's/^/    | /g'
        # This will attempt to wait for 2 minutes before failing out.
        if [ "${attempt}" -ge 24 ]; then
            echo "Failed: expected wiredTigerCacheSizeGB to be set in mongo service."
            exit 1
        fi
        sleep 5
        attempt=$((attempt+1))
    done

    # Set the value back in case we are reusing a controller
    juju controller-config mongo-memory-profile=default

    sleep 5

    destroy_model "mongo-memory-profile"
}

test_mongo_memory_profile() {
    if [ -n "$(skip 'test_mongo_memory_profile')" ]; then
        echo "==> SKIP: Asked to skip controller mongo memory profile tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_mongo_memory_profile"
    )
}
