import logging
from argparse import Namespace
from mock import Mock, patch, call
import StringIO

from assess_resources import (
    assess_resources,
    large_assess,
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
        mock_assess.assert_called_once_with(client, make_args())


class TestAssessResources(TestCase):

    def test_verify_status(self):
        verify_status(
            make_resource_list(), 'dummy-resource/foo', 'foo', '1234', 27)

    def test_verify_status_serviceid(self):
        verify_status(make_resource_list('serviceid'), 'dummy-resource/foo',
                      'foo', '1234', 27)

    def test_verify_status_exception(self):
        status = make_resource_list()
        status['resources'][0]['expected']['origin'] = 'charmstore'
        with self.assertRaisesRegexp(
                JujuAssertionError, 'Unexpected resource list values'):
            verify_status(status, 'dummy-resource/foo', 'foo', '1234', 27)

    def test_verify_status_unit_exception(self):
        status = make_resource_list()
        status['resources'][0]['unit']['origin'] = 'charmstore'
        with self.assertRaisesRegexp(
                JujuAssertionError, 'Unexpected unit resource list values'):
            verify_status(status, 'dummy-resource/foo', 'foo', '1234', 27)

    def test_verify_status_no_resoruce_id_exception(self):
        with self.assertRaisesRegexp(
                JujuAssertionError, 'Resource id not found.'):
            verify_status(make_resource_list(), 'NO_ID', 'foo', '1234', 27)

    def test_push_resource(self):
        mock_client = Mock(
            spec=["deploy", "wait_for_started", "list_resources",
                  "wait_for_resource", "show_status"])
        mock_client.version = '2.0.0'
        mock_client.list_resources.return_value = make_resource_list()
        push_resource(mock_client, 'foo', '1234', 27, 1800, 1800)
        mock_client.deploy.assert_called_once_with(
            'dummy-resource', resource='foo=dummy-resource/foo.txt')
        mock_client.wait_for_started.assert_called_once_with(timeout=1800)
        mock_client.show_status.assert_called_once_with()

    def test_push_resource_attach(self):
        mock_client = Mock(
            spec=["attach", "wait_for_started", "list_resources",
                  "wait_for_resource", "show_status"])
        mock_client.version = '2.0.0'
        mock_client.list_resources.return_value = make_resource_list()
        push_resource(mock_client, 'foo', '1234', 27, 1800, 1800, deploy=False)
        mock_client.attach.assert_called_once_with(
            'dummy-resource', resource='foo=dummy-resource/foo.txt')
        mock_client.wait_for_started.assert_called_once_with(timeout=1800)

    def test_assess_resources(self):
        fake_file = FakeFile()
        args = make_args()
        with patch("assess_resources.push_resource", autospec=True) as mock_p:
            with patch('assess_resources.NamedTemporaryFile',
                       autospec=True) as mock_ntf:
                mock_ntf.return_value.__enter__.return_value = fake_file
                assess_resources(None, args)
        calls = [
            call(
                None, 'foo',
                '4ddc48627c6404e538bb0957632ef68618c0839649d9ad9e41ad94472c158'
                '9f4b7f9d830df6c4b209d7eb1b4b5522c4d', 27, 1800, 1800),
            call(
                None, 'bar',
                'ffbf43d68a6960de63908bb05c14a026abeda136119d3797431bdd7b469c1'
                'f027e57a28aeec0df01a792e9e70aad2d6b', 17, 1800, 1800,
                deploy=False),
            call(
                None, 'bar',
                '2a3821585efcccff1562efea4514dd860cd536441954e182a7649910e21f6'
                'a179a015677a68a351a11d3d2f277e551e4', 27, 1800, 1800,
                deploy=False, resource_file='baz.txt'),
            call(
                None, 'bar',
                '3164673a8ac27576ab5fc06b9adc4ce0aca5bd3025384b1cf2128a8795e74'
                '7c431e882785a0bf8dc70b42995db388575', 1024 * 1024, 1800, 1800,
                deploy=False, resource_file='/tmp/fake'),
        ]
        self.assertEqual(mock_p.mock_calls, calls)

    def test_assess_resources_large_test(self):
        fake_file = FakeFile()
        args = make_args()
        args.large_test_enabled = True
        with patch("assess_resources.push_resource", autospec=True) as mock_p:
            with patch('assess_resources.fill_dummy_file',
                       autospec=True) as mock_fdf:
                with patch('assess_resources.NamedTemporaryFile',
                           autospec=True) as mock_ntf:
                    mock_ntf.return_value.__enter__.return_value = fake_file
                    with patch('assess_resources.large_assess',
                               autospec=True) as mock_lt:
                        assess_resources(None, args)
        calls = [
            call(
                None, 'foo',
                '4ddc48627c6404e538bb0957632ef68618c0839649d9ad9e41ad94472c158'
                '9f4b7f9d830df6c4b209d7eb1b4b5522c4d', 27, 1800, 1800),
            call(
                None, 'bar',
                'ffbf43d68a6960de63908bb05c14a026abeda136119d3797431bdd7b469c1'
                'f027e57a28aeec0df01a792e9e70aad2d6b', 17, 1800, 1800,
                deploy=False),
            call(
                None, 'bar',
                '2a3821585efcccff1562efea4514dd860cd536441954e182a7649910e21f6'
                'a179a015677a68a351a11d3d2f277e551e4', 27, 1800, 1800,
                deploy=False, resource_file='baz.txt'),
            call(None, 'bar',
                 '3164673a8ac27576ab5fc06b9adc4ce0aca5bd3025384b1cf2128a8795e7'
                 '47c431e882785a0bf8dc70b42995db388575', 1024 * 1024, 1800,
                 1800, deploy=False, resource_file='/tmp/fake')]
        self.assertEqual(mock_p.mock_calls, calls)
        mock_fdf.assert_called_once_with('/tmp/fake', 1024 * 1024)
        mock_lt.assert_called_once_with(None, 1800, 1800)

    def test_large_tests(self):
        fake_file = FakeFile()
        with patch("assess_resources.push_resource", autospec=True) as mock_pr:
            with patch('assess_resources.NamedTemporaryFile',
                       autospec=True) as mock_ntf:
                mock_ntf.return_value.__enter__.return_value = fake_file
                with patch('assess_resources.fill_dummy_file',
                           autospec=True):
                    large_assess(None, 1800, 1800)
        calls = [
            call(
                None, 'bar',
                'd7c014629d74ae132cc9f88e3ec2f31652f40a7a1fcc52c54b04d6c0d0891'
                '69bcd55958d1277b4cdf6262f21c712d0a7', 1024 * 1024 * 10, 1800,
                1800, deploy=False, resource_file='/tmp/fake'),
            call(
                None, 'bar',
                'c11e93892b66de781e4d0883efe10482f8d1642f3b6574ba2ee0da6f8db03'
                'f53c0eadfb5e5e0463574c113024ded369e', 1024 * 1024 * 100, 1800,
                1800, deploy=False, resource_file='/tmp/fake'),
            call(
                None, 'bar',
                '77db39eca74c6205e31a7701e488a1df4b9b38a527a6084bdbb6843fd430a'
                '0b51047378ee0255e633b32c0dda3cf43ab', 1024 * 1024 * 200, 1800,
                1800, deploy=False, resource_file='/tmp/fake')]
        self.assertEqual(mock_pr.mock_calls, calls)


def make_args():
    return Namespace(
        agent_stream=None, agent_timeout=1800, agent_url=None,
        bootstrap_host=None, debug=False, env='an-env', juju_bin='/bin/juju',
        keep_env=False, large_test_enabled=False, logs='/tmp/logs', machine=[],
        region=None, resource_timeout=1800, series=None,
        temp_env_name='an-env-mod', upload_tools=False, verbose=10)


def make_resource_list(service_app_id='applicationId'):
    return {'resources': [{
        'expected': {
            'origin': 'upload', 'used': True, 'description': 'foo resource.',
            'username': 'admin@local', 'resourceid': 'dummy-resource/foo',
            'name': 'foo', service_app_id: 'dummy-resource', 'size': 27,
            'fingerprint': '1234', 'type': 'file', 'path': 'foo.txt'},
        'unit': {
            'origin': 'upload', 'username': 'admin@local', 'used': True,
            'name': 'foo', 'resourceid': 'dummy-resource/foo',
            service_app_id: 'dummy-resource', 'fingerprint': '1234',
            'path': 'foo.txt', 'size': 27, 'type': 'file',
            'description': 'foo resource.'}}]}


class FakeFile:
    name = '/tmp/fake'
