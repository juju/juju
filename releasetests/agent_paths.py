#!/usr/bin/env python3
from argparse import ArgumentParser
import json
import re
import sys

from simplestreams.generate_simplestreams import json_dump


def main():
    parser = ArgumentParser()
    parser.add_argument('input')
    parser.add_argument('output')
    args = parser.parse_args()
    paths_hashes = {}
    with open(args.input) as input_file:
        stanzas = json.load(input_file)
    hashes = {}
    old_hash_urls = {}
    for stanza in stanzas:
        path_hash = stanza['sha256']
        old_hash_urls[path_hash] = stanza['item_url']
        agent_filename = stanza['path'].split('/')[-1]
        path = 'agent/{}/{}'.format(stanza['version'], agent_filename)
        path = re.sub('-win(2012(hv)?(r2)?|2016(nano)?|7|8|81|10)-',
                      '-windows-', path)
        paths_hashes.setdefault(path, stanza['sha256'])
        if paths_hashes[path] != path_hash:
            raise ValueError('Conflicting hash')
        stanza['path'] = path
        hashes[path] = path_hash
    ph_list = {}
    for path, path_hash in hashes.items():
        ph_list.setdefault(path_hash, set()).add(path)
    for path_hash, paths in ph_list.items():
        if len(paths) > 1:
            print(paths)
    json_dump(stanzas, args.output)
    agent_downloads = []
    for stanza in stanzas:
        agent_downloads.append({
            'path': stanza['path'],
            'sha256': stanza['sha256'],
            'url': old_hash_urls[stanza['sha256']],
        })
    json_dump(agent_downloads, 'downloads-' + args.output)

if __name__ == '__main__':
    sys.exit(main())
