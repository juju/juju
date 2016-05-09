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

from winazurearm import (
    ARMClient,
    list_resources,
    main,
    OLD_MACHINE_AGE,
)  # nopep8 (as above)


AZURE_ENVIRON = {
    'AZURE_SUBSCRIPTION_ID': 'subscription_id',
    'AZURE_CLIENT_ID': 'client_id',
    'AZURE_SECRET': 'secret',
    'AZURE_TENANT': 'tenant',
}


@patch.dict(os.environ, AZURE_ENVIRON)
class WinAzureARMTestCase(TestCase):

    @patch('winazurearm.ARMClient.init_services', autospec=True)
    def test_main_list_resources(self, is_mock):
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        with patch('winazurearm.list_resources', autospec=True) as lr_mock:
            code = main(['list-resources', 'juju-deploy*'])
        self.assertEqual(0, code)
        self.assertEqual(1, is_mock.call_count)
        lr_mock.assert_called_once_with(
            client, glob='juju-deploy*', print_out=True, recursive=False)

    @patch('winazurearm.ARMClient.init_services', autospec=True)
    def test_main_delete_resources(self, is_mock):
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        with patch('winazurearm.delete_resources', autospec=True) as dr_mock:
            code = main(['delete-resources', 'juju-deploy*'])
        self.assertEqual(0, code)
        self.assertEqual(1, is_mock.call_count)
        dr_mock.assert_called_once_with(
            client, glob='juju-deploy*', old_age=OLD_MACHINE_AGE)

    @patch('winazurearm.ARMClient.init_services', autospec=True)
    def test_main_delete_resources_old_age(self, is_mock):
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        with patch('winazurearm.delete_resources', autospec=True) as dr_mock:
            code = main(['delete-resources', '-o', '2', 'juju-deploy*'])
        self.assertEqual(0, code)
        self.assertEqual(1, is_mock.call_count)
        dr_mock.assert_called_once_with(
            client, glob='juju-deploy*', old_age=2)

    @patch('winazurearm.ARMClient.init_services', autospec=True)
    def test_main_delete_instance_instance_id(self, is_mock):
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        with patch('winazurearm.delete_instance', autospec=True) as di_mock:
            code = main(['delete-instance', 'instance-id'])
        self.assertEqual(0, code)
        self.assertEqual(1, is_mock.call_count)
        di_mock.assert_called_once_with(
            client, 'instance-id', resource_group=None)

    @patch('winazurearm.ARMClient.init_services', autospec=True)
    def test_main_delete_instance_name_groop(self, is_mock):
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        with patch('winazurearm.delete_instance', autospec=True) as di_mock:
            code = main(['delete-instance', 'name', 'group'])
        self.assertEqual(0, code)
        self.assertEqual(1, is_mock.call_count)
        di_mock.assert_called_once_with(client, 'name', resource_group='group')

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
