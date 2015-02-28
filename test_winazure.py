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
