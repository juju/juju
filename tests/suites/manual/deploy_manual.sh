test_deploy_manual() {
	if [ "$(skip 'test_deploy_manual')" ]; then
		echo "==> TEST SKIPPED: deploy manual"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		case "${BOOTSTRAP_PROVIDER:-}" in
		"lxd")
			run "run_deploy_manual_lxd"
			;;
		"ec2")
			run "run_deploy_manual_aws"
			;;
		*)
			echo "==> TEST SKIPPED: deploy manual - tests for LXD and AWS only"
			;;
		esac
	)
}

prepare_preexisting_authorized_key() {
	local addr preexisting_pub preexisting_pub_b64
	addr=${1}
	preexisting_pub=${2}
	preexisting_pub_b64=$(printf '%s' "${preexisting_pub}" | base64 -w0)

	ssh "ubuntu@${addr}" bash -s -- "${preexisting_pub_b64}" <<'EOF'
set -euo pipefail
command -v cloud-init >/dev/null 2>&1 && cloud-init status --wait >/dev/null 2>&1 || true
key=$(printf '%s' "${1}" | base64 -d)
[ -s ~/.ssh/authorized_keys ] && [ -n "$(tail -c 1 ~/.ssh/authorized_keys)" ] && echo >>~/.ssh/authorized_keys
printf '%s' "${key}" >>~/.ssh/authorized_keys
EOF
}

prepare_non_nopasswd_ubuntu_and_provisioner() {
	local addr
	addr=${1}

	ssh "ubuntu@${addr}" sudo bash -s <<'EOF'
set -euo pipefail
command -v cloud-init >/dev/null 2>&1 && cloud-init status --wait >/dev/null 2>&1 || true
id manualtest &>/dev/null || useradd -m -s /bin/bash manualtest
install -d -m 0700 -o manualtest -g manualtest ~manualtest/.ssh
install -m 0600 -o manualtest -g manualtest ~ubuntu/.ssh/authorized_keys ~manualtest/.ssh/authorized_keys
printf 'manualtest ALL=(ALL) NOPASSWD:ALL\n' >/etc/sudoers.d/99-manualtest
chmod 0440 /etc/sudoers.d/99-manualtest
find /etc/sudoers.d -maxdepth 1 -type f ! -name '99-manualtest' -exec sed -i '/^[[:space:]]*ubuntu\b/d' {} +
EOF

	if ssh -n -o BatchMode=yes "ubuntu@${addr}" sudo -n true 2>/dev/null; then
		echo "expected ubuntu passwordless sudo to be disabled on ${addr}"
		exit 1
	fi
}

verify_preexisting_authorized_key() {
	local addr preexisting_key out dupes
	local -a ssh_opts
	addr=${1}
	preexisting_key=${2}
	echo "==> Verifying pre-existing authorized key on ${addr}"

	ssh_opts=(
		-n -F /dev/null -i "${preexisting_key}"
		-o BatchMode=yes -o IdentitiesOnly=yes
		-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null
	)

	out=$(ssh "${ssh_opts[@]}" "ubuntu@${addr}" echo "preexisting-ok")
	check_contains "${out}" "preexisting-ok"

	dupes=$(ssh "${ssh_opts[@]}" "ubuntu@${addr}" "awk 'NF' ~/.ssh/authorized_keys | sort | uniq -d")
	if [[ -n ${dupes} ]]; then
		echo "unexpected duplicate entries in authorized_keys on ${addr}: ${dupes}"
		exit 1
	fi
}

manual_deploy() {
	local cloud_name name addr_m1 addr_m2
	local preexisting_key preexisting_pub

	set -o pipefail

	cloud_name=${1}
	name=${2}
	addr_m1=${3}
	addr_m2=${4}

	juju add-cloud --client "${cloud_name}" "${TEST_DIR}/cloud_name.yaml" 2>&1 | tee "${TEST_DIR}/add-cloud.log"

	file="${TEST_DIR}/test-${name}.log"

	export BOOTSTRAP_PROVIDER="manual"
	unset BOOTSTRAP_REGION
	bootstrap "${cloud_name}" "test-${name}" "${file}"
	juju switch controller

	preexisting_key="${TEST_DIR}/${name}-preexisting"
	ssh-keygen -f "${preexisting_key}" -t ed25519 -C "preexisting@${name}.com" -N ""
	preexisting_pub=$(cat "${preexisting_key}.pub")

	# Prepare addr_m1 with a pre-existing key and addr_m2 with a
	# pre-existing key plus disabled passwordless sudo for ubuntu so
	# InitUbuntuUser executes initUbuntuScript.
	prepare_preexisting_authorized_key "${addr_m1}" "${preexisting_pub}"
	prepare_non_nopasswd_ubuntu_and_provisioner "${addr_m2}"
	prepare_preexisting_authorized_key "${addr_m2}" "${preexisting_pub}"

	juju add-machine ssh:ubuntu@"${addr_m1}" 2>&1 | tee "${TEST_DIR}/add-machine-1.log"
	juju add-machine ssh:manualtest@"${addr_m2}" 2>&1 | tee "${TEST_DIR}/add-machine-2.log"

	verify_preexisting_authorized_key "${addr_m1}" "${preexisting_key}"
	verify_preexisting_authorized_key "${addr_m2}" "${preexisting_key}"

	juju enable-ha --to "1,2" 2>&1 | tee "${TEST_DIR}/enable-ha.log"
	wait_for "controller" "$(active_condition "controller" 0)"

	verify_preexisting_authorized_key "${addr_m1}" "${preexisting_key}"
	verify_preexisting_authorized_key "${addr_m2}" "${preexisting_key}"

	machine_base=$(juju machines --format=json | yq -r '.machines | .["0"] | .base | "\(.name)@\(.channel)"')

	if [[ -z ${machine_base} ]]; then
		echo "machine 0 has invalid base"
		exit 1
	fi

	juju deploy ubuntu --to=0 --base="${machine_base}"

	wait_for "ubuntu" "$(idle_condition "ubuntu" 1)"

	juju remove-application ubuntu

	destroy_controller "test-${name}"

	delete_user_profile "${name}"
}
