import StringIO

from mock import (
    Mock,
    patch,
    )

from assess_agent_metadata import (
    assess_check_metadata,
    get_controller_url_and_sha256,
    verify_deployed_tool,
    parse_args,
    )

from tests import (
    parse_error,
    TestCase,
    )

from utility import (
    JujuAssertionError,
    )

AGENT_METADATA_URL = 'file:///~/stream'
SAMPLE_SHA256 = \
    "ce3c940bd7523d307ae546d2f46e722538b0972fbb77abc5ba6bd639400827a8"
VALID_URL = "file:///home/juju/juju-2.0.1-xenial-amd64.tgz"


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

    def test_parse_args_agent_metadata_url(self):
        args = parse_args(['base', 'foo', 'bar',
                           '--agent-metadata-url', AGENT_METADATA_URL])
        self.assertEqual(args.agent_metadata_url, AGENT_METADATA_URL)


class TestAssess(TestCase):
    def test_assess_check_metadata(self):
        args = parse_args(['metadata', 'bars', '/foo'])
        args.agent_metadata_url = AGENT_METADATA_URL
        mock_client = Mock(spec=["get_model_config"])
        mock_client.get_model_config.return_value = \
            {'agent-metadata-url': {'value': AGENT_METADATA_URL}}
        assess_check_metadata(args, mock_client)

    def test_assess_check_metadata_invalid(self):
        args = parse_args(['metadata', 'bars', '/foo'])
        args.agent_metadata_url = AGENT_METADATA_URL
        mock_client = Mock(spec=["get_model_config"])
        mock_client.get_model_config.return_value = \
            {'agent-metadata-url': {'value': "INVALID"}}
        with self.assertRaises(JujuAssertionError):
            assess_check_metadata(args, mock_client)

    def test_get_controller_url_and_sha256(self):
        expected_sha256 = SAMPLE_SHA256
        expected_url =\
            "https://example.com/juju-2.0.1-xenial-amd64.tgz"

        args = parse_args(['metadata', 'bars', '/foo'])
        args.agent_metadata_url = AGENT_METADATA_URL
        mock_client = Mock()
        controller_client = mock_client.get_controller_client()

        with patch.object(mock_client, 'get_controller_client',
                          return_value=controller_client, autospec=True):
            controller_client.run.return_value \
                = [{
                u'MachineId': u'0',
                u'Stdout': u'{"version":"2.0.1-xenial-amd64",'
                           u'"url":'u'"https://example.com/juju-2.0.1-xenial-amd64.tgz",'
                           u'"sha256":"ce3c940bd7523d307ae546d2f46e722538b0972fbb77abc5ba6bd639400827a8",'
                           u'"size":23539756}'
                }]
            controller_url, controller_sha256 = \
                get_controller_url_and_sha256(mock_client)
            self.assertEqual(controller_url, expected_url)
            self.assertEqual(controller_sha256, expected_sha256)

    def test_verify_deployed_tool_vaid(self):
        args = parse_args(['metadata', 'bars', '/foo'])
        args.agent_metadata_url = AGENT_METADATA_URL
        mock_client = Mock()
        controller_url = VALID_URL
        with patch('assess_agent_metadata.get_local_url_and_sha256',
                   return_value=dict(URL=controller_url,
                                     SHA256=SAMPLE_SHA256)):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=[controller_url, SAMPLE_SHA256]):
                verify_deployed_tool(args.agent_metadata_url, mock_client)

    def test_verify_deployed_tool_invalid_sha256(self):
        args = parse_args(['metadata', 'bars', '/foo'])
        args.agent_metadata_url = AGENT_METADATA_URL
        mock_client = Mock()
        controller_url = VALID_URL
        with patch('assess_agent_metadata.get_local_url_and_sha256',
                   return_value=dict(URL=VALID_URL, SHA256=SAMPLE_SHA256)):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=[controller_url, "INVALID_SHA256"]):
                with self.assertRaises(JujuAssertionError):
                    verify_deployed_tool(
                        args.agent_metadata_url, mock_client)

    def test_verify_deployed_tool_invalid_url(self):
        args = parse_args(['metadata', 'bars', '/foo'])
        args.agent_metadata_url = AGENT_METADATA_URL
        mock_client = Mock()
        with patch('assess_agent_metadata.get_local_url_and_sha256',
                   return_value=dict(URL=VALID_URL, SHA256=SAMPLE_SHA256)):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=["file:///INVALID_URL", SAMPLE_SHA256]):
                with self.assertRaises(JujuAssertionError):
                    verify_deployed_tool(args.agent_metadata_url, mock_client)
