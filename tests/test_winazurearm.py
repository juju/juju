from collections import namedtuple
from datetime import (
    datetime,
    timedelta,
)
from mock import (
    Mock,
    patch,
)
import os
import sys
from unittest import TestCase

import pytz

from winazurearm import (
    ARMClient,
    DEFAULT_RESOURCE_PREFIX,
    list_resources,
    main,
    OLD_MACHINE_AGE,
    ResourceGroupDetails,
)  # nopep8 (as above)


AZURE_ENVIRON = {
    'AZURE_SUBSCRIPTION_ID': 'subscription_id',
    'AZURE_CLIENT_ID': 'client_id',
    'AZURE_SECRET': 'secret',
    'AZURE_TENANT': 'tenant',
}

ResourceGroup = namedtuple('ResourceGroup', ['name'])
StorageAccount = namedtuple('StorageAccount', ['name', 'creation_time'])
VirtualMachine = namedtuple('VirtualMachine', ['name', 'vm_id'])
Network = namedtuple('Network', ['name'])
Address = namedtuple('Address', ['name', 'ip_address'])


def fake_init_services(client):
    """Use lazy init to install mocks."""
    # client.resource.resource_groups.list()
    client.resource = Mock(resource_groups=Mock(list=Mock(return_value=[])))
    # client.storage.storage_accounts.list_by_resource_group()
    client.storage = Mock(
        storage_accounts=Mock(list_by_resource_group=Mock(return_value=[])))
    # client.compute.virtual_machines.list()
    client.compute = Mock(virtual_machines=Mock(list=Mock(return_value=[])))
    # client.network.public_ip_addresses.list()
    # client.network.virtual_networks.list()
    client.network = Mock(
        public_ip_addresses=Mock(list=Mock(return_value=[])),
        virtual_networks=Mock(list=Mock(return_value=[])))


@patch.dict(os.environ, AZURE_ENVIRON)
@patch('winazurearm.ARMClient.init_services',
       autospec=True, side_effect=fake_init_services)
class WinAzureARMTestCase(TestCase):

    def test_main_list_resources(self, is_mock):
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        with patch('winazurearm.list_resources', autospec=True) as lr_mock:
            code = main(['list-resources', 'juju-deploy*'])
        self.assertEqual(0, code)
        self.assertEqual(1, is_mock.call_count)
        lr_mock.assert_called_once_with(
            client, glob='juju-deploy*', print_out=True, recursive=False)

    def test_main_delete_resources(self, is_mock):
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        with patch('winazurearm.delete_resources', autospec=True) as dr_mock:
            code = main(['delete-resources', 'juju-deploy*'])
        self.assertEqual(0, code)
        self.assertEqual(1, is_mock.call_count)
        dr_mock.assert_called_once_with(
            client, glob='juju-deploy*', old_age=OLD_MACHINE_AGE)

    def test_main_delete_resources_old_age(self, is_mock):
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        with patch('winazurearm.delete_resources', autospec=True) as dr_mock:
            code = main(['delete-resources', '-o', '2', 'juju-deploy*'])
        self.assertEqual(0, code)
        self.assertEqual(1, is_mock.call_count)
        dr_mock.assert_called_once_with(
            client, glob='juju-deploy*', old_age=2)

    def test_main_delete_instance_instance_id(self, is_mock):
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        with patch('winazurearm.delete_instance', autospec=True) as di_mock:
            code = main(['delete-instance', 'instance-id'])
        self.assertEqual(0, code)
        self.assertEqual(1, is_mock.call_count)
        di_mock.assert_called_once_with(
            client, 'instance-id', resource_group=None)

    def test_main_delete_instance_name_groop(self, is_mock):
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        with patch('winazurearm.delete_instance', autospec=True) as di_mock:
            code = main(['delete-instance', 'name', 'group'])
        self.assertEqual(0, code)
        self.assertEqual(1, is_mock.call_count)
        di_mock.assert_called_once_with(client, 'name', resource_group='group')

    def test_list_resources(self, is_mock):
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        client.init_services()
        groups = [ResourceGroup('juju-foo-0'), ResourceGroup('juju-bar-1')]
        client.resource.resource_groups.list.return_value = groups
        result = list_resources(client, 'juju-bar*')
        rdg = ResourceGroupDetails(client, groups[-1])
        self.assertEqual([rdg], result)
        client.resource.resource_groups.list.assert_called_once_with()

    def test_list_resources_ignore_default(self, is_mock):
        # Default resources are created by Azure. They should only be
        # seen via the UI. A glob for everything will still ignore defaults.
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        client.init_services()
        groups = [ResourceGroup('{}-network'.format(DEFAULT_RESOURCE_PREFIX))]
        client.resource.resource_groups.list.return_value = groups
        result = list_resources(client, '*')
        self.assertEqual([], result)

    def test_list_resources_recursive(self, is_mock):
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        client.init_services()
        # For the call to find many groups.
        groups = [ResourceGroup('juju-foo-0'), ResourceGroup('juju-bar-1')]
        client.resource.resource_groups.list.return_value = groups
        # For the call to load a ResourceGroupDetails instance.
        storage_account = StorageAccount('abcd-12', datetime.now(tz=pytz.UTC))
        client.storage.storage_accounts.list_by_resource_group.return_value = [
            storage_account]
        virtual_machine = VirtualMachine('admin-machine-0', 'bcde-1234')
        client.compute.virtual_machines.list.return_value = [virtual_machine]
        address = Address('machine-0-public-ip', '1.2.3.4')
        client.network.public_ip_addresses.list.return_value = [address]
        network = Network('juju-bar-network-1')
        client.network.virtual_networks.list.return_value = [network]
        result = list_resources(client, 'juju-bar*', recursive=True)
        rdg = ResourceGroupDetails(
            client, groups[-1], storage_accounts=[storage_account],
            vms=[virtual_machine], addresses=[address], networks=[network])
        rdg.is_loaded = True
        self.assertEqual([rdg], result)

    # def test_main_delete_resources(self):
    #     with patch('winazure.delete_resources', autospec=True) as l_mock:
    #         with patch('winazure.ServiceManagementService',
    #                    autospec=True, return_value='sms') as sms_mock:
    #             main(['-d', '-v', '-c' 'cert.pem', '-s', 'secret',
    #                   'delete-services', '-o', '2', 'juju-deploy*'])
    #     sms_mock.assert_called_once_with('secret', 'cert.pem')
    #     l_mock.assert_called_once_with(
    #         'sms', glob='juju-deploy*', old_age=2, verbose=True, dry_run=True)

    # def test_list_resourcess(self):
    #     sms = ServiceManagementService('secret', 'cert.pem')
    #     hs1 = Mock()
    #     hs1.service_name = 'juju-upgrade-foo'
    #     hs2 = Mock()
    #     hs2.service_name = 'juju-deploy-foo'
    #     services = [hs1, hs2]
    #     with patch.object(sms, 'list_hosted_services', autospec=True,
    #                       return_value=services) as ls_mock:
    #         services = list_resources(sms, 'juju-deploy-*', verbose=False)
    #     ls_mock.assert_called_once_with()
    #     self.assertEqual([hs2], services)
