run_unregister() {
	echo

	file="${TEST_DIR}/unregister.log"

	ensure "unregister" "${file}"

	echo "Get controller name"
	controller_name=$(juju controllers --format=json | jq -r '."current-controller"')

	echo "Check controller is known"
	juju controllers --format=json | jq -r ".\"controllers\" | has(\"${controller_name}\")" | check true

	echo "Backup controller info before unregister"
	cp ~/.local/share/juju/controllers.yaml ~/.local/share/juju/controllers.yaml.bak
	cp ~/.local/share/juju/accounts.yaml ~/.local/share/juju/accounts.yaml.bak

	echo "Unregister controller"
	juju unregister --yes "${controller_name}"

	echo "Check controller is not known"
	juju controllers --format=json | jq -r ".\"controllers\".\"${controller_name}\"" | check null

	echo "Check the default controller is not equal to unregistered one"
	check_not_contains "$(juju controllers --format=json | jq -r '."current-controller"')" "${controller_name}"

	echo "Restore controller info after unregister"
	mv ~/.local/share/juju/controllers.yaml.bak ~/.local/share/juju/controllers.yaml
	mv ~/.local/share/juju/accounts.yaml.bak ~/.local/share/juju/accounts.yaml

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
