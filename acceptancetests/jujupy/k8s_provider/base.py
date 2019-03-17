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
import subprocess
from pprint import pformat
from enum import Enum

from jujupy.utility import (
    ensure_dir,
    until_timeout,
)


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

    k8s_cloud_name = 'k8cloud'

    timeout = None
    juju_home = None
    kube_home = None
    kubectl_path = None
    kube_config_path = None

    default_storage_class_name = None

    def _ensure_cluster_stack(self):
        """ensures or checks if stack/infrastructure is ready to use.
        - ensures kubectl/apiserver is functioning.
        """
        raise NotImplemented()

    def _ensure_cluster_config(self):
        """ensures the cluster is correctly configured and ready to use.
        - ensures the cluster is ready to deploy workloads.
        """
        raise NotImplemented()

    def _ensure_kube_dir(self):
        """ensures $KUBECONFIG/.kube dir setup correctly:
        - kubectl bin
        - config
        """
        raise NotImplemented()

    def _node_address_getter(self, node):
        """filters node addresses to get the correct accessible address.
        """
        raise NotImplemented()

    def __init__(self, client, timeout=1800):
        self.client = client
        self.timeout = timeout
        self.juju_home = self.client.env.juju_home

        self.kubectl_path = os.path.join(self.juju_home, 'kubectl')
        self.kube_home = os.path.join(self.juju_home, '.kube')
        # ensure kube home
        ensure_dir(self.kube_home)
        self.kube_config_path = os.path.join(self.kube_home, 'config')

        # ensure kube config env var
        os.environ[KUBE_CONFIG_PATH_ENV_VAR] = self.kube_config_path

        self._ensure_cluster_stack()
        self._ensure_kube_dir()
        self.check_cluster_healthy()
        self._ensure_cluster_config()
        self._add_k8s()

    def add_model(self, model_name):
        # returns the newly added CAAS model.
        return self.client.add_model(env=self.client.env.clone(model_name), cloud_region=self.cloud_name)

    def _add_k8s(self):
        self.client.controller_juju(
            'add-k8s',
            (self.cloud_name, '--controller', self.client.env.controller.name)
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

    def sh(self, *args):
        return subprocess.check_output(
            args,
            stderr=subprocess.STDOUT,
        ).decode('UTF-8').strip()

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
        return self._node_address_getter(nodes[0])
