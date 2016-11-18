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
