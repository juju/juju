wait_for_controller_machines() {
	amount=${1}

	attempt=0
	# shellcheck disable=SC2143
	until [[ "$(juju machines -m controller --format=json | jq -r '.machines | .[] | .["juju-status"] | select(.current == "started") | .current' | wc -l | grep "${amount}")" ]]; do
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
	until [[ "$(juju show-controller --format=json | jq -r '.[] | .["controller-machines"] | .[] | select(.["ha-status"] == "ha-enabled") | .["instance-id"]' | wc -l | grep "${amount}")" ]]; do
		echo "[+] (attempt ${attempt}) polling ha"
		juju show-controller 2>&1 | yq '.[]["controller-machines"]' | sed 's/^/    | /g'
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))

		# Wait for roughly 16 minutes for a availability. In the field it's known
		# that availability can take this long.
		if [[ ${attempt} -gt 100 ]]; then
			echo "high availability failed waiting for machines to start"
			exit 1
		fi
	done

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling ha')"
		juju show-controller 2>&1 | sed 's/^/    | /g'

		sleep "${SHORT_TIMEOUT}"
	fi
}
