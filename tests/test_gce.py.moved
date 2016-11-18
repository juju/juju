from datetime import (
    datetime,
    timedelta,
)
from mock import (
    Mock,
    patch,
)

import pytz

from tests import TestCase
import gce


GCE_ENVIRON = {
    'GCE_SA_EMAIL': '1234asdf@serviceaccount.google.com',
    'GCE_PEM_PATH': '/my-gce-serveraccount.json',
    'GCE_PROJECT_ID': 'test-project',
}


def make_fake_client():
    client = Mock(['list_nodes', 'destroy_node'])
    return client


class GCETestCase(TestCase):

    test_environ = GCE_ENVIRON

    def test_main_list_instances(self):
        client = make_fake_client()
        with patch('gce.get_client', autospec=True,
                   return_value=client) as gc_mock:
            with patch('gce.list_instances', autospec=True) as li_mock:
                with patch.dict('os.environ', GCE_ENVIRON):
                    code = gce.main(
                        ['gce.py', 'list-instances', 'juju-deploy*'])
        self.assertEqual(0, code)
        gc_mock.assert_called_once_with(
            GCE_ENVIRON['GCE_SA_EMAIL'], GCE_ENVIRON['GCE_PEM_PATH'],
            GCE_ENVIRON['GCE_PROJECT_ID'])
        li_mock.assert_called_once_with(
            client, glob='juju-deploy*', print_out=True)

    def test_main_delete_instance(self):
        client = make_fake_client()
        with patch('gce.get_client', autospec=True,
                   return_value=client) as gc_mock:
            with patch('gce.delete_instances', autospec=True) as di_mock:
                with patch.dict('os.environ', GCE_ENVIRON):
                    code = gce.main(
                        ['gce.py', 'delete-instances', '-o', '2',
                         'juju-deploy*'])
        self.assertEqual(0, code)
        gc_mock.assert_called_once_with(
            GCE_ENVIRON['GCE_SA_EMAIL'], GCE_ENVIRON['GCE_PEM_PATH'],
            GCE_ENVIRON['GCE_PROJECT_ID'])
        di_mock.assert_called_once_with(
            client, 'juju-deploy*', old_age=2, dry_run=False)

    # def test_list_instances(self):
    #     client = make_fake_client()
    #     client.init_services()
    #     groups = [ResourceGroup('juju-foo-0'), ResourceGroup('juju-bar-1')]
    #     client.resource.resource_groups.list.return_value = groups
    #     result = list_instances(client, 'juju-bar*')
    #     rgd = ResourceGroupDetails(client, groups[-1])
    #     self.assertEqual([rgd], result)
    #     client.resource.resource_groups.list.assert_called_once_with()

    # def test_list_instances_ignore_default(self):
    #     # Default resources are created by Azure. They should only be
    #     # seen via the UI. A glob for everything will still ignore defaults.
    #     client = make_fake_client()
    #     client.init_services()
    #     groups = [ResourceGroup('{}-network'.format(DEFAULT_RESOURCE_PREFIX))]
    #     client.resource.resource_groups.list.return_value = groups
    #     result = list_instances(client, '*')
    #     self.assertEqual([], result)

    # def test_delete_instances_found_old(self):
    #     now = datetime.now(tz=pytz.utc)
    #     client = make_fake_client()
    #     client.init_services()
    #     group = ResourceGroup('juju-bar-1')
    #     client.resource.resource_groups.list.return_value = [group]
    #     # The resource_groups's storage_account is 4 hours old.
    #     storage_account = StorageAccount('abcd-12', now - timedelta(hours=4))
    #     client.storage.storage_accounts.list_by_resource_group.return_value = [
    #         storage_account]
    #     poller = FakePoller()
    #     client.resource.resource_groups.delete.return_value = poller
    #     # Delete resource groups that are 2 hours old.
    #     count = delete_instances(client, 'juju-bar*', old_age=2, now=now)
    #     self.assertEqual(1, count)
    #     client.resource.resource_groups.delete.assert_called_once_with(
    #         'juju-bar-1')
    #     self.assertIs(True, poller.is_done)

    # def test_delete_instances_not_found_old(self):
    #     now = datetime.now(tz=pytz.utc)
    #     client = make_fake_client()
    #     client.init_services()
    #     group = ResourceGroup('juju-bar-1')
    #     client.resource.resource_groups.list.return_value = [group]
    #     # The resource_groups's storage_account is 2 hours old.
    #     storage_account = StorageAccount('abcd-12', now - timedelta(hours=2))
    #     client.storage.storage_accounts.list_by_resource_group.return_value = [
    #         storage_account]
    #     # Delete resource groups that are 8 hours old.
    #     count = delete_instances(client, 'juju-bar*', old_age=8, now=now)
    #     self.assertEqual(0, count)
    #     self.assertEqual(0, client.resource.resource_groups.delete.call_count)

    # def test_delete_instances_read_only(self):
    #     now = datetime.now(tz=pytz.utc)
    #     client = ARMClient(
    #         'subscription_id', 'client_id', 'secret', 'tenant', read_only=True)
    #     client.init_services()
    #     group = ResourceGroup('juju-bar-1')
    #     client.resource.resource_groups.list.return_value = [group]
    #     # The resource_groups's storage_account is 4 hours old.
    #     storage_account = StorageAccount('abcd-12', now - timedelta(hours=4))
    #     client.storage.storage_accounts.list_by_resource_group.return_value = [
    #         storage_account]
    #     count = delete_instances(client, 'juju-bar*', old_age=2, now=now)
    #     self.assertEqual(0, count)
    #     self.assertEqual(0, client.resource.resource_groups.delete.call_count)

    # def test_delete_instances_old_age_0(self):
    #     now = datetime.now(tz=pytz.utc)
    #     client = make_fake_client()
    #     client.init_services()
    #     a_group = ResourceGroup('juju-bar-1')
    #     b_group = ResourceGroup('juju-foo-0')
    #     client.resource.resource_groups.list.return_value = [a_group, b_group]
    #     poller = FakePoller()
    #     client.resource.resource_groups.delete.return_value = poller
    #     # Delete resource groups that are 0 hours old.
    #     # All matched resource_groups are deleted
    #     count = delete_instances(client, 'juju-bar*', old_age=0, now=now)
    #     self.assertEqual(1, count)
    #     self.assertEqual(1, client.resource.resource_groups.delete.call_count)
    #     self.assertIs(True, poller.is_done)

    # def test_delete_instance_with_id(self):
    #     client = make_fake_client()
    #     client.init_services()
    #     poller = FakePoller()
    #     client.compute.virtual_machines.delete.return_value = poller
    #     vm1 = VirtualMachine('name-0', 'id-a')
    #     rgd1 = ResourceGroupDetails(client, ResourceGroup('one'), vms=[vm1])
    #     vm2 = VirtualMachine('name-0', 'id-b')
    #     rgd2 = ResourceGroupDetails(client, ResourceGroup('two'), vms=[vm2])
    #     with patch('gce.list_instances', autospec=True,
    #                return_value=[rgd1, rgd2]) as lr_mock:
    #         # Passing just an id will take the * glob path.
    #         delete_instance(client, 'id-a')
    #     lr_mock.assert_called_once_with(client, glob='*', recursive=True)
    #     client.compute.virtual_machines.delete.assert_called_once_with(
    #         'one', 'name-0')
    #     self.assertIs(True, poller.is_done)

    # def test_delete_instance_without_match(self):
    #     client = make_fake_client()
    #     client.init_services()
    #     vm1 = VirtualMachine('name-0', 'id-a')
    #     rgd1 = ResourceGroupDetails(client, ResourceGroup('one'), vms=[vm1])
    #     with patch('gce.list_instances', autospec=True,
    #                return_value=[rgd1]):
    #         # Passing an non-existent id bypasses the call to delete.
    #         with self.assertRaises(ValueError):
    #             delete_instance(client, 'id-z')
    #     self.assertEqual(0, client.compute.virtual_machines.delete.call_count)
