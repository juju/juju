# -*- coding: utf-8 -*-

import sys, yaml, pyaml


def main(argv=None):
	import argparse
	parser = argparse.ArgumentParser(
		description='Process and dump prettified YAML to stdout.')
	parser.add_argument('path', nargs='?', metavar='path',
		help='Path to YAML to read (default: use stdin).')
	parser.add_argument('-w', '--width', type=int, metavar='chars',
		help='Max line width hint to pass to pyyaml for the dump.'
			' Only used to format scalars and collections (e.g. lists).')
	opts = parser.parse_args(argv or sys.argv[1:])

	src = open(opts.path) if opts.path else sys.stdin
	try: data = yaml.safe_load(src)
	finally: src.close()

	pyaml_kwargs = dict()
	if opts.width: pyaml_kwargs['width'] = opts.width
	pyaml.pprint(data, **pyaml_kwargs)

if __name__ == '__main__': sys.exit(main())
