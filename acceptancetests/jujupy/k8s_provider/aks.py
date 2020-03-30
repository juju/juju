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
import yaml
import json
from time import sleep
import shutil
import tempfile
import os

from azure.common.client_factory import get_client_from_json_dict
from azure.mgmt import containerservice
from msrestazure import azure_exceptions
from jujupy.utility import until_timeout

from .base import (
    Base,
    K8sProviderType,
)
from .factory import register_provider


logger = logging.getLogger(__name__)
# CLUSTER_STATUS = container_v1.enums.Cluster.Status


def is_not_found(e):
    # e: azure_exceptions.CloudError
    # TODO!!!!
    return True


@register_provider
class AKS(Base):

    name = K8sProviderType.AKS

    driver = None
    ake_cluster_name = None

    default_params = None
    resource_group = None

    def __init__(self, bs_manager, timeout=1800):
        super().__init__(bs_manager, timeout)

        self.ake_cluster_name = self.client.env.controller.name  # use controller name for cluster name
        self.default_storage_class_name = ''
        self.__init_driver(bs_manager.client.env)

    def __init_driver(self, env):
        # zone = env.get_host_cloud_region()[2] + '-b'
        cfg = env._config
        self.resource_group = cfg['resource-group']

        cred = {
            'clientId': cfg['application-id'],
            'clientSecret': cfg['application-password'],
            'subscriptionId': cfg['subscription-id'],
            'tenantId': cfg['tenant-id'],
            'activeDirectoryEndpointUrl': 'https://login.microsoftonline.com',
            'resourceManagerEndpointUrl': 'https://management.azure.com/',
            'activeDirectoryGraphResourceId': 'https://graph.windows.net/',
            'sqlManagementEndpointUrl': 'https://management.core.windows.net:8443/',
            'galleryEndpointUrl': 'https://gallery.azure.com/',
            'managementEndpointUrl': 'https://management.core.windows.net/',
        }

        self.driver = get_client_from_json_dict(containerservice.ContainerServiceClient, cred)

        # list all running clusters
        running_clusters = self.driver.list_clusters()
        logger.info('running aks clusters: %s', running_clusters)

    def _ensure_cluster_stack(self):
        self.provision_aks()

    def _tear_down_substrate(self):
        try:
            '''
  Deletes the specified managed container service (AKS) in the specified subscription and resource group.

  :return: True
  '''
        print("Deleting the AKS instance {0}".format(self.ake_cluster_name))
        try:
            poller = self.driver.managed_clusters.delete(self.resource_group, self.ake_cluster_name)
            self.get_poller_result(poller)
            return True
        # except exceptions.NotFound:
        except Exception as e:
            print(e)
            raise

    def get_parameters(self):
        return self.driver.managed_clusters.models.ManagedCluster(
            location=self.location,
            dns_prefix=self.dns_prefix,
            kubernetes_version=self.kubernetes_version,
            tags=self.tags,
            service_principal_profile=service_principal_profile,
            agent_pool_profiles=agentpools,
            linux_profile=self.create_linux_profile_instance(self.linux_profile),
            enable_rbac=self.enable_rbac,
            network_profile=self.create_network_profile_instance(self.network_profile),
            aad_profile=self.create_aad_profile_instance(self.aad_profile),
            addon_profiles=self.create_addon_profile_instance(self.addon)
        )

    def list_clusters(self):
        return list(self.driver.managed_clusters.list())

    def _ensure_kube_dir(self):
        ...

    def _ensure_cluster_config(self):
        access_profile = self.driver.managed_clusters.get_access_profile(
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
        return self.driver.managed_clusters.get(self.resource_group, name)

    def provision_aks(self):
        def log_remaining(remaining, msg=''):
            sleep(3)
            if remaining % 30 == 0:
                msg += ' timeout in %ss...' % remaining
                logger.info(msg)

        # do pre cleanup;
        self._tear_down_substrate()
        for remaining in until_timeout(600):
            # wait for the old cluster to be deleted.
            try:
                self._get_cluster(self.ake_cluster_name)
            except exceptions.NotFound:
                break
            finally:
                log_remaining(remaining)

        # provision cluster.
        logger.info('creating cluster -> %s', self.ake_cluster_name)
        params = {'location': 'westus2',
                  'orchestrator_profile': {'orchestrator_type': 'Kubernetes'},
                  'agent_pool_profiles': [{'name': 'default',
                                           'count': 2,
                                           'vm_size': 'Standard_D2_v2',
                                           'dns_prefix': 'acs1-rg1-e240e5master'}],
                  'master_profile': {'count': 1,
                                     'vm_size': 'Standard_D2_v2',
                                     'dns_prefix': 'acs1-rg1-e240e5agent'},
                  'linux_profile': {'ssh': {'public_keys': [{'key_data': ''}]},
                                    'adminUsername': 'azureuser'}}
        # params = self.get_parameters()
        r = self.driver.managed_clusters.create_or_update(self.resource_group, self.ake_cluster_name, params)

        try:
            poller = self.driver.managed_clusters.create_or_update(
                self.resource_group, self.name, params)
            response = self.get_poller_result(poller)
            response.kube_config = self.get_aks_kubeconfig()
            return create_aks_dict(response)
        except azure_exceptions.CloudError as e:
            print('Error attempting to create the AKS instance.', e.message)
            raise e

        logger.info('created cluster -> %s', r)
        # wait until cluster fully provisioned.
        logger.info('waiting for cluster fully provisioned.')
        for remaining in until_timeout(600):
            try:
                cluster = self._get_cluster(self.ake_cluster_name)
                if cluster.status == CLUSTER_STATUS.RUNNING:
                    return
            except Exception as e:  # noqa
                logger.info(e)
            finally:
                log_remaining(remaining)

    def get_poller_result(self, poller, wait=5):
        try:
            delay = wait
            while not poller.done():
                print("Waiting for {0} sec".format(delay))
                poller.wait(timeout=delay)
            return poller.result()
        except Exception as e:
            print(str(e))
            raise e

    def create_linux_profile_instance(self, linuxprofile):
        m = self.driver.managed_clusters.models
        return m.ContainerServiceLinuxProfile(
            admin_username=linuxprofile['admin_username'],
            ssh=m.ContainerServiceSshConfiguration(public_keys=[
                m.ContainerServiceSshPublicKey(key_data=str(linuxprofile['ssh_key']))])
        )


def create_agent_pool_profiles_dict(agentpoolprofiles):
    return [dict(
        count=profile.count,
        vm_size=profile.vm_size,
        name=profile.name,
        os_disk_size_gb=profile.os_disk_size_gb,
        storage_profile=profile.storage_profile,
        vnet_subnet_id=profile.vnet_subnet_id,
        os_type=profile.os_type
    ) for profile in agentpoolprofiles] if agentpoolprofiles else None


def create_linux_profile_dict(linuxprofile):
    return dict(
        ssh_key=linuxprofile.ssh.public_keys[0].key_data,
        admin_username=linuxprofile.admin_username
    )


def create_aks_dict(aks):
    return dict(
        id=aks.id,
        name=aks.name,
        location=aks.location,
        dns_prefix=aks.dns_prefix,
        kubernetes_version=aks.kubernetes_version,
        tags=aks.tags,
        linux_profile=create_linux_profile_dict(aks.linux_profile),
        service_principal_profile=create_service_principal_profile_dict(
            aks.service_principal_profile),
        provisioning_state=aks.provisioning_state,
        agent_pool_profiles=create_agent_pool_profiles_dict(
            aks.agent_pool_profiles),
        type=aks.type,
        kube_config=aks.kube_config,
        enable_rbac=aks.enable_rbac,
        network_profile=create_network_profiles_dict(aks.network_profile),
        aad_profile=create_aad_profiles_dict(aks.aad_profile),
        addon=create_addon_dict(aks.addon_profiles),
        fqdn=aks.fqdn,
        node_resource_group=aks.node_resource_group
    )


# class ClusterConfig(object):

#     def __init__(self, project_id, cluster):
#         self.cluster_name = cluster.name
#         self.zone_id = cluster.zone
#         self.project_id = project_id
#         self.server = 'https://' + cluster.endpoint
#         self.ca_data = cluster.master_auth.cluster_ca_certificate
#         self.client_key_data = cluster.master_auth.client_key
#         self.client_cert_data = cluster.master_auth.client_certificate
#         self.context_name = 'gke_{project_id}_{zone_id}_{cluster_name}'.format(
#             project_id=self.project_id,
#             zone_id=self.zone_id,
#             cluster_name=self.cluster_name,
#         )

#     def user(self, auth_provider='gcp'):
#         if auth_provider is None:
#             return {
#                 'name': self.context_name,
#                 'user': {
#                     'client-certificate-data': self.client_cert_data,
#                     'client-key-data': self.client_key_data
#                 },
#             }
#         # TODO(ycliuhw): remove gcloud dependency once 'google-cloud-container' supports defining master-auth type.
#         gcloud_bin_path = shutil.which('gcloud')
#         if gcloud_bin_path is None:
#             raise AssertionError("gcloud bin is required!")
#         return {
#             'name': self.context_name,
#             'user': {
#                 'auth-provider': {
#                     'name': auth_provider,
#                     'config': {
#                         # Command for gcloud credential helper
#                         'cmd-path': gcloud_bin_path,
#                         # Args for gcloud credential helper
#                         'cmd-args': 'config config-helper --format=json',
#                         # JSONpath to the field that is the raw access token
#                         'token-key': '{.credential.access_token}',
#                         # JSONpath to the field that is the expiration timestamp
#                         'expiry-key': '{.credential.token_expiry}',
#                     }
#                 },
#             },
#         }

#     @property
#     def cluster(self):
#         return {
#             'name': self.context_name,
#             'cluster': {
#                 'server': self.server,
#                 'certificate-authority-data': self.ca_data,
#             },
#         }

#     @property
#     def context(self):
#         return {
#             'name': self.context_name,
#             'context': {
#                 'cluster': self.context_name,
#                 'user': self.context_name,
#             },
#         }

#     def dump(self):
#         d = {
#             'apiVersion': 'v1',
#             'kind': 'Config',
#             'contexts': [self.context],
#             'clusters': [self.cluster],
#             'current-context': self.context_name,
#             'preferences': {},
#             'users': [self.user()],
#         }
#         return yaml.dump(d)
