from argparse import Namespace
from contextlib import contextmanager
from os import makedirs
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
    main,
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
        with temp_dir() as dest_dist:
            dir_path = join(dest_dist, 'tools', 'streams', 'v1')
            makedirs(dir_path)
            file1 = join(dir_path, "index.json")
            file2 = join(dir_path, "proposed-tools.json")
            file3 = join(dir_path, "random.txt")
            open(file1, 'w').close()
            open(file2, 'w').close()
            open(file3, 'w').close()
            meta_files = get_meta_files(dest_dist)
            self.assertItemsEqual(
                meta_files, (dir_path, ['index.json', 'proposed-tools.json']))

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
                   side_effect=self.my_gpg) as smr:
            with temp_dir() as stream_dir:
                dest_dist = join(stream_dir, 'juju-dist', 'devel')
                meta_dir = join(dest_dist, 'tools', 'streams', 'v1')
                makedirs(meta_dir)
                meta_file = join(meta_dir, 'index.json')
                write_file(meta_file, self.content)
                with NamedTemporaryFile() as temp_file:
                    with patch('sign_metadata.NamedTemporaryFile',
                               autospec=True, return_value=temp_file) as ntf:
                        sign_metadata('thedude@example.com', dest_dist, 'devel')
                        self.verify_signed_content(meta_dir)
        self.verify_calls(smr.mock_calls, meta_file, temp_file.name)
        ntf.assert_called_once_with()

    def test_sign_metadata_weekly(self):
        with patch('sign_metadata.run', autospec=True,
                   side_effect=self.my_gpg) as smr:
            with temp_dir() as stream_dir:
                dest_dist = join(stream_dir, 'juju-dist', 'weekly')
                juju_dist = join(stream_dir, 'juju-dist')
                dest_dir = join(dest_dist, 'tools', 'streams', 'v1')
                dist_dir = join(juju_dist, 'tools', 'streams', 'v1')
                makedirs(dest_dir)
                makedirs(dist_dir)
                dest_meta_file = join(dest_dir, 'index.json')
                juju_dist_file = join(dist_dir, 'index.json')
                write_file(dest_meta_file, self.content)
                write_file(juju_dist_file, self.content)
                with NamedTemporaryFile() as temp_file:
                    with NamedTemporaryFile() as temp_file2:
                        with patch('sign_metadata.NamedTemporaryFile',
                                   autospec=True,
                                   side_effect=[temp_file, temp_file2]):
                            sign_metadata('thedude@example.com', dest_dist,
                                          'weekly', juju_dist=juju_dist)
                            self.verify_signed_content(dest_dir)
                            self.verify_signed_content(dist_dir)
        dest_signed_file = dest_meta_file.replace('.json', '.sjson')
        dest_gpg_file = '{}.gpg'.format(dest_meta_file)
        juju_signed_file = juju_dist_file.replace('.json', '.sjson')
        juju_gpg_file = '{}.gpg'.format(juju_dist_file)
        calls = [
            call(['gpg', '--no-tty', '--clearsign', '--default-key',
                  'thedude@example.com', '-o',  dest_signed_file,
                  temp_file.name]),
            call(['gpg', '--no-tty', '--detach-sign', '--default-key',
                  'thedude@example.com', '-o', dest_gpg_file, temp_file.name]),
            call(['gpg', '--no-tty', '--clearsign', '--default-key',
                  'thedude@example.com', '-o', juju_signed_file,
                  temp_file2.name]),
            call(['gpg', '--no-tty', '--detach-sign', '--default-key',
                  'thedude@example.com', '-o', juju_gpg_file,
                  temp_file2.name])]
        self.assertEqual(smr.mock_calls, calls)

    def verify_signed_content(self, meta_dir):
        file_path = join(meta_dir, 'index.sjson')
        with open(file_path) as i:
            signed_content = i.read()
        self.assertEqual(signed_content, '{}{}{}'.format(
            self.gpg_header(), self.expected, self.gpg_footer()))
        self.assertTrue(
            isfile(join(meta_dir, 'index.json.gpg')))

    def verify_calls(self, mock_calls, meta_file, temp_file):
        signed_file = meta_file.replace('.json', '.sjson')
        gpg_file = '{}.gpg'.format(meta_file)
        calls = [
            call(['gpg', '--no-tty', '--clearsign', '--default-key',
                  'thedude@example.com', '-o', signed_file, temp_file]),
            call(['gpg', '--no-tty', '--detach-sign', '--default-key',
                  'thedude@example.com', '-o', gpg_file, temp_file])]
        self.assertEqual(mock_calls, calls)

    def gpg_header(self):
        return '-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA1\n'

    def gpg_footer(self):
        return ('\n-----BEGIN PGP SIGNATURE-----\nblah blah\n'
                '-----END PGP SIGNATURE-----')

    def my_gpg(self, arg):
        output_file = arg[6]
        input_file = arg[7]
        if '--clearsign' in arg:
            with open(input_file) as i:
                with open(output_file, 'w') as o:
                    content = i.read()
                    o.write('{}{}{}'.format(
                        self.gpg_header(), content, self.gpg_footer()))
        if '--detach-sign' in arg:
            open(output_file, 'w').close()

    def test_parse_args_default(self):
        with temp_dir() as stream_dir:
            args = parse_args(['devel', stream_dir, 's_key'])
        self.assertEqual(args, Namespace(
            purpose='devel', signing_key='s_key', signing_passphrase_file=None,
            stream_dir=stream_dir))

    def test_parse_args_set_options(self):
        with temp_dir() as stream_dir:
            with NamedTemporaryFile() as pass_file:
                args = parse_args(['devel', stream_dir, 's_key',
                                   '--signing-passphrase-file', pass_file.name])
        self.assertEqual(args, Namespace(
            purpose='devel', signing_key='s_key',
            signing_passphrase_file=pass_file.name, stream_dir=stream_dir))

    def test_parse_args_error(self):
        with parse_error(self) as stderr:
            parse_args()
        self.assertIn(
            "choose from 'devel', 'proposed', 'released', 'testing', 'weekly'",
            stderr.getvalue())

        with parse_error(self) as stderr:
            parse_args(['proposed'])
        self.assertIn("error: too few arguments", stderr.getvalue())

        with parse_error(self) as stderr:
            parse_args(['proposed', 'stream_dir', 'signing_key'])
        self.assertIn("Invalid stream directory path", stderr.getvalue())

        with parse_error(self) as stderr:
            with temp_dir() as stream_dir:
                parse_args(['devel', stream_dir, 's_key',
                            '--signing-passphrase-file', 'no/file/'])
        self.assertIn("Invalid passphrase file path ", stderr.getvalue())

    def test_main(self):
        with patch('sign_metadata.sign_metadata', autospec=True) as sm:
            with temp_dir() as stream_dir:
                with NamedTemporaryFile() as pass_file:
                    args = Namespace(
                        purpose='devel', signing_key='s_key',
                        signing_passphrase_file=pass_file.name,
                        stream_dir=stream_dir)
                    main(args)
        dest_dist = join(stream_dir, 'juju-dist', 'devel')
        juju_dist = join(stream_dir, 'juju-dist')
        sm.assert_called_once_with(
            's_key', dest_dist, 'devel', juju_dist,  pass_file.name)

    def test_main_released(self):
        with patch('sign_metadata.sign_metadata', autospec=True) as sm:
            with temp_dir() as stream_dir:
                with NamedTemporaryFile() as pass_file:
                    args = Namespace(
                        purpose='released', signing_key='s_key',
                        signing_passphrase_file=pass_file.name,
                        stream_dir=stream_dir)
                    main(args)
        dest_dist = join(stream_dir, 'juju-dist')
        juju_dist = join(stream_dir, 'juju-dist')
        sm.assert_called_once_with(
            's_key', dest_dist, 'released', juju_dist,  pass_file.name)


@contextmanager
def parse_error(test_case):
    stderr = StringIO()
    with test_case.assertRaises(SystemExit):
        with patch('sys.stderr', stderr):
            yield stderr
