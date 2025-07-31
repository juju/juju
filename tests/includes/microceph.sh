enable_microceph_backed_storage() {
	juju switch controller

	if [[ $BOOTSTRAP_PROVIDER == "lxd" || $BOOTSTRAP_PROVIDER == "localhost" ]]; then
		# Deploy to a virtual machine so loopback devices work out of the box
		juju deploy microceph --channel="latest/edge" --config "snap-channel=reef/stable" --constraints "root-disk=16G cores=4 virt-type=virtual-machine"
	else
		juju deploy microceph --channel="latest/edge" --config "snap-channel=reef/stable" --constraints "root-disk=16G cores=4"
	fi

	# microceph is currently not a stable charm, so remove with force. Drop this when we use a stable release
	add_clean_func "juju remove-application -m controller microceph --force"

	juju deploy ceph-radosgw --channel "reef/stable"

	wait_for "microceph" "$(idle_condition "microceph")"

	juju add-storage microceph/0 osd-standalone="loop,2G,3"
	juju integrate microceph ceph-radosgw
	juju expose ceph-radosgw

	# NOTE: This can sometimes take surprisigly long
	wait_for "ceph-radosgw" "$(active_idle_condition "ceph-radosgw")"

	gw_ip=$(juju status --format json | jq -r '.applications["ceph-radosgw"].units["ceph-radosgw/0"]["public-address"]')
	key=$(juju ssh microceph/0 "sudo radosgw-admin user create --uid juju --display-name Juju" | jq -r ".keys[0]")
	juju controller-config \
		"object-store-type=s3" \
		"object-store-s3-endpoint=http://${gw_ip}:80" \
		"object-store-s3-static-key=$(echo "$key" | jq -r ".access_key")" \
		"object-store-s3-static-secret=$(echo "$key" | jq -r ".secret_key")"
}
