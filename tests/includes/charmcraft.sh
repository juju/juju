# pack_charm uses charmcraft to pack the local charm at the given directory,
# and returns a path to the packed charm which can be supplied to juju deploy.
#
# Example usage:
#    juju deploy $(pack_charm ./testcharms/charms/lxd-profile)
pack_charm() {
	local CHARM_DIR=$1
	CHARM_NAME=$(basename "$CHARM_DIR")

	charmcraft pack -p "$CHARM_DIR"
	find . -maxdepth 1 -name "${CHARM_NAME}_*.charm" -print
}
