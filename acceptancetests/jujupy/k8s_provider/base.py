# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2019 Canonical Ltd.
#
# This program is free software: you can redistribute it and/or modify it
# under the terms of the Lesser GNU General Public License version 3, as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful, but WITHOUT
# ANY WARRANTY; without even the implied warranties of MERCHANTABILITY,
# SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR PURPOSE.  See the Lesser
# GNU General Public License for more details.
#
# You should have received a copy of the Lesser GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

# Functionality for handling installed or other juju binaries
# (including paths etc.)


from __future__ import print_function

import os
from time import sleep
import json
import logging
import shutil
import subprocess
from pprint import pformat
from enum import Enum
from contextlib import contextmanager

from jujupy.utility import (
    ensure_dir,
    until_timeout,
)
from jujupy.client import temp_bootstrap_env


logger = logging.getLogger(__name__)


class ProviderNotAvailable(AttributeError):
    """Raised when a provider is requested that isn't registered"""
    ...


class ProviderNotValid(ValueError):
    """Raised when a provider is not correctly named.
    always use K8sProviderType to name a new provider.
    """
    ...


class K8sProviderType(Enum):
    MICROK8S = 1
    K8S_CORE = 2
    GKE = 3
    AKS = 4

    @classmethod
    def keys(cls):
        return list(cls.__members__.keys())

    @classmethod
    def values(cls):
        return list(cls.__members__.values())


# caas `add-k8s` did not implement parsing kube config path via flag,
# so parse it via env var ->  https://github.com/kubernetes/client-go/blob/master/tools/clientcmd/loader.go#L44
KUBE_CONFIG_PATH_ENV_VAR = 'KUBECONFIG'


class Base(object):

    name = None

    cloud_name = 'k8cloud'

    timeout = None
    juju_home = None
    kube_home = None
    kubectl_path = None
    kube_config_path = None

    default_storage_class_name = None
    kubeconfig_cluster_name = None

    def _ensure_cluster_stack(self):
        """ensures or checks if stack/infrastructure is ready to use.
        - ensures kubectl/apiserver is functioning.
        """
        raise NotImplementedError()

    def _ensure_cluster_config(self):
        """ensures the cluster is correctly configured and ready to use.
        - ensures the cluster is ready to deploy workloads.
        """
        raise NotImplementedError()

    def _ensure_kube_dir(self):
        """ensures $KUBECONFIG/.kube dir setup correctly:
        - kubectl bin
        - config
        """
        raise NotImplementedError()

    def _node_address_getter(self, node):
        """filters node addresses to get the correct accessible address.
        """
        raise NotImplementedError()

    def _tear_down_substrate(self):
        """tear down substrate cloud - k8s cluster.
        """
        raise NotImplementedError()

    def __init__(self, bs_manager, timeout=1800):
        self.client = bs_manager.client
        self.bs_manager = bs_manager
        # register cleanup_hook
        bs_manager.cleanup_hook = self.ensure_cleanup

        self.timeout = timeout
        old_environment = bs_manager.client.env.environment

        bs_manager.client.env.environment = bs_manager.temp_env_name
        with temp_bootstrap_env(bs_manager.client.env.juju_home, bs_manager.client) as tm_h:
            self.client.env.juju_home = tm_h
            self.refresh_home(self.client)
        bs_manager.client.env.environment = old_environment

    def refresh_home(self, client):
        self.juju_home = client.env.juju_home

        self.kubectl_path = os.path.join(self.juju_home, 'kubectl')
        self.kube_home = os.path.join(self.juju_home, '.kube')
        # ensure kube home
        ensure_dir(self.kube_home)
        self.kube_config_path = os.path.join(self.kube_home, 'config')

        # ensure kube config env var
        os.environ[KUBE_CONFIG_PATH_ENV_VAR] = self.kube_config_path

    @contextmanager
    def substrate_context(self):
        try:
            self._ensure_cluster_stack()
            self._ensure_kube_dir()
            self.check_cluster_healthy()
            self._ensure_cluster_config()

            yield self
        finally:
            # tear down cluster.
            self._tear_down_substrate()

    def add_model(self, model_name):
        # returns the newly added CAAS model.
        return self.client.add_model(env=self.client.env.clone(model_name), cloud_region=self.cloud_name)

    def add_k8s(self, is_local=False, juju_home=None):
        args = (
            self.cloud_name,
        )
        juju_home = juju_home or self.client.env.juju_home
        if is_local:
            args += (
                '--local',
                '--cluster-name', self.kubeconfig_cluster_name,
            )
            # use this local cloud to bootstrap later.
            self.bs_manager.client.env.set_cloud_name(self.cloud_name)
        else:
            args += (
                '--controller', self.client.env.controller.name,
            )
        logger.info("running add-k8s %s", args)
        self.client._backend.juju(
            'add-k8s', args,
            used_feature_flags=self.client.used_feature_flags, juju_home=juju_home,
        )
        logger.debug('added caas cloud, now all clouds are -> \n%s', self.client.list_clouds(format='yaml'))

    def check_cluster_healthy(self, timeout=0):
        def check():
            try:
                cluster_info = self.kubectl('cluster-info')
                logger.debug('cluster_info -> \n%s', cluster_info)
                nodes_info = self.kubectl('get', 'nodes')
                logger.debug('nodes_info -> \n%s', pformat(nodes_info))
                return True
            except subprocess.CalledProcessError as e:
                logger.error('error -> %s', e)
                return False
        for remaining in until_timeout(timeout):
            if check():
                return True
            sleep(3)
        return False

    def kubectl(self, *args):
        return self.sh(*(self._kubectl_bin + args))

    def get_configmap(self, namespace, cm_name):
        return json.loads(
            self.kubectl('get', '-n', namespace, 'cm', cm_name, '-o', 'json')
        )

    def patch_configmap(self, namespace, cm_name, key, value):
        cm = self.get_configmap(namespace, cm_name)
        data = cm.get('data', {})
        data[key] = value if isinstance(value, str) else str(value)
        cm['data'] = data
        self.kubectl_apply(json.dumps(cm))

    def sh(self, *args):
        args = [str(arg) for arg in args]
        logger.debug('sh -> %s', ' '.join(args))
        return subprocess.check_output(
            # cmd should be a list of str.
            args,
            stderr=subprocess.STDOUT,
        ).decode('UTF-8').strip()

    def _ensure_kubectl_bin(self):
        kubectl_bin_path = shutil.which('kubectl')
        if kubectl_bin_path is not None:
            self.kubectl_path = kubectl_bin_path
        else:
            self.sh(
                'curl', 'https://storage.googleapis.com/kubernetes-release/release/v1.14.0/bin/linux/amd64/kubectl',
                '-o', self.kubectl_path
            )
            os.chmod(self.kubectl_path, 0o774)

    @property
    def _kubectl_bin(self):
        return (self.kubectl_path, '--kubeconfig', self.kube_config_path,)

    def kubectl_apply(self, stdin):
        with subprocess.Popen(('echo', stdin), stdout=subprocess.PIPE) as echo:
            o = subprocess.check_output(
                self._kubectl_bin + ('apply', '-f', '-'),
                stdin=echo.stdout,
            ).decode('UTF-8').strip()
            logger.debug(o)

    def get_external_hostname(self):
        # assume here always use single node cdk core or microk8s
        return '{}.xip.io'.format(self.get_first_worker_ip())

    def get_first_worker_ip(self):
        nodes = json.loads(
            self.kubectl('get', 'nodes', '-o', 'json')
        )
        logger.debug("trying to get first worker node IP, nodes are -> \n%s", pformat(nodes))
        return self._node_address_getter(nodes['items'][0])

    def ensure_cleanup(self):
        controller_uuid = self.client.get_controller_uuid()
        namespaces = json.loads(
            self.kubectl('get', 'ns', '-o', 'json')
        )
        logger.info("all namespaces: %s", namespaces)
        juju_owned_ns = [
            ns['metadata']['name']
            for ns in namespaces['items']
            if ns['metadata'].get('annotations', {}).get('juju.io/controller') == controller_uuid
        ]
        logger.info("juju owned namespaces: %s", juju_owned_ns)
        for ns_name in juju_owned_ns:
            logger.info("deleting namespace: %s", ns_name)
            try:
                self.kubectl('delete', 'ns', ns_name)
            except Exception as e:
                logger.warn(e)
