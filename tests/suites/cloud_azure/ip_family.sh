run_ip_family_dual_stack() {
	(
		log_file="${TEST_DIR}/ip-family-dual.log"

		# The SSH reachability probes below require IPv6 egress on the CI
		# runner; fail fast instead of paying for an Azure bootstrap that
		# cannot be fully verified.
		if ! ip -6 route show default | grep -q .; then
			echo "==> ERROR: no IPv6 egress on CI runner, cannot verify SSH reachability"
			return 1
		fi

		bootstrap_additional_args=(--constraints "ip-family=dual")
		BOOTSTRAP_ADDITIONAL_ARGS="${bootstrap_additional_args[*]}" \
			BOOTSTRAP_REUSE=false \
			bootstrap "ip-family-dual" "$log_file"

		# The bootstrap helper leaves the newly added (empty) model active;
		# switch back to the controller model, where the bootstrap machine
		# and the bootstrap-time model constraints live
		# (mirrors constraints_model.sh:14).
		juju switch controller

		# Test #1: model-constraints includes ip-family=dual
		check_contains "$(juju model-constraints)" "ip-family=dual"

		# Test #2: the controller machine got an IPv6 address and is
		# reachable over IPv6 (per PR #22736 QA step 5).
		ipv6_addr=$(machine_ipv6 0)
		if [[ -z ${ipv6_addr} ]]; then
			echo "ERROR: no IPv6 address found on controller machine"
			return 1
		fi
		controller_hostname=$(juju show-machine 0 --format json |
			yq -r '.machines["0"]["hostname"]')
		if [[ -z ${controller_hostname} ]]; then
			echo "ERROR: controller machine has no hostname"
			return 1
		fi

		# On the controller machine we have an ssh key, so the probe
		# succeeds and returns the hostname.
		check_ipv6_ssh_probe "${ipv6_addr}" "${controller_hostname}" "$log_file"

		# Tests #3/#4: post-bootstrap provisioning paths. Run them in the
		# harness-created "ip-family-dual" model: bootstrap --constraints
		# lands only on the controller model (domain/model/bootstrap/
		# bootstrap.go:213), so this model is constraint-free and each
		# path must carry its own constraint. Machine IDs restart at 0.
		juju switch ip-family-dual

		# Test #3: per-machine constraint path
		# (machine domain -> provisioner -> StartInstance).
		juju add-machine --constraints "ip-family=dual" >>"$log_file" 2>&1
		wait_for_machine_agent_status "0" "started"
		ipv6_m0=$(machine_ipv6 0)
		if [[ -z ${ipv6_m0} ]]; then
			echo "ERROR: no IPv6 on machine 0 (add-machine --constraints ip-family=dual)"
			return 1
		fi

		# Workload machines have no ssh key by default.
		check_ipv6_ssh_probe "${ipv6_m0}" "Permission denied" "$log_file"

		# Test #4: application deploy constraint path
		# (application service -> provisioner -> StartInstance).
		# Deploy provisions a fresh machine per unit by default -> machine 1.
		juju deploy ubuntu --constraints "ip-family=dual" >>"$log_file" 2>&1
		wait_for_machine_agent_status "1" "started"
		ipv6_m1=$(machine_ipv6 1)
		if [[ -z ${ipv6_m1} ]]; then
			echo "ERROR: no IPv6 on machine 1 (deploy --constraints ip-family=dual)"
			return 1
		fi

		check_ipv6_ssh_probe "${ipv6_m1}" "Permission denied" "$log_file"

		destroy_controller "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	)
}

# machine_ipv6 echoes the first IPv6 address of the given machine.
# Candidates are restricted to hex digits and colons with at least two
# colons, and anything shaped like a MAC address is excluded.
machine_ipv6() {
	local id

	id=${1}

	juju show-machine "${id}" --format json |
		MACHINE_ID="${id}" yq -r '.machines[strenv(MACHINE_ID)]["ip-addresses"][] |
			select(. | test("^[0-9a-fA-F:]+$") and test(":.*:") and
				(test("^([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}$") | not))' |
		head -1
}

# check_ipv6_ssh_probe probes SSH over IPv6 on the given address and checks
# the output against an expected string. Port 22 is the only IPv6-open port
# at the NSG. On the controller machine an ssh key is available, so the
# remote command succeeds and returns the hostname; workload machines have
# no ssh key by default, so we expect "Permission denied", which still
# proves IPv6 reachability. A timeout/unreachable error would indicate no
# route. BatchMode makes auth failures immediate and deterministic (no
# interactive password prompt).
check_ipv6_ssh_probe() {
	local addr expected log_file ssh_out

	addr=${1}
	expected=${2}
	log_file=${3}

	echo "Probe ssh connection through ${addr}"
	ssh_out=$(timeout 30 ssh -6 -o BatchMode=yes \
		-o StrictHostKeyChecking=no -o ConnectTimeout=5 \
		ubuntu@"${addr}" hostname 2>&1 || true)
	echo "${ssh_out}" >>"${log_file}"
	check_contains "${ssh_out}" "${expected}"
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
