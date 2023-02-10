run_check_doc_go() {
	python3 tests/suites/static_analysis/doc_go.py -i ./_deps
}

test_doc_go() {
	if [ "$(skip 'test_doc_go')" ]; then
		echo "==> TEST SKIPPED: test doc go"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run_linter "run_check_doc_go"
	)
}
