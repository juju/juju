run_enable_ha() {
    echo

    file="${TEST_DIR}/enable_ha.log"

    ensure "enable-ha" "${file}"

    juju deploy "cs:~jameinel/ubuntu-lite-7"

    juju enable-ha

    attempt=0
    until [[ "$(juju machines -m controller --format=json | jq -r ".machines | .[] | .[\"juju-status\"] | select(.current == \"started\") | .current" | wc -l | grep "3")" ]]; do
        echo "[+] (attempt ${attempt}) polling machines"
        juju machines -m controller 2>&1 | sed 's/^/    | /g'
        sleep "${SHORT_TIMEOUT}"
        attempt=$((attempt+1))
        if [[ "${attempt}" -gt 50 ]]; then
            echo "enable-ha failed waiting for machines to start"
            exit 1
        fi
    done

    if [ "${attempt}" -gt 0 ]; then
        echo "[+] $(green 'Completed polling status for')" "$(green "${name}")"
        juju machines -m controller 2>&1 | sed 's/^/    | /g'
        # Although juju reports as an idle condition, some charms require a
        # breathe period to ensure things have actually settled.
        sleep "${SHORT_TIMEOUT}"
    fi

    juju remove-machine -m controller 1
    juju remove-machine -m controller 2

    attempt=0
    until [[ "$(juju machines -m controller --format=json | jq -r ".machines | .[] | .[\"juju-status\"] | select(.current == \"started\") | .current" | wc -l | grep "1")" ]]; do
        echo "[+] (attempt ${attempt}) polling machines"
        juju machines -m controller 2>&1 | sed 's/^/    | /g'
        sleep "${SHORT_TIMEOUT}"
        attempt=$((attempt+1))
        if [[ "${attempt}" -gt 50 ]]; then
            echo "removing ha failed waiting for machines to be destroyed"
            exit 1
        fi
    done

    if [ "${attempt}" -gt 0 ]; then
        echo "[+] $(green 'Completed polling status for')" "$(green "${name}")"
        juju machines -m controller 2>&1 | sed 's/^/    | /g'
        # Although juju reports as an idle condition, some charms require a
        # breathe period to ensure things have actually settled.
        sleep "${SHORT_TIMEOUT}"
    fi

    destroy_model "enable-ha"
}

test_enable_ha() {
    if [ -n "$(skip 'test_enable_ha')" ]; then
        echo "==> SKIP: Asked to skip controller enable-ha tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_enable_ha"
    )
}
