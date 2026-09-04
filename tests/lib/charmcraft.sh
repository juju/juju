# pack_charm uses charmcraft to pack the local charm at the given directory,
# and returns a path to the packed charm which can be supplied to juju deploy.
#
# Example usage:
#    juju deploy $(pack_charm ./testcharms/charms/lxd-profile)
pack_charm() {
	local CHARM_DIR=$1
	CHARM_NAME=$(basename "$CHARM_DIR")

	# charmcraft builds in the current working directory, so invoke it from the
	# charm directory to keep build artifacts out of the test directory and to
	# avoid staging/priming failures with the dump plugin.
	(
		cd "$CHARM_DIR" || exit 1
		charmcraft pack --destructive-mode
	)

	local charms=("${CHARM_DIR}/${CHARM_NAME}"_*.charm)
	if [[ ! -f "${charms[0]}" ]]; then
		echo "no charm package found in ${CHARM_DIR}" >&2
		return 1
	fi
	echo "${charms[0]}"
}
