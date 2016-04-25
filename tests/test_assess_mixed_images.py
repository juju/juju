"""Tests for assess_mixed_images module."""

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
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)
        self.assertEqual('trusty', args.series)

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
        mock_client = FakeJujuClient(jes_enabled=True)
        mock_client.bootstrap()
        assess_mixed_images(mock_client)
        self.assertEqual({
            'machines': {
                '0': {'dns-name': '0.example.com'},
                '1': {'dns-name': '1.example.com'},
                },
            'services': {
                'dummy-sink': {
                    'exposed': False,
                    'relations': {'source': ['dummy-source']},
                    'units': {'dummy-sink/0': {'machine': '0'}}
                    },
                'dummy-source': {
                    'exposed': False,
                    'relations': {},
                    'units': {'dummy-source/0': {'machine': '1'}}
                    }
                }
            }, mock_client.get_status().status)

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
