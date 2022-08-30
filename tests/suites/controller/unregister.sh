run_unregister() {
	echo

	file="${TEST_DIR}/unregister.log"

	ensure "unregister" "${file}"

	controller_name=$(juju controllers --format=json | jq -r '."current-controller"')

	echo "Change admin password"
	./tests/suites/controller/expect-scripts/juju-change-user-password.exp

	echo "Backup controller info before unregister"
	cp ~/.local/share/juju/controllers.yaml ~/.local/share/juju/controllers.yaml.bak

	echo "Unregister controller"
	juju unregister --yes "${controller_name}"

	juju controllers --format=json | jq -r ".\"controllers\".\"${controller_name}\"" | check null

	echo "Restore controller info after unregister"
	mv ~/.local/share/juju/controllers.yaml.bak ~/.local/share/juju/controllers.yaml

	echo "Login to controller info after restoring"
	./tests/suites/controller/expect-scripts/juju-login.exp "${controller_name}"
	juju switch unregister

	juju controllers --format=json | jq -r '."current-controller"' | check "${controller_name}"

	destroy_model "unregister"
}

test_unregister() {
	if [ -n "$(skip 'test_unregister')" ]; then
		echo "==> SKIP: Asked to skip controller unregister tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_unregister"
	)
}
