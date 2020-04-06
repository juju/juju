# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2020 Canonical Ltd.
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

import logging
import os

from azure.common.client_factory import get_client_from_json_dict
from azure.mgmt import containerservice
from msrestazure import azure_exceptions

from .base import (
    Base,
    K8sProviderType,
)
from .factory import register_provider


logger = logging.getLogger(__name__)


@register_provider
class AKS(Base):

    name = K8sProviderType.AKS

    ake_cluster_name = None

    cfg = None
    credential = None
    location = None
    resource_group = None

    client = None
    parameters = None

    def __init__(self, bs_manager, timeout=1800):
        super().__init__(bs_manager, timeout)

        self.ake_cluster_name = self.client.env.controller.name  # use controller name for cluster name
        self.default_storage_class_name = ''
        self.__init_client(bs_manager.client.env)

    def __init_client(self, env):
        self.cfg = {k: v for k, v in env._config.items()}
        self.credential = {
            'clientId': self.cfg['application-id'],
            'clientSecret': self.cfg['application-password'],
            'subscriptionId': self.cfg['subscription-id'],
            'tenantId': self.cfg['tenant-id'],
            'activeDirectoryEndpointUrl': 'https://login.microsoftonline.com',
            'resourceManagerEndpointUrl': 'https://management.azure.com/',
            'activeDirectoryGraphResourceId': 'https://graph.windows.net/',
            'sqlManagementEndpointUrl': 'https://management.core.windows.net:8443/',
            'galleryEndpointUrl': 'https://gallery.azure.com/',
            'managementEndpointUrl': 'https://management.core.windows.net/',
        }
        self.location = self.cfg['location']
        self.resource_group = self.cfg['resource-group']
        self.client = get_client_from_json_dict(containerservice.ContainerServiceClient, self.credential)
        self.parameters = self.get_parameters(
            location=self.location,
            client_id=self.credential['clientId'],
            client_secret=self.credential['clientSecret']
        )

        # list all running clusters
        logger.info(
            'running aks clusters: \n\t- %s',
            '\n\t- '.join([c.name for c in self.list_clusters(self.resource_group)])
        )

    def _ensure_cluster_stack(self):
        self.provision_aks()

    def _tear_down_substrate(self):
        print("Deleting the AKS instance {0}".format(self.ake_cluster_name))
        try:
            poller = self.client.managed_clusters.delete(self.resource_group, self.ake_cluster_name)
            r = get_poller_result(poller)
            if r is not None:
                logger.info("cluster has been deleted -> %s", r.as_dict())
        except Exception as e:
            print(e)
            raise

    def get_parameters(
        self, location, client_id, client_secret,
        kubernetes_version=None,
        pub_ssh_key_path=os.path.expanduser('~/.ssh/id_rsa.pub'),
    ):
        m = self.client.managed_clusters.models

        service_principal_profile = m.ManagedClusterServicePrincipalProfile(
            client_id=client_id, secret=client_secret)

        agentpool_default = m.ManagedClusterAgentPoolProfile(
            name='default',
            count=2,
            vm_size='Standard_D2_v2',
        )

        with open(pub_ssh_key_path, 'r') as pub_ssh_file_fd:
            pub_ssh_file_fd = pub_ssh_file_fd.read()
        ssh_ = self.client.managed_clusters.models.ContainerServiceSshConfiguration(
            public_keys=[m.ContainerServiceSshPublicKey(key_data=pub_ssh_file_fd)]
        )
        linux_profile = m.ContainerServiceLinuxProfile(
            admin_username='azureuser', ssh=ssh_)

        return m.ManagedCluster(
            location=location,
            dns_prefix=self.ake_cluster_name,  # use cluster name as dns prefix.
            kubernetes_version=kubernetes_version or self.get_k8s_version(location),
            service_principal_profile=service_principal_profile,
            agent_pool_profiles=[agentpool_default],
            linux_profile=linux_profile,
            enable_rbac=True,
        )

    def list_clusters(self, resource_group):
        return self.client.managed_clusters.list_by_resource_group(resource_group)

    def _ensure_kube_dir(self):
        ...

    def _ensure_cluster_config(self):
        access_profile = self.client.managed_clusters.get_access_profile(
            resource_group_name=self.resource_group,
            resource_name=self.ake_cluster_name,
            role_name="clusterUser",
        )
        kubeconfig_content = access_profile.kube_config.decode('utf-8')

        with open(self.kube_config_path, 'w') as f:
            logger.debug('writing kubeconfig to %s\n%s', self.kube_config_path, kubeconfig_content)
            f.write(kubeconfig_content)

    def _node_address_getter(self, node):
        return [addr['address'] for addr in node['status']['addresses'] if addr['type'] == 'ExternalIP'][0]

    def _get_cluster(self, name):
        return self.client.managed_clusters.get(self.resource_group, name)

    def provision_aks(self):
        # do pre cleanup;
        self._tear_down_substrate()

        # provision cluster.
        logger.info('creating cluster -> %s', self.ake_cluster_name)
        try:
            poller = self.client.managed_clusters.create_or_update(
                self.resource_group,
                self.ake_cluster_name,
                self.parameters,
            )
            result = get_poller_result(poller)
            logger.info(
                "cluster %s has been successfully provisioned -> \n%s",
                self.ake_cluster_name, result.as_dict(),
            )
        except azure_exceptions.CloudError as e:
            print('Error attempting to create the AKS instance.', e.message)
            raise e

    def get_k8s_version(self, location):
        orchestrators = self.client.container_services.list_orchestrators(
            location, resource_type='managedClusters',
        )
        if len(orchestrators) == 0:
            return ""
        for o in orchestrators:
            if o.default:
                return o.orchestrator_version
        return orchestrators[0].orchestrator_version


def get_poller_result(poller, wait=5):
    try:
        delay = wait
        while not poller.done():
            logger.info("current status: %s, waiting for %s sec", poller.status(), delay)
            poller.wait(timeout=delay)
        return poller.result()
    except Exception as e:
        print(str(e))
        raise e
