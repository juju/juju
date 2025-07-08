run_ovs_netplan_config() {
	echo

	# Deploy an LXD workload to a pre-built machine with 3 NICs that are
	# configured as follows:
	# - Two NICs are bonded and are part of an OVS bridge. The bridge is assigned
	# an address in 'space1'.
	# - The third NIC is assigned an address in 'space2'.
	juju switch controller
	juju deploy juju-qa-space-invader --series focal --constraints='tags=ovs' --to lxd:0 --bind 'space1 invade-b=space2'
	unit_index=$(get_unit_index "space-invader")
	wait_for "space-invader" "$(idle_condition "space-invader" 0 "${unit_index}")"

	# Check that the merged netplan configuration (/etc/netplan/99-juju.yaml)
	# that Juju generated because we asked for two spaces to be available to the
	# LXD container retained the `openvswitch: {}` section. See LP1942328.
	echo "[+] ensuring that the merged netplan configuration (99-juju.yaml) retained the empty openvswitch block injected by MAAS"
	got=$(juju ssh 0 'sudo cat /etc/netplan/99-juju.yaml | grep openvswitch || echo "FAIL"')
	if [[ $got =~ "FAIL" ]]; then
		# shellcheck disable=SC2046
		echo $(red "The merged netplan configuration did not retain the openvswitch block as an indicator that the bridge is managed by OVS instead of brctl")
		exit 1
	fi
}

test_ovs_netplan_config() {
	if [ "$(skip 'test_ovs_netplan_config')" ]; then
		echo "==> TEST SKIPPED: test OVS netplan config"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_ovs_netplan_config" "$@"
	)
}

# get_unit_index(app_name)
#
# Lookup and return the unit index for app_name.
get_unit_index() {
	local app_name

	app_name=${1}

	index=$(juju status | grep "${app_name}/" | cut -d' ' -f1 | cut -d'/' -f2 | cut -d'*' -f1)
	echo "$index"
}
