kubectl() {
	local k8s="${BOOTSTRAP_CLOUD}"
	case "${BOOTSTRAP_PROVIDER}" in
	"k8s")
		;;
	*)
		# Use a local k8s that is available for IAAS testing needs.
		k8s="$(default_k8s)"
		;;
	esac
	case "${k8s}" in
	"microk8s")
		if [ "$1" = "config" ] && [ "$2" = "view" ]; then
			microk8s.config
		else
			microk8s kubectl "$@"
		fi
		;;
	"minikube")
		minikube kubectl -- "$@"
		;;
	*)
		$(which kubectl) "$@"
		;;
	esac
}

default_k8s() {
	if which "minikube" >/dev/null 2>&1; then
		printf "minikube"
	elif which "microk8s" >/dev/null 2>&1; then
		printf "microk8s"
	fi
}
