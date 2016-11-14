from contextlib import contextmanager
from textwrap import dedent
from StringIO import StringIO

from mock import patch

from fakejuju import fake_juju_client
from jujupy import JujuData
from assess_add_cloud import (
    assess_all_clouds,
    assess_cloud,
    write_status,
    )
from tests import FakeHomeTestCase
from utility import JujuAssertionError


class TestCase(FakeHomeTestCase):

    def make_fake_juju_client(self):
        env = JujuData('foo', juju_home=self.juju_home)
        return fake_juju_client(env=env)


class TestAssessCloud(TestCase):

    @contextmanager
    def cloud_client(self, clouds):
        client = self.make_fake_juju_client()
        client.env.load_yaml()

        def get_clouds(cloud_name):
            if clouds is not None:
                return clouds
            return {'clouds': {
                cloud_name: client.env.clouds['clouds'][cloud_name],
                }}

        def dump(cloud_name):
            client.env.write_clouds(client.env.juju_home,
                                    clouds)

        with patch.object(client, 'add_cloud_interactive', dump):
            yield client

    def test_assess_cloud(self):
        expected_cloud = {'clouds': {'foo': {
                'type': 'maas',
                'endpoint': 'http://bar.example.com',
                }}}
        with self.cloud_client(expected_cloud) as client:
            assess_cloud(client, expected_cloud['clouds']['foo'])

    def test_assess_cloud_missing(self):
        with self.cloud_client({'clouds': {}}) as client:
            with self.assertRaisesRegexp(JujuAssertionError,
                                         'Clouds missing!'):
                assess_cloud(client, {
                    'type': 'maas',
                    'endpoint': 'http://bar.example.com',
                    })

    def test_assess_cloud_mismatch(self):
        with self.cloud_client({'clouds': {'foo': {}}}) as client:
            with self.assertRaisesRegexp(JujuAssertionError,
                                         'Cloud mismatch'):
                stderr = StringIO()
                with patch('sys.stderr', stderr):
                    assess_cloud(client, {
                        'type': 'maas',
                        'endpoint': 'http://bar.example.com',
                        })
        self.assertEqual(dedent("""
            Expected:
            {endpoint: 'http://bar.example.com', type: maas}

            Actual:
            {}
            """), stderr.getvalue())


class TestAssessAllClouds(TestCase):

    def test_assess_all_clouds(self):
        client = self.make_fake_juju_client()
        clouds = {'a': {}, 'b': {}}
        exception = Exception()
        with patch('assess_add_cloud.assess_cloud',
                   side_effect=[None, exception]):
            with patch('sys.stdout'):
                with patch('logging.exception') as exception_mock:
                    succeeded, failed = assess_all_clouds(client, clouds)
        self.assertEqual({'a'}, succeeded)
        self.assertEqual({'b'}, failed)
        exception_mock.assert_called_once_with(exception)


class TestWriteStatus(TestCase):

    def do_write(self, status, items):
        stdout = StringIO()
        with patch('sys.stdout', stdout):
            write_status(status, items)
        return stdout.getvalue()

    def test_write_none(self):
        self.assertEqual('pending: none\n', self.do_write('pending', set()))

    def test_write_one(self):
        self.assertEqual('pending: q\n', self.do_write('pending', {'q'}))

    def test_write_two(self):
        self.assertEqual('pending: q, r\n',
                         self.do_write('pending', {'r', 'q'}))
