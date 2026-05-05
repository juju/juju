kubectl() {
	local k8s="${BOOTSTRAP_CLOUD}"
	case "${BOOTSTRAP_PROVIDER}" in
	"k8s")
		k8s="${BOOTSTRAP_CLOUD:-$(default_k8s)}"
		;;
	*)
		# Use a local k8s that is available for CAAS testing needs.
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
	if command -v minikube >/dev/null 2>&1 && [[ "Stopped" != "$(minikube status -o json | yq .APIServer)" ]]; then
		printf "minikube"
	elif command -v microk8s >/dev/null 2>&1 && [[ "True" == "$(microk8s status --format yaml | yq .microk8s.running)" ]]; then
		printf "microk8s"
	fi
}
