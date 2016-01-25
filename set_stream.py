#!/usr/bin/env python

from argparse import ArgumentParser
import json
import os

from simplestreams.generate_simplestreams import json_dump


def main():
    parser = ArgumentParser()
    parser.add_argument('in_file')
    parser.add_argument('out_file')
    parser.add_argument('revision_build')
    parser.add_argument('--update-path', action='store_true')
    args = parser.parse_args()
    with open(args.in_file) as in_file:
        stanzas = json.load(in_file)
    stream = 'revision-build-{}'.format(args.revision_build)
    for stanza in stanzas:
        stanza['content_id'] = 'com.ubuntu.juju:{}:tools'.format(stream)
        if not args.update_path:
            continue
        path = os.path.join(
            'agent', 'revision-build-{}'.format(args.revision_build),
            os.path.basename(stanza['path']))
        stanza['path'] = path
    json_dump(stanzas, args.out_file)


if __name__ == '__main__':
    main()
