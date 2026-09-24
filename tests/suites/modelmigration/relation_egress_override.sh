# Model migration tests for per-relation network policy.
#
# Migrating a consuming model with a per-relation egress override (juju
# integrate --via CIDRs) from a 3.6 controller into this 4.x controller must
# keep the override: the migration import has to persist the admin-supplied
# per-relation CIDRs, and network-get in relation scope must report them
# instead of the egress-subnets model default. This is the end-to-end
# counterpart of domain/modelmigration/import_relation_network_test.go
# (TestRelationEgressOverride).
#
# Only the supported 3.6 -> 4.0 migration direction is exercised. A
# 4.0 -> 4.0 migration would need the MigrationTarget v8 / SerializedModelV2
# path that lands in 4.1.
#
# The juju_36 client (3.6) is expected to already be installed on the PATH (by
# the user or the CI job) and to share the ambient JUJU_DATA client store with
# the juju client, so that both resolve the same source controller. Both
# controllers are bootstrapped on LXD.

# Fixed controller names keep the suite safe to re-run: the clean slate at the
# start of the test removes any leftovers from previous runs. The variables are
# defined at file level because the source controller cleanup registered with
# add_clean_func runs in the main shell after the subtest subshell has exited,
# where function-local variables are no longer available.
MODEL_MIGRATION_SRC_CTRL="modelmigration-src"
MODEL_MIGRATION_DST_CTRL="modelmigration-dst"

# The per-relation egress override applied with juju integrate --via.
MODEL_MIGRATION_VIA_CIDRS="198.51.100.0/25,192.0.2.0/24"

# A model-level egress default (egress-subnets) that differs from the
# per-relation override, so that a dropped override is distinguishable from the
# model default after the migration.
MODEL_MIGRATION_EGRESS_DEFAULT="203.0.113.0/24"

# All polling loops use this delay. The controller precheck waits five minutes;
# model landing and dummy-sink settling each wait 15 minutes.
MODEL_MIGRATION_POLL_DELAY=10
MODEL_MIGRATION_CONTROLLER_POLL_ATTEMPTS=30
MODEL_MIGRATION_SETTLE_POLL_ATTEMPTS=90
MODEL_MIGRATION_SETTLE_TIMEOUT_MINUTES=$((MODEL_MIGRATION_SETTLE_POLL_ATTEMPTS * MODEL_MIGRATION_POLL_DELAY / 60))

# modelmigration_network_get_egress prints the egress-subnets the dummy-sink/0
# unit reports for relation 0 of the source endpoint. The per-relation egress
# policy (--via) is only reported when network-get runs in relation scope (-r);
# without it the hook tool returns binding-level egress, which is always the
# egress-subnets model default. The `--` separator keeps juju exec from
# consuming the hook tool's flags.
modelmigration_network_get_egress() {
  local client model

  client=${1}
  shift

  model=${1}
  shift

  "${client}" exec -m "${model}" --unit dummy-sink/0 -- \
    network-get source -r 0 --format=yaml |
    grep -A3 'egress-subnets'
}

# modelmigration_destroy_src_controller destroys the 3.6 source controller. The
# framework only tracks controllers bootstrapped with the current juju client,
# so the source controller is torn down by this function instead. It is
# registered with add_clean_func once the source controller is bootstrapped, so
# it also runs when the test fails part way through; it is idempotent, so
# calling it again at the end of a successful test is a no-op.
modelmigration_destroy_src_controller() {
  if [ -n "${SKIP_DESTROY}" ]; then
    echo "====> Skipping destroy source controller"
    return
  fi

  OUT="$(juju_36 controllers --format json 2>/dev/null |
    yq -r '.controllers | keys | .[]' 2>/dev/null |
    grep "^${MODEL_MIGRATION_SRC_CTRL}$" || true)"
  if [ -z "${OUT}" ]; then
    return
  fi

  echo "====> Destroying juju 3.6 source controller ($(green "${MODEL_MIGRATION_SRC_CTRL}"))"
  output="${TEST_DIR}/${MODEL_MIGRATION_SRC_CTRL}-destroy-controller.log"
  juju_36 destroy-controller --destroy-all-models --force --no-prompt \
    "${MODEL_MIGRATION_SRC_CTRL}" >"${output}" 2>&1 || true
  echo "====> Destroyed juju 3.6 source controller ($(green "${MODEL_MIGRATION_SRC_CTRL}"))"
}

run_modelmigration_relation_egress_override() {
  local src dst cloud file unit_view via1 via2 saved_build_agent
  local uuid_4x uuid_36 attempt current settled

  src="${MODEL_MIGRATION_SRC_CTRL}"
  dst="${MODEL_MIGRATION_DST_CTRL}"

  # Both controllers must be bootstrapped on the same cloud: the 4.x target
  # precheck rejects a migration when the model's cloud is not known on the
  # target controller. The default matches the cloud name used by reproduce.sh;
  # BOOTSTRAP_CLOUD overrides it for both.
  cloud="${BOOTSTRAP_CLOUD:-lxd}"

  echo
  echo "==> Using juju ($(juju version)) and juju_36 ($(juju_36 version))"
  echo "==> Clean slate"
  # kill-controller is synchronous: it force-removes the controller machine and
  # drops the store entry immediately, so a controller left "destroying" by an
  # earlier failed run cannot block the bootstrap below.
  juju_36 kill-controller --no-prompt "${src}" >/dev/null 2>&1 ||
    juju_36 destroy-controller --destroy-all-models --force --no-prompt "${src}" >/dev/null 2>&1 ||
    true
  juju kill-controller --no-prompt "${dst}" >/dev/null 2>&1 ||
    juju destroy-controller --destroy-all-models --force --no-prompt "${dst}" >/dev/null 2>&1 ||
    true

  echo "==> Bootstrap the source controller (${src}) with juju 3.6"
  file="${TEST_DIR}/${src}-bootstrap.log"
  juju_36 bootstrap "${cloud}" "${src}" \
    --model-default enable-os-upgrade=false 2>&1 | OUTPUT "${file}"

  # The source controller is bootstrapped outside of the framework's controller
  # tracking, so register its teardown now to make sure it also runs if the
  # test fails part way through.
  add_clean_func "modelmigration_destroy_src_controller"

  echo "==> Create the offerer model on ${src}"
  juju_36 add-model -c "${src}" offerer

  juju_36 deploy -m "${src}:offerer" juju-qa-dummy-source
  juju_36 config -m "${src}:offerer" dummy-source token=yeah-boi
  juju_36 wait-for application -m "${src}:offerer" dummy-source --query='status=="active"'
  juju_36 offer -c "${src}" offerer.dummy-source:sink my-offer

  juju_36 status -m "${src}:offerer"

  echo "==> Create the consumer model on ${src} with a per-relation egress override"
  juju_36 add-model -c "${src}" consumer

  juju_36 model-config -m "${src}:consumer" "egress-subnets=${MODEL_MIGRATION_EGRESS_DEFAULT}"

  juju_36 deploy -m "${src}:consumer" juju-qa-dummy-sink

  # Same-controller consume: the offer creates the remote application and
  # remote entity tokens that the 4.x import must map the egress CIDRs onto.
  # 3.6 exports local offers with the source controller's connection info
  # (state/migrations/externalcontrollers.go), so the migrated model can reach
  # back to ${src} to keep the relation alive.
  juju_36 consume -m "${src}:consumer" admin/offerer.my-offer
  juju_36 integrate -m "${src}:consumer" dummy-sink:source my-offer \
    --via "${MODEL_MIGRATION_VIA_CIDRS}"

  juju_36 wait-for application -m "${src}:consumer" dummy-sink --query='status=="active"'
  juju_36 status -m "${src}:consumer" --relations

  echo "==> Check the per-relation egress override before migrating"
  echo "Model default (expect ${MODEL_MIGRATION_EGRESS_DEFAULT}):"
  OUT="$(juju_36 model-config -m "${src}:consumer" egress-subnets)"
  echo "${OUT}"
  check_contains "${OUT}" "${MODEL_MIGRATION_EGRESS_DEFAULT}"

  unit_view="$(modelmigration_network_get_egress juju_36 "${src}:consumer" || true)"
  echo "Unit view of the relation (expect ${MODEL_MIGRATION_VIA_CIDRS}):"
  echo "${unit_view}"
  via1="${MODEL_MIGRATION_VIA_CIDRS%%,*}"
  via2="${MODEL_MIGRATION_VIA_CIDRS##*,}"
  check_contains "${unit_view}" "${via1}"
  check_contains "${unit_view}" "${via2}"

  echo "==> Bootstrap the target controller (${dst})"
  # The migration import runs on the target controller's jujud, not with the
  # binaries of this machine. The target must therefore be bootstrapped with
  # the agent binaries built from this checkout (--build-agent); with the
  # default agent version lookup the bootstrap downloads the released agent
  # binaries, which do not include the code under test, and the import silently
  # runs stale code, dropping the egress override: the very bug this test
  # guards against. BUILD_AGENT is restored afterwards so that later suites
  # keep their own bootstrap behaviour.
  saved_build_agent="${BUILD_AGENT}"
  export BUILD_AGENT=true

  # BOOTSTRAP_ADDITIONAL_ARGS is extended rather than overwritten so that
  # arguments from the environment survive; pre_bootstrap appends the agent
  # version arguments and post_bootstrap resets the variable afterwards.
  export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --model-default enable-os-upgrade=false"
  file="${TEST_DIR}/${dst}-bootstrap.log"
  juju_bootstrap "${cloud}" "${dst}" "modelmigration" "${file}"

  export BUILD_AGENT="${saved_build_agent}"

  echo "==> Sanity check: both clients resolve the same ${src} controller"
  # The 3.6 bootstrap registered ${src} (with credentials) in the shared client
  # store and juju migrate needs to resolve the same controller. If the two
  # clients used different stores, the migration would hit a stale ${src} and
  # fail with a cryptic permission denied error: a model missing from a
  # controller is reported as ErrPerm, not as not found.
  uuid_4x="$(juju show-controller "${src}" --format json 2>/dev/null |
    yq -r ".[\"${src}\"] | .details | .uuid" 2>/dev/null || true)"
  uuid_36="$(juju_36 show-controller "${src}" --format json 2>/dev/null |
    yq -r ".[\"${src}\"] | .details | .uuid" 2>/dev/null || true)"
  if [ -z "${uuid_4x}" ] || [ "${uuid_4x}" != "${uuid_36}" ]; then
    echo "ERROR: juju and juju_36 resolve ${src} to different controllers"
    echo "(juju: ${uuid_4x:-unresolved}; juju_36: ${uuid_36:-unresolved})"
    echo "Both clients must share one JUJU_DATA client store."
    exit 1
  fi
  OUT="$(juju models -c "${src}" --format json 2>/dev/null |
    yq -r '.models | .[] | .["short-name"] | select(. == "consumer")' 2>/dev/null || true)"
  if [ -z "${OUT}" ]; then
    echo "ERROR: the juju client cannot see ${src}:admin/consumer"
    exit 1
  fi

  # The 4.x target precheck (TargetPrecheck -> checkMachines) requires the
  # target controller's own machines to report instance status running. Right
  # after the bootstrap the controller machine's instance status is still
  # pending; the instancepoller only flips it to running on its first provider
  # poll (seconds after the controller agent starts). Migrating inside that
  # window fails with: machine "0" instance status is not running. Wait it out.
  echo "==> Waiting for the ${dst} controller machine instance status"
  attempt=0
  settled=""
  while [ "${attempt}" -lt "${MODEL_MIGRATION_CONTROLLER_POLL_ATTEMPTS}" ]; do
    current="$(juju status -m "${dst}:controller" --format json 2>/dev/null |
      yq -r '.machines["0"]["machine-status"].current' 2>/dev/null || true)"
    if [ "${current}" = "running" ]; then
      settled="true"
      break
    fi
    echo "[+] (attempt ${attempt}) ${dst} controller machine instance status: ${current:-unknown}"
    sleep "${MODEL_MIGRATION_POLL_DELAY}"
    attempt=$((attempt + 1))
  done
  if [ -z "${settled}" ]; then
    echo "[-] $(red 'timed out waiting for')" "$(red "${dst}")"
    echo "    the controller machine never reported instance status running"
    exit 1
  fi

  echo "==> Migrate ${src}:admin/consumer to ${dst}"
  juju migrate "${src}:admin/consumer" "${dst}"

  echo "==> Waiting for the consumer model to land on ${dst}"
  attempt=0
  settled=""
  while [ "${attempt}" -lt "${MODEL_MIGRATION_SETTLE_POLL_ATTEMPTS}" ]; do
    if juju status -m "${dst}:consumer" >/dev/null 2>&1; then
      settled="true"
      break
    fi
    echo "[+] (attempt ${attempt}) waiting for ${dst}:consumer"
    sleep "${MODEL_MIGRATION_POLL_DELAY}"
    attempt=$((attempt + 1))
  done
  if [ -z "${settled}" ]; then
    echo "[-] $(red 'timed out waiting for')" "$(red "${dst}:consumer")"
    juju status -m "${dst}:consumer" --format yaml 2>&1 |
      sed 's/^/    | /g' || true
    exit 1
  fi

  echo "==> Waiting for dummy-sink to settle on ${dst}:consumer"
  # A migrated unit can take a while to resettle, and the egress override check
  # below is the actual assertion, so only warn on a timeout instead of
  # failing.
  attempt=0
  settled=""
  while [ "${attempt}" -lt "${MODEL_MIGRATION_SETTLE_POLL_ATTEMPTS}" ]; do
    current="$(juju status -m "${dst}:consumer" --format json 2>/dev/null |
      yq -r '.applications["dummy-sink"]["application-status"].current' 2>/dev/null || true)"
    if [ "${current}" = "active" ]; then
      settled="true"
      break
    fi
    echo "[+] (attempt ${attempt}) dummy-sink application status: ${current:-unknown}"
    sleep "${MODEL_MIGRATION_POLL_DELAY}"
    attempt=$((attempt + 1))
  done
  if [ -z "${settled}" ]; then
    echo "WARNING: dummy-sink not active on ${dst}:consumer after ${MODEL_MIGRATION_SETTLE_TIMEOUT_MINUTES}m; continuing to the egress checks"
    juju status -m "${dst}:consumer" --format yaml 2>&1 |
      sed 's/^/    | /g' || true
  fi

  echo "==> Check the per-relation egress override survived the migration"
  juju status -m "${dst}:consumer" --relations

  echo "Model default (expect ${MODEL_MIGRATION_EGRESS_DEFAULT}):"
  OUT="$(juju model-config -m "${dst}:consumer" egress-subnets)"
  echo "${OUT}"
  check_contains "${OUT}" "${MODEL_MIGRATION_EGRESS_DEFAULT}"

  unit_view="$(modelmigration_network_get_egress juju "${dst}:consumer" || true)"
  echo "Unit view of the relation (expect ${MODEL_MIGRATION_VIA_CIDRS}):"
  echo "${unit_view}"
  check_contains "${unit_view}" "${via1}"
  check_contains "${unit_view}" "${via2}"

  # Destroy the target first: while the source controller is still alive,
  # destroying the consumer model breaks the cross-model relation cleanly and
  # drops the offer connections. The source controller is then destroyed by the
  # registered cleanup function.
  destroy_controller "${dst}"
  modelmigration_destroy_src_controller
}

test_modelmigration_relation_egress_override() {
  if [ "$(skip 'test_modelmigration_relation_egress_override')" ]; then
    echo "==> TEST SKIPPED: relation egress override migration tests"
    return
  fi

  (
    set_verbosity

    cd .. || exit

    run "run_modelmigration_relation_egress_override"
  )
}
