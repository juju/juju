run_text_primary_key() {
	# Due to a bug in old versions of SQLite, the devs decided that non-integer
	# primary keys can take a null value by default. We never want this. This
	# test ensures that there are no tables in our DDL which could be affected
	# by this.
	# See https://www.sqlite.org/lang_createtable.html
	res="$(egrep -r 'TEXT PRIMARY KEY\s*,?$' ./domain/schema/)"
	if [ -n "$res" ]; then
		echo "SQLite allows for nullable text primary keys, which is not what we want."
		echo "TEXT PRIMARY KEY found in the following files. Please add NOT NULL constraints:"
		echo "$res" | awk '{print $1}' | sort | uniq
		return 1
	fi
}

test_text_primary_key() {
	if [ "$(skip 'test_text_primary_key')" ]; then
		echo "==> TEST SKIPPED: test_text_primary_key"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run_linter "run_text_primary_key"
	)
}
