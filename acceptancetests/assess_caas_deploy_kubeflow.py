#!/usr/bin/env python3
""" Test caas k8s cluster bootstrap then deploy kubeflow

    1. spin up k8s cluster and assert the cluster is `healthy`;
    2. deploy kubeflow bundle to caas model;
    3. wait for all workload stabalized;
    4. run kubeflow tests;
"""

from __future__ import print_function

import argparse
import json
import logging
import contextlib
import os
import shutil
import time
import sys
import textwrap
import subprocess
from pprint import pformat
from time import sleep

from deploy_stack import BootstrapManager
from utility import (
    JujuAssertionError, add_basic_testing_arguments, configure_logging,
)
from jujupy.k8s_provider import K8sProviderType, providers
from jujupy.utility import until_timeout


__metaclass__ = type
log = logging.getLogger("assess_caas_kubeflow_deployment")

KUBEFLOW_REPO_NAME = "bundle-kubeflow"
KUBEFLOW_REPO_URI = f"https://github.com/juju-solutions/{KUBEFLOW_REPO_NAME}.git"
KUBEFLOW_DIR = os.path.join(os.getcwd(), KUBEFLOW_REPO_NAME)
CHARM_INTERFACES_DIR = KUBEFLOW_DIR
OSM_REPO_NAME = "canonical-osm"
OSM_REPO_URI = f"git://git.launchpad.net/{OSM_REPO_NAME}"

bundle_info = {
    'full': {
        'file_name': 'bundle.yaml',
        'uri': 'kubeflow',
    },
    'lite': {
        'file_name': 'bundle-lite.yaml',
        'uri': 'kubeflow-lite',
    },
    'edge': {
        'file_name': 'bundle-edge.yaml',
        'uri': 'kubeflow-edge',
    },
}


def run(*args, silence=False):
    if silence:
        return subprocess.check_call(
            list(args),
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    return subprocess.check_call(list(args))


def retry(is_ok, do, timeout=300, should_raise=False):
    for _ in until_timeout(timeout):
        if is_ok():
            try:
                return do()
            except Exception as e:
                if should_raise:
                    raise e
        sleep(3)
    raise JujuAssertionError('retry timeout after %s' % timeout)


@contextlib.contextmanager
def jump_dir(path):
    old_path = os.getcwd()
    os.chdir(path)
    try:
        yield
    finally:
        os.chdir(old_path)


def k8s_resource_exists(caas_client, resource):
    try:
        run(*(caas_client._kubectl_bin + ('get', resource)), silence=True)
        return True
    except subprocess.CalledProcessError:
        return False


def application_exists(k8s_model, application):
    try:
        k8s_model.juju('show-application', (application, ))
        return True
    except subprocess.CalledProcessError:
        return False


def deploy_kubeflow(caas_client, k8s_model, bundle, build):
    start = time.time()

    caas_client.kubectl('label', 'namespace', k8s_model.model_name, 'istio-injection=enabled')
    if build:
        with jump_dir(KUBEFLOW_DIR):
            k8s_model.juju(
                'bundle',
                (
                    'deploy',
                    '--bundle', f'{KUBEFLOW_DIR}/{bundle_info[bundle]["file_name"]}',
                    '--build',
                ),
                include_e=False,
            )
    else:
        k8s_model.deploy(
            charm=bundle_info[bundle]['uri'],
            channel="stable",
        )

    if application_exists(k8s_model, 'istio-ingressgateway'):
        retry(
            lambda: True,
            lambda: print(
                'patching role/istio-ingressgateway-operator ->', caas_client.kubectl(
                    'patch',
                    'role/istio-ingressgateway-operator',
                    '-n', k8s_model.model_name,
                    '-p',
                    json.dumps(
                        {
                            "apiVersion": "rbac.authorization.k8s.io/v1",
                            "kind": "Role",
                            "metadata": {"name": "istio-ingressgateway-operator"},
                            "rules": [{"apiGroups": ["*"], "resources": ["*"], "verbs": ["*"]}],
                        }
                    ),
                )
            ),
            timeout=60,
        )

    k8s_model.wait_for_workloads(timeout=60*30)

    if application_exists(k8s_model, 'pipelines-api'):
        retry(
            lambda: True,
            lambda: print(
                'applying service/ml-pipeline ->', caas_client.kubectl_apply(
                    json.dumps(
                        {
                            'apiVersion': 'v1',
                            'kind': 'Service',
                            'metadata': {'labels': {'juju-app': 'pipelines-api'}, 'name': 'ml-pipeline'},
                            'spec': {
                                'ports': [
                                    {'name': 'grpc', 'port': 8887, 'protocol': 'TCP', 'targetPort': 8887},
                                    {'name': 'http', 'port': 8888, 'protocol': 'TCP', 'targetPort': 8888},
                                ],
                                'selector': {'juju-app': 'pipelines-api'},
                                'type': 'ClusterIP',
                            },
                        },
                    ),
                    namespace=k8s_model.model_name,
                )
            ),
            timeout=60,
            should_raise=True,
        )

    caas_client.kubectl(
        "wait",
        f"--namespace={k8s_model.model_name}",
        "--for=condition=Ready",
        "pod",
        "--timeout=30m",
        "--all",
    )

    pub_addr = get_pub_addr(caas_client)
    password = "foobar"
    if application_exists(k8s_model, 'dex-auth'):
        log.info("configuring dex-auth application")
        k8s_model.set_config('dex-auth', {'public-url': f'http://{pub_addr}:80'})
        k8s_model.set_config('dex-auth', {'static-password': f'{password}'})

    if application_exists(k8s_model, 'oidc-gatekeeper'):
        log.info("configuring oidc-gatekeeper application")
        k8s_model.set_config('oidc-gatekeeper', {'public-url': f'http://{pub_addr}:80'})

    log.info("Waiting for Kubeflow to become ready")

    k8s_model.wait_for_workloads(timeout=60*10)
    caas_client.kubectl(
        "wait",
        "--for=condition=available",
        "-n",
        k8s_model.model_name,
        "deployment",
        "--all",
        "--timeout=10m",
    )

    log.info(
        f'\nCongratulations, Kubeflow is now available. Took {int(time.time() - start)} seconds.',
    )
    kubeflow_info(k8s_model.controller_name, k8s_model.model_name, pub_addr)


def kubeflow_info(controller_name: str, model_name: str, pub_addr: str):
    """Displays info about the deploy Kubeflow instance."""

    print(
        textwrap.dedent(
            f"""

        The central dashboard is available at http://{pub_addr}/

        To display the login username, run:

            juju config dex-auth static-username

        If you manually set the login password, run this command to display it:

            juju config dex-auth static-password

        Otherwise, the login password was autogenerated, and can be viewed with:

            juju run --app dex-auth --operator cat /run/password

        To tear down Kubeflow, run this command:

            # Run `juju destroy-model --help` for a full listing of options,
            # such as how to release storage instead of destroying it.
            juju destroy-model {controller_name}:{model_name} --destroy-storage

        For more information, see documentation at:

        https://github.com/juju-solutions/bundle-kubeflow/blob/master/README.md

        """
        )
    )


def get_pub_addr(caas_client):
    """Gets the hostname that Ambassador will respond to."""

    for charm in ('ambassador', 'istio-ingressgateway'):
        # Check if we've manually set a hostname on the ingress
        try:
            output = caas_client.kubectl('get', f'ingress/{charm}', '-ojson')
            return json.loads(output)['spec']['rules'][0]['host']
        except (KeyError, subprocess.CalledProcessError):
            pass

        # Check if juju expose has created an ELB or similar
        try:
            output = caas_client.kubectl('get', f'svc/{charm}', '-ojson')
            return json.loads(output)['status']['loadBalancer']['ingress'][0]['hostname']
        except (KeyError, subprocess.CalledProcessError):
            pass

        # Otherwise, see if we've set up metallb with a custom service
        try:
            output = caas_client.kubectl('get', f'svc/{charm}', '-ojson')
            pub_ip = json.loads(output)['status']['loadBalancer']['ingress'][0]['ip']
            return '%s.xip.io' % pub_ip
        except (KeyError, subprocess.CalledProcessError):
            pass

    # If all else fails, just use localhost
    return 'localhost'


def prepare(caas_client, caas_provider, build):
    if caas_provider == K8sProviderType.MICROK8S.name:
        caas_client.enable_microk8s_addons(
            [
                "dns", "storage", "dashboard", "ingress", "metallb:10.64.140.43-10.64.140.49",
                # "rbac",  # TODO: enable `RBAC`?
            ],
        )
        caas_client.kubectl(
            "wait", "--for=condition=available",
            "-nkube-system", "deployment/coredns", "deployment/hostpath-provisioner", "--timeout=10m",
        )

    for dep in [
        "charm",
        "juju-helpers",
        "juju-wait",
    ]:
        if shutil.which(dep):
            continue
        caas_client.sh('sudo', 'snap', 'install', dep, '--classic')

    caas_client.sh('sudo', 'apt', 'update')
    caas_client.sh('sudo', 'apt', 'install', '-y', 'libssl-dev', 'python3-setuptools')

    caas_client.sh('rm', '-rf', f'{KUBEFLOW_DIR}')
    caas_client.sh('git', 'clone', KUBEFLOW_REPO_URI, KUBEFLOW_DIR)
    caas_client.sh(
        'pip3', 'install',
        '-r', f'{KUBEFLOW_DIR}/requirements.txt',
        '-r', f'{KUBEFLOW_DIR}/test-requirements.txt',
    )

    if build:
        # When we're building the charms locally instead of deploying them from the charm store,
        # we'll need to include a particular mysql interface.
        # - https://github.com/canonical/bundle-kubeflow/issues/291
        os.environ['CHARM_INTERFACES_DIR'] = CHARM_INTERFACES_DIR
        caas_client.sh('git', 'clone', OSM_REPO_URI, f'{KUBEFLOW_DIR}/{OSM_REPO_NAME}')
        caas_client.sh(
            'cp', '-r',
            f'{KUBEFLOW_DIR}/{OSM_REPO_NAME}/charms/interfaces/juju-relation-mysql',
            f'{CHARM_INTERFACES_DIR}/mysql',
        )


def run_test(caas_provider, k8s_model, bundle):
    if caas_provider != K8sProviderType.MICROK8S.name:
        # tests/run.sh only works for microk8s.
        log.info("%s/tests/run.sh skipped for %s k8s provider", KUBEFLOW_DIR, caas_provider)
        return
    # inject `JUJU_DATA` for running tests.
    os.environ['JUJU_DATA'] = k8s_model.env.juju_home

    with jump_dir(KUBEFLOW_DIR):
        run("sg", "microk8s", "-c", f"{KUBEFLOW_DIR}/tests/run.sh -m {bundle}")


def assess_caas_kubeflow_deployment(caas_client, caas_provider, bundle, build=False):
    if not caas_client.check_cluster_healthy(timeout=60):
        raise JujuAssertionError('k8s cluster is not healthy because kubectl is not accessible')

    model_name = caas_client.client.env.controller.name + '-test-caas-model'
    k8s_model = caas_client.add_model(model_name)

    def success_hook():
        log.info(caas_client.kubectl('get', 'all,pv,pvc,ing', '--all-namespaces', '-o', 'wide'))
        log.info(caas_client.kubectl('get', 'sa,roles,clusterroles,rolebindings,clusterrolebindings', '-oyaml', '-A'))

    def fail_hook():
        success_hook()
        ns_dumps = caas_client.kubectl('get', 'all,pv,pvc,ing', '-n', model_name, '-o', 'json')
        log.info('all resources in namespace %s -> %s', model_name, pformat(json.loads(ns_dumps)))

        log.info(
            'describing pods in %s ->\n%s',
            model_name, caas_client.kubectl('describe', 'pods', f'-n{model_name}'),
        )
        caas_client.ensure_cleanup()

    try:
        prepare(caas_client, caas_provider, build)
        deploy_kubeflow(caas_client, k8s_model, bundle, build)
        log.info("sleeping for 30 seconds to let everything start up")
        sleep(30)

        run_test(caas_provider, k8s_model, bundle)
        k8s_model.juju(k8s_model._show_status, ('--format', 'tabular'))
        success_hook()
    except:  # noqa: E722
        # run cleanup steps then raise.
        fail_hook()
        raise


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="CAAS kubeflow CI test")
    parser.add_argument(
        '--caas-image-repo', action='store', default='jujuqabot',
        help='CAAS operator docker image repo to use.'
    )
    parser.add_argument(
        '--caas-provider', action='store', default='MICROK8S',
        choices=K8sProviderType.keys(),
        help='Specify K8s cloud provider to use for CAAS tests.'
    )
    parser.add_argument(
        '--bundle', action='store', default='edge',
        choices=bundle_info.keys(),
        help='Specify the kubeflow bundle version to deploy.'
    )
    parser.add_argument(
        '--build',
        action='store_true',
        help='Build local kubeflow charms and deploy local bundle or deploy bundle in charmstore.'
    )
    parser.add_argument(
        '--k8s-controller',
        action='store_true',
        help='Bootstrap to k8s cluster or not.'
    )
    parser.add_argument(
        '--enable-rbac',
        action='store_true',
        help='Deploy kubeflow with RBAC enabled.'
    )

    add_basic_testing_arguments(parser, existing=False)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    k8s_provider = providers[args.caas_provider]
    bs_manager = BootstrapManager.from_args(args)

    with k8s_provider(bs_manager, cluster_name=args.temp_env_name).substrate_context() as caas_client:
        # add-k8s --local
        if args.k8s_controller and args.caas_provider != K8sProviderType.MICROK8S.name:
            # microk8s is built-in cloud, no need run add-k8s for bootstrapping.
            caas_client.add_k8s(True)
        with bs_manager.existing_booted_context(
            args.upload_tools,
            caas_image_repo=args.caas_image_repo,
        ):
            if not args.k8s_controller:
                # add-k8s to controller
                caas_client.add_k8s(False)
            assess_caas_kubeflow_deployment(caas_client, args.caas_provider, args.bundle, args.build)
        return 0


if __name__ == '__main__':
    sys.exit(main())
