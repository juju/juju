"""Tests for assess_mixed_images module."""

from argparse import Namespace
import logging
from mock import (
    call,
    patch
)
import StringIO

from assess_mixed_images import (
    assess_mixed_images,
    parse_args,
    main,
)
from tests import (
    parse_error,
    TestCase,
)
from tests.test_jujupy import FakeJujuClient


class TestParseArgs(TestCase):

    def test_defaults(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual(Namespace(
            env='an-env',
            juju_bin='/bin/juju',
            logs='/tmp/logs',
            temp_env_name='an-env-mod',
            debug=False,
            series='trusty',
            agent_stream=None,
            agent_url=None,
            bootstrap_host=None,
            upload_tools=False,
            verbose=logging.INFO,
            image_metadata_url=None,
            keep_env=False,
            machine=[],
            region=None,
            ), args)

    def test_image_metadata_url(self):
        args = parse_args([
            'an-env', '/bin/juju', '/tmp/logs', 'an-env-mod',
            '--image-metadata-url', 'http://example.com/images'])
        self.assertEqual(args.image_metadata_url, 'http://example.com/images')

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = FakeJujuClient()
        with patch("assess_mixed_images.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_mixed_images.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch("jujupy.SimpleEnvironment.from_config",
                           return_value=client.env) as mock_e:
                    with patch("jujupy.EnvJujuClient.by_version",
                               return_value=client) as mock_c:
                        with patch("assess_mixed_images.assess_mixed_images",
                                   autospec=True) as mock_assess:
                            main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_e.assert_called_once_with("an-env")
        mock_c.assert_called_once_with(client.env, "/bin/juju", debug=False)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    def test_mixed_images(self):
        mock_client = FakeJujuClient()
        mock_client.bootstrap()
        assess_mixed_images(mock_client)
        expected = {
            'machines': {
                '0': {
                    'dns-name': '0.example.com',
                    'instance-id': '0',
                    'juju-status': {'current': 'idle'},
                    },
                '1': {
                    'dns-name': '1.example.com',
                    'instance-id': '1',
                    'juju-status': {'current': 'idle'},
                    },
                },
            'services': {
                'dummy-sink': {
                    'exposed': False,
                    'relations': {'source': ['dummy-source']},
                    'units': {
                        'dummy-sink/0': {
                            'machine': '0',
                            'juju-status': {'current': 'idle'},
                            },
                        }
                    },
                'dummy-source': {
                    'exposed': False,
                    'relations': {},
                    'units': {
                        'dummy-source/0': {
                            'machine': '1',
                            'juju-status': {'current': 'idle'},
                            }
                        }
                    }
                }
            }
        actual = mock_client.get_status().status
        self.assertEqual(expected, actual)

    def test_mixed_images_charm_2x(self):
        mock_client = FakeJujuClient()
        mock_client.bootstrap()
        with patch.object(mock_client, 'deploy') as mock_d:
            with patch('assess_mixed_images.assess_juju_relations',
                       autospec=True) as mock_ajr:
                assess_mixed_images(mock_client)
        calls = [call('dummy-sink'), call('dummy-source')]
        self.assertEqual(mock_d.mock_calls, calls)
        mock_ajr.assert_called_once_with(mock_client)

    def test_mixed_images_charm_1x(self):
        mock_client = FakeJujuClient(version='1.25.0')
        mock_client.bootstrap()
        with patch.object(mock_client, 'deploy') as mock_d:
            with patch('assess_mixed_images.assess_juju_relations',
                       autospec=True) as mock_ajr:
                assess_mixed_images(mock_client)
        calls = [call('local:centos7/dummy-sink'),
                 call('local:trusty/dummy-source')]
        self.assertEqual(mock_d.mock_calls, calls)
        mock_ajr.assert_called_once_with(mock_client)
