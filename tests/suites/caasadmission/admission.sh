
run_deploy_microk8s() {
  echo

  name=${1}
  export BOOTSTRAP_PROVIDER=microk8s
  bootstrap microk8s "${name}"

  microk8s.config > "${TEST_DIR}"/kube.conf
}

test_deploy_admission() {
  name=test-$(petname)

  run_deploy_microk8s "${name}"

  namespace=controller-"${BOOTSTRAPPED_JUJU_CTRL_NAME}"

  kubectl --kubeconfig "${TEST_DIR}"/kube.conf apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: test
  namespace: $namespace
  labels:
    juju-app: test-app
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: $namespace
  name: test
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["create", "get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: role-grantor-binding
  namespace: $namespace
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: test
subjects:
  - kind: ServiceAccount
    name: test
    namespace: $namespace
EOF

  sa_secret=$(kubectl --kubeconfig "${TEST_DIR}"/kube.conf get sa -o json test -n "$namespace" | jq -r '.secrets[0].name')
  bearer_token=$(kubectl --kubeconfig "${TEST_DIR}"/kube.conf get secret -o json "$sa_secret" -n "$namespace" | jq -r '.data.token' | base64 -d)

  kubectl --kubeconfig "${TEST_DIR}"/kube.conf config view --raw -o json | jq "del(.users[0]) | .contexts[0].context.user = \"test\" | .users[0] = {\"name\": \"test\", \"user\": {\"token\": \"$bearer_token\"}}" > "${TEST_DIR}"/kube-sa.json


  # Short sleep to let juju controller watchers catch up.
  sleep 3

 kubectl --kubeconfig "${TEST_DIR}"/kube-sa.json apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  namespace: $namespace
data:
  test: test
EOF

juju_app=$(kubectl --kubeconfig "${TEST_DIR}"/kube-sa.json get cm -n "${namespace}" test -o=jsonpath='{.metadata.labels.juju-app}')
  check_contains "${juju_app}" "test-app"

  echo "$juju_app" | check test-app
}
