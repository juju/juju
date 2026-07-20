run_ip_family_dual_stack() {
	(
		log_file="${TEST_DIR}/ip-family-dual.log"

		bootstrap_additional_args=(--constraints "ip-family=dual")
		BOOTSTRAP_ADDITIONAL_ARGS="${bootstrap_additional_args[*]}" \
			BOOTSTRAP_REUSE=false \
			bootstrap "ip-family-dual" "$log_file"

		# The bootstrap helper leaves the newly added (empty) model active;
		# switch back to the controller model, where the bootstrap machine
		# and the bootstrap-time model constraints live
		# (mirrors constraints_model.sh:14).
		juju switch controller

		# Test #3: model-constraints includes ip-family=dual
		check_contains "$(juju model-constraints)" "ip-family=dual"

		# Test #2: IPv6 reachability (per PR #22736 QA step 5).
		ipv6_addr=$(juju show-machine 0 --format json |
			yq -r '.machines["0"]["ip-addresses"][] | select(. | test(":"))' |
			head -1)
		if [[ -z ${ipv6_addr} ]]; then
			echo "ERROR: no IPv6 address found on controller machine"
			return 1
		fi

		# Port 22 is the only IPv6-open port at the NSG. Reaching auth
		# ("Permission denied") proves reachability; timeout/unreachable
		# would indicate no route. BatchMode makes the auth failure
		# immediate and deterministic (no interactive password prompt).
		# The probe runs only when the runner has IPv6 egress; the
		# allocation check above is the primary assertion.
		if ip -6 route show default | grep -q .; then
			ssh_out=$(timeout 10 ssh -6 -o BatchMode=yes \
				-o StrictHostKeyChecking=no -o ConnectTimeout=5 \
				ubuntu@"${ipv6_addr}" hostname 2>&1 || true)
			echo "${ssh_out}" >>"$log_file"
			check_contains "${ssh_out}" "Permission denied"
		else
			echo "==> ERROR: no IPv6 egress on CI runner cannot verify SSH reachability"
			return 1
		fi

		# Tests #4/#5: post-bootstrap provisioning paths. Run them in the
		# harness-created "ip-family-dual" model: bootstrap --constraints
		# lands only on the controller model (domain/model/bootstrap/
		# bootstrap.go:213), so this model is constraint-free and each
		# path must carry its own constraint. Machine IDs restart at 0.
		juju switch ip-family-dual

		# Test #4: per-machine constraint path
		# (machine domain -> provisioner -> StartInstance; PR #22736 QA step 3).
		juju add-machine --constraints "ip-family=dual" >>"$log_file" 2>&1
		wait_for_machine_agent_status "0" "started"
		ipv6_m0=$(juju show-machine 0 --format json |
			yq -r '.machines["0"]["ip-addresses"][] | select(. | test(":"))' |
			head -1)
		if [[ -z ${ipv6_m0} ]]; then
			echo "ERROR: no IPv6 on machine 0 (add-machine --constraints ip-family=dual)"
			return 1
		fi

		# Test #5: application deploy constraint path
		# (application service -> provisioner -> StartInstance; PR #22736 QA step 4).
		# Deploy provisions a fresh machine per unit by default -> machine 1.
		juju deploy ubuntu --constraints "ip-family=dual" >>"$log_file" 2>&1
		wait_for_machine_agent_status "1" "started"
		ipv6_m1=$(juju show-machine 1 --format json |
			yq -r '.machines["1"]["ip-addresses"][] | select(. | test(":"))' |
			head -1)
		if [[ -z ${ipv6_m1} ]]; then
			echo "ERROR: no IPv6 on machine 1 (deploy --constraints ip-family=dual)"
			return 1
		fi

		destroy_controller "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	)
}

test_ip_family() {
	if [ "$(skip 'test_ip_family')" ]; then
		echo "==> TEST SKIPPED: ip-family"
		return
	fi

	if [ "${BOOTSTRAP_PROVIDER}" != "azure" ]; then
		echo "==> TEST SKIPPED: ip-family tests, not using azure"
		return
	fi

	if [ "$(az account list | yq -r 'length')" -lt 1 ]; then
		echo "==> TEST SKIPPED: not logged in to Azure cloud"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_ip_family_dual_stack"
	)
}
