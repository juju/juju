from datetime import (
    datetime,
    timedelta,
)
from mock import patch
from unittest import TestCase

from winazure import (
    main,
)


class WinAzureTestCase(TestCase):

    def test_main_delete_unused_disks(self):
        with patch('winazure.delete_unused_disks', autospec=True) as d_mock:
            with patch('winazure.ServiceManagementService',
                       autospec=True, return_value='sms') as sms_mock:
                main(['-d', '-v', '-c' 'cert.pem', '-s', 'secret',
                      'delete-unused-disks'])
        sms_mock.assert_called_once_with('secret', 'cert.pem')
        d_mock.assert_called_once_with('sms', dry_run=True, verbose=True)

    def test_main_list_services(self):
        with patch('winazure.list_services', autospec=True) as l_mock:
            with patch('winazure.ServiceManagementService',
                       autospec=True, return_value='sms') as sms_mock:
                main(['-d', '-v', '-c' 'cert.pem', '-s', 'secret',
                      'list-services', 'juju-deploy*'])
        sms_mock.assert_called_once_with('secret', 'cert.pem')
        l_mock.assert_called_once_with(
            'sms', glob='juju-deploy*', verbose=True)

    def test_main_delete_services(self):
        with patch('winazure.delete_services', autospec=True) as l_mock:
            with patch('winazure.ServiceManagementService',
                       autospec=True, return_value='sms') as sms_mock:
                main(['-d', '-v', '-c' 'cert.pem', '-s', 'secret',
                      'delete-services', '-o', '2', 'juju-deploy*'])
        sms_mock.assert_called_once_with('secret', 'cert.pem')
        l_mock.assert_called_once_with(
            'sms', glob='juju-deploy*', old_age=2, verbose=True, dry_run=True)
