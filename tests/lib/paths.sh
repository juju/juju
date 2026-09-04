# machine_path returns a yq path to the LXD-profiles key for the given machine.
machine_path() {
	local machine

	machine=${1}

	echo ".machines | .[\"${machine}\"] | select(.[\"lxd-profiles\"]) | .[\"lxd-profiles\"] | keys"
}

# machine_container_path returns the yq path to the LXD-profiles key for the
# given container on the given machine.
machine_container_path() {
	local machine container

	machine=${1}
	container=${2}

	echo ".machines | .[\"${machine}\"] | .containers | .[\"${container}\"] | select(.[\"lxd-profiles\"]) | .[\"lxd-profiles\"] | keys"
}
