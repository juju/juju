from argparse import Namespace
from contextlib import contextmanager
from os.path import(
    join,
    isfile,
)
import json
from StringIO import StringIO
from tempfile import NamedTemporaryFile
from unittest import TestCase

from mock import (
    patch,
    call,
)

from sign_metadata import (
    get_gpg_options,
    get_meta_files,
    sign_metadata,
    parse_args,
    update_file_content,
)
from utils import (
    temp_dir,
    write_file,
)


__metaclass__ = type


class TestSignMetaData(TestCase):

    content = json.dumps(
        {'index':
            {'com.ubuntu.juju:released:tools':
                {'path': "streams/v1/com.ubuntu.juju-released-tools.json",
                 'format': 'products:1.0'}}
         })
    expected = json.dumps(
        {'index':
            {'com.ubuntu.juju:released:tools':
                {'path': "streams/v1/com.ubuntu.juju-released-tools.sjson",
                 "format": "products:1.0"}}
         })

    def test_get_meta_files(self):
        with temp_dir() as meta_dir:
            file1 = join(meta_dir, "index.json")
            file2 = join(meta_dir, "proposed-tools.json")
            file3 = join(meta_dir, "random.txt")
            open(file1, 'w').close()
            open(file2, 'w').close()
            open(file3, 'w').close()
            meta_files = get_meta_files(meta_dir)
            self.assertItemsEqual(
                meta_files, ['index.json', 'proposed-tools.json'])

    def test_gpg_options(self):
        output = get_gpg_options('thedude@example.com')
        expected = ("--default-key thedude@example.com", '--no-tty')
        self.assertEqual(output, expected)

    def test_gpg_options_signing_passphrase_file(self):
        output = get_gpg_options('thedude@example.com', '/tmp')
        expected = ("--default-key thedude@example.com",
                    '--no-use-agent --no-tty --passphrase-file /tmp')
        self.assertEqual(output, expected)

    def test_update_file_content(self):
        with NamedTemporaryFile() as meta_file:
            with NamedTemporaryFile() as dst_file:
                write_file(meta_file.name, self.content)
                update_file_content(meta_file.name, dst_file.name)
                with open(dst_file.name) as dst:
                    dst_file_content = dst.read()
                self.assertEqual(dst_file_content, self.expected)

    def test_sign_metadata(self):
        with patch('sign_metadata.run', autospec=True,
                   side_effect=self.fake_gpg) as smr:
            with temp_dir() as meta_dir:
                meta_file = join(meta_dir, 'index.json')
                write_file(meta_file, self.content)
                with NamedTemporaryFile() as temp_file:
                    with patch('sign_metadata.NamedTemporaryFile',
                               autospec=True, return_value=temp_file) as ntf:
                        sign_metadata('thedude@example.com', meta_dir)
                        self.verify_signed_content(meta_dir)
        signed_file = meta_file.replace('.json', '.sjson')
        gpg_file = '{}.gpg'.format(meta_file)
        calls = [
            call(['gpg', '--no-tty', '--clearsign', '--default-key',
                  'thedude@example.com', '-o', signed_file, temp_file.name]),
            call(['gpg', '--no-tty', '--detach-sign', '--default-key',
                  'thedude@example.com', '-o', gpg_file, meta_file])]
        self.assertEqual(smr.mock_calls, calls)
        ntf.assert_called_once_with()

    def test_sign_metadata_signing_passphrase_file(self):
        with patch('sign_metadata.run', autospec=True,
                   side_effect=self.fake_gpg) as smr:
            with temp_dir() as meta_dir:
                meta_file = join(meta_dir, 'index.json')
                write_file(meta_file, self.content)
                with NamedTemporaryFile() as temp_file:
                    with patch('sign_metadata.NamedTemporaryFile',
                               autospec=True, return_value=temp_file) as ntf:
                        sign_metadata(
                            'thedude@example.com', meta_dir, 'passphrase_file')
                        self.verify_signed_content(meta_dir)
        signed_file = meta_file.replace('.json', '.sjson')
        gpg_file = '{}.gpg'.format(meta_file)
        calls = [
            call(['gpg', '--no-use-agent', '--no-tty', '--passphrase-file',
                  'passphrase_file', '--clearsign', '--default-key',
                  'thedude@example.com', '-o', signed_file, temp_file.name]),
            call(['gpg', '--no-use-agent', '--no-tty', '--passphrase-file',
                  'passphrase_file', '--detach-sign', '--default-key',
                  'thedude@example.com', '-o', gpg_file, meta_file])]
        self.assertEqual(smr.mock_calls, calls)
        ntf.assert_called_once_with()

    def verify_signed_content(self, meta_dir):
        file_path = join(meta_dir, 'index.sjson')
        with open(file_path) as i:
            signed_content = i.read()
        self.assertEqual(signed_content, '{}{}{}'.format(
            self.gpg_header(), self.expected, self.gpg_footer()))
        self.assertTrue(
            isfile(join(meta_dir, 'index.json.gpg')))

    def gpg_header(self):
        return '-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA1\n'

    def gpg_footer(self):
        return ('\n-----BEGIN PGP SIGNATURE-----\nblah blah\n'
                '-----END PGP SIGNATURE-----')

    def fake_gpg(self, args):
        output_file = args[6]
        input_file = args[7]
        if '--passphrase-file' in args:
            output_file = args[9]
            input_file = args[10]
        if '--clearsign' in args:
            with open(input_file) as in_file:
                with open(output_file, 'w') as out_file:
                    content = in_file.read()
                    out_file.write('{}{}{}'.format(
                        self.gpg_header(), content, self.gpg_footer()))
        if '--detach-sign' in args:
            open(output_file, 'w').close()

    def test_parse_args_default(self):
        with temp_dir() as metadata_dir:
            args = parse_args([metadata_dir, 's_key'])
        self.assertEqual(args, Namespace(
            signing_key='s_key', signing_passphrase_file=None,
            metadata_dir=metadata_dir))

    def test_parse_args_signing_passphrase_file(self):
        with temp_dir() as metadata_dir:
            with NamedTemporaryFile() as pass_file:
                args = parse_args([metadata_dir, 's_key',
                                   '--signing-passphrase-file', pass_file.name])
        self.assertEqual(args, Namespace(
            signing_key='s_key', signing_passphrase_file=pass_file.name,
            metadata_dir=metadata_dir))

    def test_parse_args_error(self):
        with parse_error(self) as stderr:
            parse_args([])
        self.assertIn("error: too few arguments", stderr.getvalue())

        with parse_error(self) as stderr:
            parse_args(['metadata_dir', 'signing_key'])
        self.assertIn("Invalid metadata directory path", stderr.getvalue())

        with parse_error(self) as stderr:
            with temp_dir() as metadata_dir:
                parse_args([metadata_dir, 's_key', '--signing-passphrase-file',
                            'fake/file/'])
        self.assertIn("Invalid passphrase file path ", stderr.getvalue())


@contextmanager
def parse_error(test_case):
    stderr = StringIO()
    with test_case.assertRaises(SystemExit):
        with patch('sys.stderr', stderr):
            yield stderr
