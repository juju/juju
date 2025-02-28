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

test_container_resources() {
	if [ "$(skip 'test_container_resources')" ]; then
		echo "==> TEST SKIPPED: Container resources"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_container_attach_resource"
	)
}
