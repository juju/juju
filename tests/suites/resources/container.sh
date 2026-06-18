run_container_attach_resource() {
	echo
	name="container-resource"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"

	# Install podman if not present.
	if ! which "podman" >/dev/null 2>&1; then
		sudo apt install podman -y
	fi

	# Start a local container registry.
	podman run -d -p 5000:5000 --name registry docker.io/library/registry:2.7

	# Build resource images.
	podman build -t localhost:5000/resource-1 ./tests/suites/resources/containers/resource-1
	podman build -t localhost:5000/resource-2 ./tests/suites/resources/containers/resource-2

	# Push the images to the local podman registry.
	podman push --tls-verify=false localhost:5000/resource-1
	podman push --tls-verify=false localhost:5000/resource-2

	# Deploy with resource 1.
	juju deploy juju-qa-container-resource --resource app-image=localhost:5000/resource-1
	wait_for "juju-qa-container-resource" "$(idle_condition "juju-qa-container-resource")"
	# Wait for resource message in status.
	wait_for "Resource container whoami server: I am resource 1" "$(workload_status juju-qa-container-resource 0).message"

	# Attach resource 2.
	juju attach-resource juju-qa-container-resource app-image=localhost:5000/resource-2
	wait_for "juju-qa-container-resource" "$(idle_condition "juju-qa-container-resource")"
	# Wait for resource message in status.
	wait_for "Resource container whoami server: I am resource 2" "$(workload_status juju-qa-container-resource 0).message"

	# Attach revision 3 of the charmhub resource.
	# TODO (aflynn): `juju attach-resource juju-qa-container-resource app-image=3`
	# should be used below but is currently not working due to a bug. For now, the
	# refresh command is used to do the same thing.
	juju refresh juju-qa-container-resource --resource app-image=3
	# Wait for resource message in status.
	wait_for "Resource container whoami server: I am the charmhub resource (revision 3)" "$(workload_status juju-qa-container-resource 0).message"

	# Attach revision 4 of the charmhub resource.
	juju refresh juju-qa-container-resource --resource app-image=4
	# Wait for resource message in status.
	wait_for "Resource container whoami server: I am the charmhub resource (revision 4)" "$(workload_status juju-qa-container-resource 0).message"

	# Shut down and remove the registry container.
	echo "removing container registry"
	podman rm --force registry

	# Destroy the model.
	destroy_model "test-${name}"
}

add_wrench_resource_upload_delay() {
	juju ssh -m controller controller/0 \
		'mkdir -p /var/lib/juju/wrench && echo "upload-delay" > /var/lib/juju/wrench/resources'
	juju ssh -m controller controller/0 \
		'test -f /var/lib/juju/wrench/resources && grep -x "upload-delay" /var/lib/juju/wrench/resources >/dev/null'
}

cleanup_wrench_resource_upload_delay() {
	juju ssh -m controller controller/0 \
		'mkdir -p /var/lib/juju/wrench && : > /var/lib/juju/wrench/resources' >/dev/null 2>&1 || true
}

run_container_deploy_resource_slow_upload() {
	echo
	name="container-resource-k8s-metadata"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"
	add_clean_func "cleanup_wrench_resource_upload_delay"
	add_wrench_resource_upload_delay

	metadata_file="${TEST_DIR}/snappass-metadata.json"
	cat >"${metadata_file}" <<EOF
{
  "ImageName": "samueldg/snappass:latest"
}
EOF

	juju deploy snappass-test --resource snappass-image="${metadata_file}"
	wait_for "active" '.applications["snappass-test"]["application-status"].current'
	wait_for "active" '.applications["snappass-test"].units["snappass-test/0"]["workload-status"].current'

	destroy_model "test-${name}"
}

test_container_resources() {
	if [ "$(skip 'test_container_resources')" ]; then
		echo "==> TEST SKIPPED: Container resources"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_container_attach_resource"
		run "run_container_deploy_resource_slow_upload"
	)
}
