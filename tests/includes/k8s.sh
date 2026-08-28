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

# wait_for_namespace_gone polls until the given namespace no longer exists in
# the cluster. It fails the test if the namespace is still present after the
# timeout (an integer number of seconds).
#
# ```
# wait_for_namespace_gone <namespace> [<timeout>]
# ```
wait_for_namespace_gone() {
	local name timeout

	name=${1}
	timeout=${2:-600} # default timeout: 600s = 10m

	attempt=0
	start_time="$(date -u +%s)"
	while kubectl get namespace "${name}" >/dev/null 2>&1; do
		echo "[+] (attempt ${attempt}) polling for namespace ${name} to be removed"
		sleep "${SHORT_TIMEOUT}"

		elapsed=$(date -u +%s)-$start_time
		if [[ ${elapsed} -ge ${timeout} ]]; then
			echo "[-] $(red 'timed out waiting for namespace') $(red "${name}") $(red 'to be removed')"
			kubectl get namespace "${name}" 2>&1 | sed 's/^/    | /g'
			exit 1
		fi

		attempt=$((attempt + 1))
	done

	echo "[+] $(green 'Namespace removed:') $(green "${name}")"
}

default_k8s() {
	if command -v minikube >/dev/null 2>&1 && [[ "Stopped" != "$(minikube status -o json | yq .APIServer)" ]]; then
		printf "minikube"
	elif command -v microk8s >/dev/null 2>&1 && [[ "True" == "$(microk8s status --format yaml | yq .microk8s.running)" ]]; then
		printf "microk8s"
	fi
}
