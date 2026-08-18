# pack_charm uses charmcraft to pack the local charm at the given directory,
# and returns the path to the packed charm which can be supplied to juju deploy.
#
# The function returns a resolved file path so callers may safely quote the
# substitution:
#    juju deploy "$(pack_charm ./testcharms/charms/lxd-profile)"
#
# The unquoted form also works:
#    juju deploy $(pack_charm ./testcharms/charms/lxd-profile)
pack_charm() {
	local CHARM_DIR=$1
	CHARM_NAME=$(basename "$CHARM_DIR")

	charmcraft pack -p "$CHARM_DIR"
	local charm_file
	charm_file=$(ls -1 ./"${CHARM_NAME}"_*.charm 2>/dev/null | head -n1)
	echo "${charm_file:-./${CHARM_NAME}_*.charm}"
}
