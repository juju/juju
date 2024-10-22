# Copyright 2024 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

test_local_charms() {
	if [ "$(skip 'test_local_charms')" ]; then
		echo "==> TEST SKIPPED: deploy local charm tests"
		return
	fi

	(
		set_verbosity
		cd .. || exit
	)
}
