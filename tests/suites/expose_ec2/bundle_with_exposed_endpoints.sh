run_bundle_with_exposed_endpoints() {
    echo

    file="${TEST_DIR}/test-bundle-with-exposed-endpoints.log"

    ensure "bundle-with-exposed-endpoints" "${file}"

    assert_deploy_bundle_with_expose_flag_and_exposed_endpoints_fails

    destroy_model "bundle-with-exposed-endpoints"
}

assert_deploy_bundle_with_expose_flag_and_exposed_endpoints_fails() {
    echo "==> Checking that deploying a bundle with both the expose flag and exposed endpoint sections is not allowed"

    bundle=./tests/suites/expose_ec2/bundles/invalid.yaml
    got=$(juju deploy ${bundle} 2>&1 || true)
    check_contains "${got}" 'exposed-endpoints cannot be specified together with "exposed:true" in application "ubuntu-lite" as this poses a security risk when deploying bundles to older controllers'
}

test_bundle_with_exposed_endpoints() {
    if [ "$(skip 'test_bundle_with_exposed_endpoints')" ]; then
        echo "==> TEST SKIPPED: juju bundle_with_exposed_endpoints"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_bundle_with_exposed_endpoints" "$@"
    )
}
