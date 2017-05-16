import json
from mock import patch
import os.path
from textwrap import dedent
from unittest import TestCase

from jujupy import _temp_env
from utility import temp_dir
from write_industrial_test_metadata import (
    main,
    make_metadata,
    parse_args,
)


class TestWriteIndustrialTestMetadata(TestCase):

    def test_parse_args_insufficiennt_args(self):
        with patch('sys.stderr'):
            with self.assertRaises(SystemExit):
                parse_args(['foo', 'bar'])

    def test_parse_args(self):
        args = parse_args(['foo', 'bar', 'baz'])
        self.assertItemsEqual(['buildvars', 'env', 'output'],
                              [a for a in dir(args) if not a.startswith('_')])
        self.assertEqual(args.buildvars, 'foo')
        self.assertEqual(args.env, 'bar')
        self.assertEqual(args.output, 'baz')

    def test_make_metadata(self):
        with temp_dir() as tempdir:
            buildvars_path = os.path.join(tempdir, 'buildvars')
            with open(buildvars_path, 'w') as buildvars_file:
                json.dump({'foo': 'bar'}, buildvars_file)
            with patch('subprocess.check_output', return_value='1.20'):
                envs = {'environments': {'foobar': {'type': 'local'}}}
                with _temp_env(envs):
                    metadata = make_metadata(buildvars_path, 'foobar')
        self.assertEqual(metadata, {
            'new_client': {
                'buildvars': {'foo': 'bar'},
                'type': 'build',
                },
            'old_client': {
                'type': 'release',
                'version': '1.20',
                },
            'environment': {
                'name': 'foobar',
                'substrate': 'LXC (local)',
                }
            })

    def test_main(self):
        with temp_dir() as tempdir:
            buildvars_path = os.path.join(tempdir, 'buildvars')
            with open(buildvars_path, 'w') as buildvars_file:
                json.dump({'foo': 'bar'}, buildvars_file)
            output_path = os.path.join(tempdir, 'output')
            envs = {'environments': {'foo': {'type': 'ec2'}}}
            with _temp_env(envs):
                with patch('subprocess.check_output', return_value='1.20'):
                    main([buildvars_path, 'foo', output_path])
            with open(output_path) as output_file:
                output = output_file.read()
            expected = dedent("""\
                {
                  "environment": {
                    "name": "foo",\x20
                    "substrate": "AWS"
                  },\x20
                  "new_client": {
                    "buildvars": {
                      "foo": "bar"
                    },\x20
                    "type": "build"
                  },\x20
                  "old_client": {
                    "type": "release",\x20
                    "version": "1.20"
                  }
                }
                """)
            self.assertEqual(output, expected)
