cat_mongo_service() {
    # shellcheck disable=SC2046
    echo $(juju run -m controller --machine 0 'cat /lib/systemd/system/juju-db/juju-db.service' | grep "^ExecStart")
}

run_mongo_memory_profile() {
    echo

    file="${TEST_DIR}/mongo_memory_profile.txt"

    ensure "mongo-memory-profile" "${file}"

    check_not_contains "$(cat_mongo_service)" wiredTigerCacheSizeGB

    juju controller-config mongo-memory-profile=low

    sleep 5

    check_contains "$(cat_mongo_service)" wiredTigerCacheSizeGB

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

        cd ../

        run "run_mongo_memory_profile"
    )
}
