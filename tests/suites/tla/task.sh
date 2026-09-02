test_tla() {
	if [ "$(skip 'test_tla')" ]; then
		echo "==> TEST SKIPPED: tla checks"
		return
	fi

	set_verbosity

	test_tla_objectstore
}
