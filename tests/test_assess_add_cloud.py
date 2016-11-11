from contextlib import contextmanager
from textwrap import dedent
from StringIO import StringIO

from mock import patch

from fakejuju import fake_juju_client
from jujupy import JujuData
from assess_add_cloud import assess_cloud
from tests import FakeHomeTestCase
from utility import JujuAssertionError


class TestAssessCloud(FakeHomeTestCase):

    @contextmanager
    def cloud_client(self, clouds):
        env = JujuData('foo', juju_home=self.juju_home)
        client = fake_juju_client(env=env)
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
