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

controller_unit_names() {
	juju status -m controller --format=json |
		yq -r '.applications.controller.units | keys | .[]' |
		sort -t/ -k2,2n
}

wait_for_controller_unit_count() {
	local expected_count status actual_count idle_count attempt

	expected_count=${1}
	attempt=0
	until status=$(timeout 10 juju status -m controller --format=json 2>/dev/null) &&
		actual_count=$(yq -r '.applications.controller.units | length' <<<"${status}") &&
		idle_count=$(yq -r '.applications.controller.units | to_entries | map(select(.value["juju-status"].current == "idle" and .value["workload-status"].current != "error")) | length' <<<"${status}") &&
		[[ ${actual_count} -eq ${expected_count} && ${idle_count} -eq ${expected_count} ]]; do
		echo "[+] (attempt ${attempt}) waiting for ${expected_count} controller units to settle"
		timeout 10 juju status -m controller --format=yaml 2>&1 | sed 's/^/    | /g' || true
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
		if [[ ${attempt} -gt 200 ]]; then
			echo "controller units did not settle at ${expected_count} units"
			exit 1
		fi
	done

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling controller units')"
		juju status -m controller --format=yaml 2>&1 | sed 's/^/    | /g'
	fi
	sleep "${SHORT_TIMEOUT}"
}

remove_controller_unit() {
	local unit_name

	unit_name=${1}
	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s" | "kubernetes" | "microk8s")
		juju remove-unit -m controller controller --num-units 1 --no-prompt
		;;
	*)
		juju remove-unit -m controller "${unit_name}" --no-prompt
		;;
	esac
}

wait_for_ha() {
	amount=${1}

	attempt=0
	# shellcheck disable=SC2143
	# Poll controller machines until enough of them report a
	# "voter" controller-cluster-role. This is the 4.x replacement
	# for the per-machine `ha-status == "ha-enabled"` check used in
	# 3.x: only voter nodes replicate data and participate in the
	# dqlite quorum. Standbys replicate but do not vote, and spares
	# do neither, so counting them would report HA before a quorum
	# of controllers is established.
	until [[ "$(juju status -m controller --format=json 2>/dev/null | yq -r '.machines | to_entries[] | select(.value["controller-cluster-role"] == "voter") | .key' | wc -l | grep "${amount}")" ]]; do
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
