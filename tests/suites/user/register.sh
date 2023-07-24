run_user_register() {
	echo

	echo "Check that current user is admin"
	juju whoami --format=json | jq -r '."user"' | check "admin"

	echo "Add user with read rights"
	juju remove-user -y bob 2>/dev/null || true
	OUT=$(juju add-user bob)
	REG_CMD=$(echo $OUT | awk '{for (i=0; i<=NF; i++){if ($i == "register"){print $(i-1)" "$(i)" "$(i+1);exit}}}')
	if [[ ${REG_CMD} != "juju register "* ]]; then
		echo "unexpected juju register output"
		echo "${OUT}"
	fi
	juju grant bob read "test-user"

	rm -rf /tmp/bob || true
	mkdir -p /tmp/bob
	printf 'secret\nsecret\ntest\nsecret\n' | JUJU_DATA=/tmp/bob ${REG_CMD}
	MODELS=$(JUJU_DATA=/tmp/bob juju models)
	check_contains "${MODELS}" "admin/test-user"
}

test_user_register() {
	if [ -n "$(skip 'test_user_register')" ]; then
		echo "==> SKIP: Asked to skip user register tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_user_register"
	)
}
