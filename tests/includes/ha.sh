wait_for_controller_machines() {
	amount=${1}

	attempt=0
	# shellcheck disable=SC2143
	until [[ "$(juju machines -m controller --format=json | yq -r '.machines | .[] | .["juju-status"] | select(.current == "started") | .current' | wc -l | grep "${amount}")" ]]; do
		echo "[+] (attempt ${attempt}) polling machines"
		juju machines -m controller 2>&1 | sed 's/^/    | /g' || true
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))

		# Wait for roughly 16 minutes for a availability. In the field it's known
		# that availability can take this long.
		if [[ ${attempt} -gt 200 ]]; then
			echo "availability failed waiting for machines to start"
			exit 1
		fi
	done

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling machines')"
		juju machines -m controller 2>&1 | sed 's/^/    | /g'

		sleep "${SHORT_TIMEOUT}"
	fi
}

wait_for_ha() {
	amount=${1}

	attempt=0
	# shellcheck disable=SC2143
	# Poll controller machines until each reports a non-null,
	# non-"unknown" controller-cluster-role. This is the 4.x
	# replacement for the per-machine `ha-status == "ha-enabled"`
	# check used in 3.x: a controller machine only reports a real
	# dqlite role (voter/standby/spare) once it has joined the HA
	# cluster, so "present and not unknown" is the readiness proxy.
	until [[ "$(juju status -m controller --format=json 2>/dev/null | yq -r '.machines | to_entries[] | select(.value["controller-cluster-role"] != null and .value["controller-cluster-role"] != "unknown") | .key' | wc -l | grep "${amount}")" ]]; do
		echo "[+] (attempt ${attempt}) polling ha"
		juju status -m controller --format=yaml 2>&1 | yq '.machines | with_entries(.value |= pick(["instance-id", "controller-cluster-role"]))' 2>&1 | sed 's/^/    | /g' || true
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))

		# Wait for roughly 16 minutes for a availability. In the field it's known
		# that availability can take this long.
		if [[ ${attempt} -gt 100 ]]; then
			echo "high availability failed waiting for machines to join the HA cluster"
			exit 1
		fi
	done

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling ha')"
		juju status -m controller --format=yaml 2>&1 | yq '.machines | with_entries(.value |= pick(["instance-id", "controller-cluster-role"]))' 2>&1 | sed 's/^/    | /g'

		sleep "${SHORT_TIMEOUT}"
	fi
}
