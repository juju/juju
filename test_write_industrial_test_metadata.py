import json
from mock import patch
import os.path
from textwrap import dedent
from unittest import TestCase

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
                parse_args(['foo'])

    def test_parse_args(self):
        args = parse_args(['foo', 'bar'])
        self.assertItemsEqual(['buildvars', 'output'],
                              [a for a in dir(args) if not a.startswith('_')])
        self.assertEqual(args.buildvars, 'foo')
        self.assertEqual(args.output, 'bar')

    def test_make_metadata(self):
        with temp_dir() as tempdir:
            buildvars_path = os.path.join(tempdir, 'buildvars')
            with open(buildvars_path, 'w') as buildvars_file:
                json.dump({'foo': 'bar'}, buildvars_file)
            with patch('subprocess.check_output', return_value='1.20'):
                metadata = make_metadata(buildvars_path)
        self.assertEqual(metadata, {
            'new_client': {
                'buildvars': {'foo': 'bar'},
                'type': 'build',
                },
            'old_client': {
                'type': 'release',
                'version': '1.20',
                },
            })

    def test_main(self):
        with temp_dir() as tempdir:
            buildvars_path = os.path.join(tempdir, 'buildvars')
            with open(buildvars_path, 'w') as buildvars_file:
                json.dump({'foo': 'bar'}, buildvars_file)
            output_path = os.path.join(tempdir, 'output')
            with patch('subprocess.check_output', return_value='1.20'):
                main([buildvars_path, output_path])
            with open(output_path) as output_file:
                output = output_file.read()
            expected = dedent("""\
                {
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
