import errno
import unittest

import mock

import get_ami


class GetAmi(unittest.TestCase):

    def test_parse_args(self):
        args = get_ami.parse_args(['precise', 'amd64'])
        self.assertEqual('precise', args.series)
        self.assertEqual('amd64', args.arch)

    def test_query_ami(self):
        results = "ami-first\nami-second\nami-third\n"
        expected_args = [
            'sstream-query',
            get_ami.STREAM_INDEX,
            'endpoint~ec2.us-east-1.amazonaws.com',
            'arch=amd64',
            'release=precise',
            'root_store=ssd',
            'virt=pv',
            '--output-format', '%(id)s'
        ]
        with mock.patch("subprocess.check_output", return_value=results,
                        autospec=True) as co_mock:
            ami = get_ami.query_ami("precise", "amd64")
            self.assertEqual(ami, "ami-first")
        co_mock.assert_called_once_with(expected_args)

    def test_query_ami_different_region(self):
        results = "ami-first\nami-second\nami-third\n"
        expected_args = [
            'sstream-query',
            get_ami.STREAM_INDEX,
            'endpoint~ec2.cn-north-1.amazonaws.com',
            'arch=amd64',
            'release=trusty',
            'root_store=ssd',
            'virt=pv',
            '--output-format', '%(id)s'
        ]
        with mock.patch("subprocess.check_output", return_value=results,
                        autospec=True) as co_mock:
            ami = get_ami.query_ami("trusty", "amd64", region="cn-north-1")
            self.assertEqual(ami, "ami-first")
        co_mock.assert_called_once_with(expected_args)

    def test_query_ami_daily_stream(self):
        results = "ami-first\nami-second\nami-third\n"
        expected_args = [
            'sstream-query',
            get_ami.DAILY_INDEX,
            'endpoint~ec2.us-east-1.amazonaws.com',
            'arch=amd64',
            'release=trusty',
            'root_store=ssd',
            'virt=pv',
            '--output-format', '%(id)s'
        ]
        with mock.patch("subprocess.check_output", return_value=results,
                        autospec=True) as co_mock:
            ami = get_ami.query_ami("trusty", "amd64", stream="daily")
            self.assertEqual(ami, "ami-first")
        co_mock.assert_called_once_with(expected_args)

    def test_query_ami_optional_params(self):
        results = "ami-first\nami-second\nami-third\n"
        expected_args = [
            'sstream-query',
            get_ami.STREAM_INDEX,
            'endpoint~ec2.us-east-1.amazonaws.com',
            'arch=amd64',
            'release=trusty',
            'root_store=ebs',
            'virt=hvm',
            '--output-format', '%(id)s'
        ]
        with mock.patch("subprocess.check_output", return_value=results,
                        autospec=True) as co_mock:
            ami = get_ami.query_ami("trusty", "amd64", root_store="ebs",
                                    virt="hvm")
            self.assertEqual(ami, "ami-first")
        co_mock.assert_called_once_with(expected_args)

    def test_query_ami_label(self):
        results = "ami-first\nami-second\nami-third\n"
        expected_args = [
            'sstream-query',
            get_ami.STREAM_INDEX,
            'endpoint~ec2.us-east-1.amazonaws.com',
            'arch=amd64',
            'release=trusty',
            'label=release',
            'root_store=ssd',
            'virt=pv',
            '--output-format', '%(id)s'
        ]
        with mock.patch("subprocess.check_output", return_value=results,
                        autospec=True) as co_mock:
            ami = get_ami.query_ami("trusty", "amd64", label="release")
            self.assertEqual(ami, "ami-first")
        co_mock.assert_called_once_with(expected_args)

    def test_query_ami_missing_tool(self):
        error = OSError(errno.ENOENT, "not found")
        message = "sstream-query tool not found, is it installed?"
        with mock.patch("subprocess.check_output", side_effect=error,
                        autospec=True) as co_mock:
            with self.assertRaises(ValueError) as ctx:
                get_ami.query_ami("precise", "amd64")
            self.assertEqual(str(ctx.exception), message)
        self.assertEqual(co_mock.called, 1)

    def test_query_no_results(self):
        message = (
            "No amis for arch=amd64 release=precise root_store=ssd virt=pv"
            " in region=us-east-1"
        )
        with mock.patch("subprocess.check_output", return_value="",
                        autospec=True) as co_mock:
            with self.assertRaises(ValueError) as ctx:
                get_ami.query_ami("precise", "amd64")
            self.assertEqual(str(ctx.exception), message)
        self.assertEqual(co_mock.called, 1)
