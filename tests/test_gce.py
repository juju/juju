from datetime import (
    datetime,
)
from mock import (
    Mock,
    patch,
)
from argparse import Namespace

from tests import TestCase
import gce


GCE_ENVIRON = {
    'GCE_SA_EMAIL': 'me@serviceaccount.google.com',
    'GCE_PEM_PATH': '/gce-serveraccount.json',
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

    def test_parse_args_delete_instaces(self):
        with patch.dict('os.environ', GCE_ENVIRON):
            args = gce.parse_args(
                ['gce.py', '-v', '-d',
                 'delete-instances', '-o', '2', 'juju-deploy*'])
        expected = Namespace(
            command='delete-instances', dry_run=True, filter='juju-deploy*',
            old_age=2, pem_path='/gce-serveraccount.json',
            project_id='test-project', sa_email='me@serviceaccount.google.com',
            verbose=10)
        self.assertEqual(expected, args)

    def test_parse_args_list_instances(self):
        with patch.dict('os.environ', GCE_ENVIRON):
            args = gce.parse_args(
                ['gce.py', '-v', '-d', 'list-instances', 'juju-deploy*'])
        expected = Namespace(
            command='list-instances', dry_run=True, filter='juju-deploy*',
            pem_path='/gce-serveraccount.json',
            project_id='test-project', sa_email='me@serviceaccount.google.com',
            verbose=10)
        self.assertEqual(expected, args)

    def test_parse_args_without_env(self):
        args = gce.parse_args(
            ['gce.py', '-v', '-d',
             '--sa-email', 'me@serviceaccount.google.com',
             '--pem-path', '/gce-serveraccount.json',
             '--project-id', 'test-project',
             'delete-instances', '-o', '2', 'juju-deploy*'])
        expected = Namespace(
            command='delete-instances', dry_run=True, filter='juju-deploy*',
            old_age=2, pem_path='/gce-serveraccount.json',
            project_id='test-project', sa_email='me@serviceaccount.google.com',
            verbose=10)
        self.assertEqual(expected, args)

    def test_is_permanent_true(self):
        node = Mock(extra={'tags': ['permanent', 'bingo']})
        self.assertIsTrue(gce.is_permanent(node))

    def test_is_permanent_false(self):
        node = Mock(extra={'tags': ['bingo']})
        self.assertIsFalse(gce.is_permanent(node))

    def test_is_permanent_no_tags(self):
        node = Mock(extra={})
        self.assertIsFalse(gce.is_permanent(node))

    def test_is_young_true(self):
        now = datetime.utcnow()
        hour_ago = '{}-01:00'.format(now.isoformat())
        node = Mock(extra={'creationTimestamp': hour_ago})
        self.assertIsTrue(gce.is_young(node, gce.OLD_MACHINE_AGE))

    def test_is_young_false(self):
        node = Mock(
            extra={'creationTimestamp': '2016-11-01T13:06:23.968-08:00'})
        self.assertIsFalse(gce.is_young(node, gce.OLD_MACHINE_AGE))

    def test_is_young_no_created(self):
        node = Mock(extra={})
        self.assertIsTrue(gce.is_young(node, gce.OLD_MACHINE_AGE))
