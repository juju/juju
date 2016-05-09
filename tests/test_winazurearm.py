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
)  # nopep8 (as above)


AZURE_ENVIRON = {
    'AZURE_SUBSCRIPTION_ID': 'subscription_id',
    'AZURE_CLIENT_ID': 'client_id',
    'AZURE_SECRET': 'secret',
    'AZURE_TENANT': 'tenant',
}


class WinAzureARMTestCase(TestCase):

    @patch('winazurearm.ARMClient.init_services', autospec=True)
    @patch.dict(os.environ, AZURE_ENVIRON)
    def test_main_list_resources(self, is_mock):
        client = ARMClient('subscription_id', 'client_id', 'secret', 'tenant')
        with patch('winazurearm.list_resources', autospec=True) as lr_mock:
            code = main(['list-resources', 'juju-deploy*'])
        self.assertEqual(0, code)
        self.assertEqual(1, is_mock.call_count)
        lr_mock.assert_called_once_with(
            client, glob='juju-deploy*', print_out=True, recursive=False)
