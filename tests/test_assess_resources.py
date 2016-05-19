import copy
import logging
from mock import Mock, patch, call
import StringIO

from assess_resources import (
    assess_resources,
    parse_args,
    push_resource,
    main,
    verify_status,
)
from tests import (
    parse_error,
    TestCase,
)
from utility import JujuAssertionError


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertNotIn("TODO", fake_stdout.getvalue())


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        env = object()
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_resources.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_resources.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch("jujupy.SimpleEnvironment.from_config",
                           return_value=env) as mock_e:
                    with patch("jujupy.EnvJujuClient.by_version",
                               return_value=client) as mock_c:
                        with patch("assess_resources.assess_resources",
                                   autospec=True) as mock_assess:
                            main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_e.assert_called_once_with("an-env")
        mock_c.assert_called_once_with(env, "/bin/juju", debug=False)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    status = {'resources': [{
        'expected': {
            'origin': 'upload', 'used': True, 'description': 'foo resource.',
            'username': 'admin@local', 'resourceid': 'dummy-resource/foo',
            'name': 'foo', 'serviceid': 'dummy-resource', 'path': 'foo.txt',
            'fingerprint': '1234', 'type': 'file', 'size': 27},
        'unit': {
            'origin': 'upload', 'username': 'admin@local', 'used': True,
            'name': 'foo', 'resourceid': 'dummy-resource/foo',
            'serviceid': 'dummy-resource', 'fingerprint': '1234',
            'path': 'foo.txt', 'size': 27, 'type': 'file',
            'description': 'foo resource.'}}]}

    def test_verify_status(self):
        verify_status(self.status, 'dummy-resource/foo', 'foo', '1234', 27)

    def test_verify_status_exception(self):
        status = copy.deepcopy(self.status)
        status['resources'][0]['expected']['origin'] = 'charmstore'
        with self.assertRaisesRegexp(
                JujuAssertionError, 'Unexpected resource list values'):
            verify_status(status, 'dummy-resource/foo', 'foo', '1234', 27)

    def test_verify_status_unit_exception(self):
        status = copy.deepcopy(self.status)
        status['resources'][0]['unit']['origin'] = 'charmstore'
        with self.assertRaisesRegexp(
                JujuAssertionError, 'Unexpected unit resource list values'):
            verify_status(status, 'dummy-resource/foo', 'foo', '1234', 27)

    def test_verify_status_no_resoruce_id_exception(self):
        with self.assertRaisesRegexp(
                JujuAssertionError, 'Resource id not found.'):
            verify_status(self.status, 'NO_ID', 'foo', '1234', 27)

    def test_push_resource(self):
        mock_client = Mock(
            spec=["deploy", "wait_for_started", "list_resources"])
        mock_client.version = '2.0.0'
        mock_client.list_resources.return_value = self.status
        push_resource(mock_client, 'foo', '1234', 27)
        mock_client.deploy.assert_called_once_with(
            'dummy-resource', resource='foo=dummy-resource/foo.txt')
        mock_client.wait_for_started.assert_called_once_with()

    def test_push_resource_attach(self):
        mock_client = Mock(
            spec=["attach", "wait_for_started", "list_resources"])
        mock_client.version = '2.0.0'
        mock_client.list_resources.return_value = self.status
        push_resource(mock_client, 'foo', '1234', 27, deploy=False)
        mock_client.attach.assert_called_once_with(
            'dummy-resource', resource='foo=dummy-resource/foo.txt')
        mock_client.wait_for_started.assert_called_once_with()

    def test_assess_resources(self):
        with patch("assess_resources.push_resource", autospec=True) as mock_p:
            assess_resources(None)
        calls = [
            call(
                None, 'foo',
                '4ddc48627c6404e538bb0957632ef68618c0839649d9ad9e41ad94472c158'
                '9f4b7f9d830df6c4b209d7eb1b4b5522c4d', 27),
            call(
                None, 'bar',
                'ffbf43d68a6960de63908bb05c14a026abeda136119d3797431bdd7b469c1'
                'f027e57a28aeec0df01a792e9e70aad2d6b', 17, deploy=False)]
        self.assertEqual(mock_p.mock_calls, calls)
