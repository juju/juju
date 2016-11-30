from argparse import Namespace
from datetime import (
    datetime,
)
from mock import (
    Mock,
    patch,
)
try:
    # Import none-unicode first because it means unicode is not default.
    from StringIO import StringIO
except ImportError:
    from io import StringIO

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


def make_fake_node(name='foo', state='running',
                   zone=None, created=None, tags=None):
    node = Mock(extra={}, state=state)
    # Overide the mock name callable with a Node name attribute.
    node.name = name
    # Zone is not guaranteed, but when it is present, it is a object
    if zone:
        zone_region = Mock()
        zone_region.name = zone
        node.extra['zone'] = zone_region
    if tags:
        node.extra['tags'] = tags
    if created:
        node.extra['creationTimestamp'] = created
    return node


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
            GCE_ENVIRON['GCE_PROJECT_ID'], region=None)
        li_mock.assert_called_once_with(
            client, glob='juju-deploy*', print_out=True)

    def test_main_list_instances_region(self):
        client = make_fake_client()
        with patch('gce.get_client', autospec=True,
                   return_value=client) as gc_mock:
            with patch('gce.list_instances', autospec=True) as li_mock:
                with patch.dict('os.environ', GCE_ENVIRON):
                    code = gce.main(
                        ['gce.py', '--region', 'test-region', 'list-instances',
                         'juju-deploy*'])
        self.assertEqual(0, code)
        gc_mock.assert_called_once_with(
            GCE_ENVIRON['GCE_SA_EMAIL'], GCE_ENVIRON['GCE_PEM_PATH'],
            GCE_ENVIRON['GCE_PROJECT_ID'], region='test-region')
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
            GCE_ENVIRON['GCE_PROJECT_ID'], region=None)
        di_mock.assert_called_once_with(
            client, 'juju-deploy*', old_age=2, dry_run=False)

    def test_get_client(self):
        client = Mock(spec=['ex_get_zone'])
        client.return_value.ex_get_zone.return_value = 'bar'
        with patch('libcloud.compute.providers.get_driver', autospec=True
                   ) as gl_mock:
            gl_mock.return_value = client
            gce.get_client(GCE_ENVIRON['GCE_SA_EMAIL'],
                       GCE_ENVIRON['GCE_PEM_PATH'],
                       GCE_ENVIRON["GCE_PROJECT_ID"], region='foo')
        gl_mock.assert_called_once_with('gce')
        client.return_value.ex_get_zone.assert_called_once_with('foo')

    def test_get_client_no_region(self):
        client = Mock(spec=['ex_get_zone'])
        client.return_value.ex_get_zone.return_value = 'bar'
        with patch('libcloud.compute.providers.get_driver', autospec=True
                   ) as gl_mock:
            gl_mock.return_value = client
            gce.get_client(GCE_ENVIRON['GCE_SA_EMAIL'],
                       GCE_ENVIRON['GCE_PEM_PATH'],
                       GCE_ENVIRON["GCE_PROJECT_ID"], region=None)
        gl_mock.assert_called_once_with('gce')
        self.assertEqual(client.return_value.ex_get_zone.call_count, 0)

    def test_get_client_exception(self):
        client = Mock(spec=['ex_get_zone'])
        client.return_value.ex_get_zone.return_value = None
        with patch('libcloud.compute.providers.get_driver', autospec=True
                   ) as gl_mock:
            gl_mock.return_value = client
            with self.assertRaises(ValueError):
                gce.get_client(GCE_ENVIRON['GCE_SA_EMAIL'],
                           GCE_ENVIRON['GCE_PEM_PATH'],
                           GCE_ENVIRON["GCE_PROJECT_ID"], region='foo')
        gl_mock.assert_called_once_with('gce')

    def test_parse_args_delete_instaces(self):
        with patch.dict('os.environ', GCE_ENVIRON):
            args = gce.parse_args(
                ['gce.py', '-v', '-d',
                 'delete-instances', '-o', '2', 'juju-deploy*'])
        expected = Namespace(
            command='delete-instances', dry_run=True, filter='juju-deploy*',
            old_age=2, pem_path='/gce-serveraccount.json',
            project_id='test-project', region=None,
            sa_email='me@serviceaccount.google.com', verbose=10)
        self.assertEqual(expected, args)

    def test_parse_args_list_instances(self):
        with patch.dict('os.environ', GCE_ENVIRON):
            args = gce.parse_args(
                ['gce.py', '-v', '-d', 'list-instances', 'juju-deploy*'])
        expected = Namespace(
            command='list-instances', dry_run=True, filter='juju-deploy*',
            pem_path='/gce-serveraccount.json', project_id='test-project',
            region=None, sa_email='me@serviceaccount.google.com',
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
            project_id='test-project', region=None,
            sa_email='me@serviceaccount.google.com', verbose=10)
        self.assertEqual(expected, args)

    def test_is_permanent_true(self):
        node = make_fake_node(tags=['permanent', 'bingo'])
        self.assertIsTrue(gce.is_permanent(node))

    def test_is_permanent_false(self):
        node = make_fake_node(tags=['bingo'])
        self.assertIsFalse(gce.is_permanent(node))

    def test_is_permanent_no_tags(self):
        node = make_fake_node(tags=[])
        self.assertIsFalse(gce.is_permanent(node))

    def test_is_young_true(self):
        now = datetime.utcnow()
        hour_ago = '{}-01:00'.format(now.isoformat())
        node = make_fake_node(created=hour_ago)
        self.assertIsTrue(gce.is_young(node, gce.OLD_MACHINE_AGE))

    def test_is_young_false(self):
        node = make_fake_node(created='2016-11-01T13:06:23.968-08:00')
        self.assertIsFalse(gce.is_young(node, gce.OLD_MACHINE_AGE))

    def test_is_young_no_created(self):
        node = make_fake_node()
        self.assertEqual({}, node.extra)
        self.assertIsTrue(gce.is_young(node, gce.OLD_MACHINE_AGE))

    def test_list_instances(self):
        no_node = make_fake_node(name='bingo')
        yes_node = make_fake_node(name='juju-controller')
        client = make_fake_client()
        client.list_nodes.return_value = [no_node, yes_node]
        found = gce.list_instances(client, 'juju-*')
        client.list_nodes.assert_called_once_with()
        self.assertEqual([yes_node], found)

    def test_list_instances_with_print(self):
        node_one = make_fake_node(name='juju-controller')
        node_two = make_fake_node(
            name='juju-app', created='2016-11-01T13:01:01.0+01:00',
            zone='us-west1')
        client = make_fake_client()
        client.list_nodes.return_value = [node_one, node_two]
        with patch('sys.stdout', new_callable=StringIO) as so_sio:
            found = gce.list_instances(client, 'juju-*', print_out=True)
        client.list_nodes.assert_called_once_with()
        self.assertEqual([node_one, node_two], found)
        self.assertEqual(
            'juju-controller\tUNKNOWN\tNone\trunning\njuju-app\tus-west1\t'
            '2016-11-01T13:01:01.0+01:00\trunning\n',
            so_sio.getvalue())

    def test_delete_instances(self):
        now = datetime.utcnow()
        hour_ago = '{}-01:00'.format(now.isoformat())
        young_node = make_fake_node(name='juju-young', created=hour_ago)
        perm_node = make_fake_node(name='juju-perm', tags=['permanent'])
        old_node = make_fake_node(
            name='juju-old', created='2016-11-01T13:06:23.968-08:00')
        client = make_fake_client()
        client.list_nodes.return_value = [young_node, perm_node, old_node]
        count = gce.delete_instances(
            client, 'juju-*', old_age=gce.OLD_MACHINE_AGE)
        self.assertEqual(1, count)
        client.destroy_node.assert_called_once_with(old_node)

    def test_delete_instances_none_found(self):
        client = make_fake_client()
        client.list_nodes.return_value = []
        count = gce.delete_instances(client, 'juju-*')
        self.assertEqual(0, count)
        self.assertEqual(0, client.destroy_node.call_count)
        self.assertEqual(
            'WARNING The no machines match juju-* that are older than 14\n',
            self.log_stream.getvalue())

    def test_delete_instances_destroy_failed(self):
        old_node = make_fake_node(
            name='juju-old', created='2016-11-01T13:06:23.968-08:00')
        client = make_fake_client()
        client.list_nodes.return_value = [old_node]
        client.destroy_node.return_value = False
        count = gce.delete_instances(client, 'juju-*')
        self.assertEqual(0, count)
        self.assertEqual(1, client.destroy_node.call_count)
        self.assertEqual(
            'ERROR Cannot delete juju-old\n',
            self.log_stream.getvalue())

