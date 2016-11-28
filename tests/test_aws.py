from argparse import Namespace
from datetime import (
    datetime,
    timedelta,
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

from dateutil import parser as date_parser
from dateutil import tz

from tests import TestCase
import aws


AWS_ENVIRON = {
    'AWS_ACCESS_KEY': 'access123',
    'AWS_SECRET_KEY': 'secret',
}


def make_fake_client():
    client = Mock(['list_nodes', 'destroy_node'], region_name='us-west-1')
    return client


def make_fake_node(name='foo', state='running', created=None, tags=None):
    node = Mock(extra={}, state=state, created_at=created)
    # Overide the mock name callable with a Node name attribute.
    node.name = name
    if tags:
        node.extra['tags'] = tags
    return node


class AWSTestCase(TestCase):

    test_environ = AWS_ENVIRON

    def test_main_list_instances(self):
        client = make_fake_client()
        with patch('aws.get_client', autospec=True,
                   return_value=client) as gc_mock:
            with patch('aws.list_instances', autospec=True) as li_mock:
                with patch.dict('os.environ', AWS_ENVIRON):
                    code = aws.main(
                        ['aws.py', 'us-west-1',
                         'list-instances', 'juju-deploy*'])
        self.assertEqual(0, code)
        gc_mock.assert_called_once_with(
            AWS_ENVIRON['AWS_ACCESS_KEY'], AWS_ENVIRON['AWS_SECRET_KEY'],
            'us-west-1')
        li_mock.assert_called_once_with(
            client, glob='juju-deploy*', print_out=True)

    def test_main_delete_instance(self):
        client = make_fake_client()
        with patch('aws.get_client', autospec=True,
                   return_value=client) as gc_mock:
            with patch('aws.delete_instances', autospec=True) as di_mock:
                with patch.dict('os.environ', AWS_ENVIRON):
                    code = aws.main(
                        ['aws.py', 'us-west-1',
                         'delete-instances', '-o', '2', 'juju-deploy*'])
        self.assertEqual(0, code)
        gc_mock.assert_called_once_with(
            AWS_ENVIRON['AWS_ACCESS_KEY'], AWS_ENVIRON['AWS_SECRET_KEY'],
            region='us-west-1')
        di_mock.assert_called_once_with(
            client, 'juju-deploy*', old_age=2, dry_run=False)

    def test_parse_args_delete_instaces(self):
        with patch.dict('os.environ', AWS_ENVIRON):
            args = aws.parse_args(
                ['aws.py', '-v', '-d', 'us-west-1',
                 'delete-instances', '-o', '2', 'juju-deploy*'])
        expected = Namespace(
            command='delete-instances', dry_run=True, filter='juju-deploy*',
            old_age=2, aws_access_key='access123', aws_secret='secret',
            region='us-west-1', verbose=10)
        self.assertEqual(expected, args)

    def test_parse_args_list_instances(self):
        with patch.dict('os.environ', AWS_ENVIRON):
            args = aws.parse_args(
                ['aws.py', '-v', '-d', 'us-west-1',
                 'list-instances', 'juju-deploy*'])
        expected = Namespace(
            command='list-instances', dry_run=True, filter='juju-deploy*',
            aws_access_key='access123', aws_secret='secret',
            region='us-west-1', verbose=10)
        self.assertEqual(expected, args)

    def test_parse_args_without_env(self):
        args = aws.parse_args(
            ['aws.py', '-v', '-d', 'us-west-1',
             '--aws-access-key', 'access123', '--aws-secret', 'secret',
             'delete-instances', '-o', '2', 'juju-deploy*'])
        expected = Namespace(
            command='delete-instances', dry_run=True, filter='juju-deploy*',
            old_age=2, aws_access_key='access123', aws_secret='secret',
            region='us-west-1', verbose=10)
        self.assertEqual(expected, args)

    def test_is_permanent_true(self):
        node = make_fake_node(tags={'permanent': 'true', 'bingo': 'foo'})
        self.assertIsTrue(aws.is_permanent(node))

    def test_is_permanent_false(self):
        node = make_fake_node(tags={'permanent': 'false', 'bingo': 'foo'})
        self.assertIsFalse(aws.is_permanent(node))

    def test_is_permanent_no_tags(self):
        node = make_fake_node(tags={})
        self.assertIsFalse(aws.is_permanent(node))

    def test_is_young_true(self):
        now = datetime.now(tz.gettz('UTC'))
        hour_ago = now - timedelta(hours=1)
        node = make_fake_node(created=hour_ago)
        self.assertIsTrue(aws.is_young(node, aws.OLD_MACHINE_AGE))

    def test_is_young_false(self):
        days_ago = datetime.now(tz.gettz('UTC')) - timedelta(days=2)
        node = make_fake_node(created=days_ago)
        self.assertIsFalse(aws.is_young(node, aws.OLD_MACHINE_AGE))

    def test_is_young_no_created(self):
        node = make_fake_node()
        self.assertEqual({}, node.extra)
        self.assertIsTrue(aws.is_young(node, aws.OLD_MACHINE_AGE))

    def test_list_instances(self):
        no_node = make_fake_node(name='bingo')
        not_node = make_fake_node(name='juju-terminated', state=['terminated'])
        yes_node = make_fake_node(name='juju-controller')
        client = make_fake_client()
        client.list_nodes.return_value = [no_node, yes_node, not_node]
        found = aws.list_instances(client, 'juju-*')
        client.list_nodes.assert_called_once_with()
        self.assertEqual([yes_node], found)

    def test_list_instances_with_print(self):
        node_one = make_fake_node(name='juju-controller')
        hours_ago = date_parser.parse('2016-11-01T13:01:01.0+01:00')
        node_two = make_fake_node(name='juju-app', created=hours_ago)
        client = make_fake_client()
        client.list_nodes.return_value = [node_one, node_two]
        with patch('sys.stdout', new_callable=StringIO) as so_sio:
            found = aws.list_instances(client, 'juju-*', print_out=True)
        client.list_nodes.assert_called_once_with()
        self.assertEqual([node_one, node_two], found)
        self.assertEqual(
            'juju-controller\tus-west-1\tUNKNOWN\trunning\n'
            'juju-app\tus-west-1\t2016-11-01T13:01:01+01:00\trunning\n',
            so_sio.getvalue())

    def test_delete_instances(self):
        now = datetime.now(tz.gettz('UTC'))
        hour_ago = now - timedelta(hours=1)
        young_node = make_fake_node(name='juju-young', created=hour_ago)
        perm_node = make_fake_node(
            name='juju-perm', tags={'permanent': 'true'})
        days_ago = datetime.now(tz.gettz('UTC')) - timedelta(days=2)
        old_node = make_fake_node(name='juju-old', created=days_ago)
        client = make_fake_client()
        client.list_nodes.return_value = [young_node, perm_node, old_node]
        count = aws.delete_instances(
            client, 'juju-*', old_age=aws.OLD_MACHINE_AGE)
        self.assertEqual(1, count)
        client.destroy_node.assert_called_once_with(old_node)

    def test_delete_instances_none_found(self):
        client = make_fake_client()
        client.list_nodes.return_value = []
        count = aws.delete_instances(client, 'juju-*')
        self.assertEqual(0, count)
        self.assertEqual(0, client.destroy_node.call_count)
        self.assertEqual(
            'WARNING The no machines match juju-* that are older than 14\n',
            self.log_stream.getvalue())

    def test_delete_instances_destroy_failed(self):
        days_ago = datetime.now(tz.gettz('UTC')) - timedelta(days=2)
        old_node = make_fake_node(name='juju-old', created=days_ago)
        client = make_fake_client()
        client.list_nodes.return_value = [old_node]
        client.destroy_node.return_value = False
        count = aws.delete_instances(client, 'juju-*')
        self.assertEqual(0, count)
        self.assertEqual(1, client.destroy_node.call_count)
        self.assertEqual(
            'ERROR Cannot delete juju-old\n',
            self.log_stream.getvalue())
