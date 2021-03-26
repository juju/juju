run_other() {
	echo

	file="${TEST_DIR}/test-other.log"
	ensure "other" "${file}"

	echo "Hello other!"

	destroy_model "other"
}

test_other() {
	if [ -n "$(skip 'test_other')" ]; then
		echo "==> SKIP: Asked to skip other tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_other"
	)
}
