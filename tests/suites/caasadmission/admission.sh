run_controller_model_admission() {
  # Echo out to ensure nice output to the test suite.
	echo

  # The following ensures that a bootstrap juju exists.
  model_name="controller-model-admission"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	name=test-$(petname)

	namespace=controller-"${BOOTSTRAPPED_JUJU_CTRL_NAME}"

	kubectl --kubeconfig "${KUBE_CONFIG}" apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: $name
  namespace: $namespace
  labels:
    app.kubernetes.io/name: test-app
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: $namespace
  name: $name
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["create", "get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: $name
  namespace: $namespace
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: $name
subjects:
  - kind: ServiceAccount
    name: $name
    namespace: $namespace
EOF

	sa_secret=$(kubectl --kubeconfig "${KUBE_CONFIG}" get sa -o json "${name}" -n "$namespace" | jq -r '.secrets[0].name')
	bearer_token=$(kubectl --kubeconfig "${KUBE_CONFIG}" get secret -o json "$sa_secret" -n "$namespace" | jq -r '.data.token' | base64 -d)

	kubectl --kubeconfig "${KUBE_CONFIG}" config view --raw -o json | jq "del(.users[0]) | .contexts[0].context.user = \"test\" | .users[0] = {\"name\": \"test\", \"user\": {\"token\": \"$bearer_token\"}}" >"${TEST_DIR}"/kube-sa.json

	# Wait for the model operator to be ready
	echo "waiting for modeloperator to become available"
	while :; do
		# shellcheck disable=SC2046
		if [ $(kubectl --kubeconfig "${TEST_DIR}"/kube-sa.json get deploy -n "${namespace}" "modeloperator" -o=jsonpath='{.status.readyReplicas}' || echo "0") -eq 1 ]; then
			break
		fi
		sleep 1
	done

	# We still sleep quickly here to let everything settle down. By adding
	# propper probes we could avoid this.
	sleep 5

	kubectl --kubeconfig "${TEST_DIR}"/kube-sa.json apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: $name
  namespace: $namespace
data:
  test: test
EOF

	juju_app=$(kubectl --kubeconfig "${TEST_DIR}"/kube-sa.json get cm -n "${namespace}" "${name}" -o=jsonpath='{.metadata.labels.app\.juju\.is\/created-by}')
	check_contains "${juju_app}" "test-app"

	echo "$juju_app" | check test-app

	destroy_model "${model_name}"
}

run_new_model_admission() {
  # Echo out to ensure nice output to the test suite.
	echo

  # The following ensures that a bootstrap juju exists.
  model_name="new-model-admission"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	name=test-$(petname)
	namespace=${model_name}

	kubectl --kubeconfig "${KUBE_CONFIG}" apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: $name
  namespace: $namespace
  labels:
    app.kubernetes.io/name: test-app
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: $namespace
  name: $name
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["create", "get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: $name
  namespace: $namespace
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: $name
subjects:
  - kind: ServiceAccount
    name: $name
    namespace: $namespace
EOF

	sa_secret=$(kubectl --kubeconfig "${KUBE_CONFIG}" get sa -o json "$name" -n "$namespace" | jq -r '.secrets[0].name')
	bearer_token=$(kubectl --kubeconfig "${KUBE_CONFIG}" get secret -o json "$sa_secret" -n "$namespace" | jq -r '.data.token' | base64 -d)

	kubectl --kubeconfig "${TEST_DIR}"/kube.conf config view --raw -o json | jq "del(.users[0]) | .contexts[0].context.user = \"test\" | .users[0] = {\"name\": \"test\", \"user\": {\"token\": \"$bearer_token\"}}" >"${TEST_DIR}"/kube-sa.json

	# Wait for the model operator to be ready
	echo "waiting for modeloperator to become available"
	while :; do
		# shellcheck disable=SC2046
		if [ $(kubectl --kubeconfig "${TEST_DIR}"/kube-sa.json get deploy -n "${namespace}" "modeloperator" -o=jsonpath='{.status.readyReplicas}' || echo "0") -eq 1 ]; then
			break
		fi
		sleep 1
	done

	# We still sleep quickly here to let everything settle down. By adding
	# propper probes we could avoid this.
	sleep 5

	kubectl --kubeconfig "${TEST_DIR}"/kube-sa.json apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: $name
  namespace: $namespace
data:
  test: test
EOF

	juju_app=$(kubectl --kubeconfig "${TEST_DIR}"/kube-sa.json get cm -n "${namespace}" "${name}" -o=jsonpath='{.metadata.labels.app\.juju\.is\/created-by}')
	check_contains "${juju_app}" "test-app"

	echo "$juju_app" | check test-app

	destroy_model "${model_name}"
}

# Tests that after the model operator pod restarts it can come back up without
# having to be validated by itself.
run_model_chicken_and_egg() {
  # Echo out to ensure nice output to the test suite.
	echo

  # The following ensures that a bootstrap juju exists.
  model_name="model-chicken-and-egg"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	namespace=controller-"${BOOTSTRAPPED_JUJU_CTRL_NAME}"

	sleep 15
	kubectl --kubeconfig "${KUBE_CONFIG}" delete svc modeloperator -n "${namespace}"

	kubectl --kubeconfig "${KUBE_CONFIG}" patch deployment modeloperator -n "${namespace}" -p '{"metadata": {"labels": {"test": "foo"}}}'

	test_value=$(kubectl --kubeconfig "${KUBE_CONFIG}" get deployment -n "${namespace}" modeloperator -o=jsonpath='{.metadata.labels.test}')

	check_contains "${test_value}" "foo"

	destroy_model "${model_name}"
}
