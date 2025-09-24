# Copyright 2025 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

"""This generates commands that can be run to cleanup unused secrets from a juju controller.

The script uses `juju ssh -m controller 0` to run some mongo commands to get a
dump of all the obsolete revisions for all applications across all models, and
then turns that into bash lines that can be used to `juju exec ... secret-remove` to actually
trigger the secrets being cleaned up.

Note that at present, `secret-remove` will only ever schedule a single revision to be cleaned
up, so we have to run many batches. Though you could run some of these commands
in parallel to speed up the execution.

This command does actually remove any secrets directly.
"""

import json
import subprocess
import tempfile


mongoExecCmds = '''
conf=/var/lib/juju/agents/machine-*/agent.conf
user=$(sudo awk '/tag/ {print $2}' ${conf})
password=$(sudo awk '/statepassword/ {print $2}' ${conf})
if [ -f /snap/bin/juju-db.mongo ]; then
    client=/snap/bin/juju-db.mongo
elif [ -f /usr/lib/juju/mongo*/bin/mongo ]; then
    client=/usr/lib/juju/mongo*/bin/mongo
else
    client=/usr/bin/mongo
fi
${client} 127.0.0.1:37017/juju --authenticationDatabase admin \
    --ssl --sslAllowInvalidCertificates \
    --username "${user}" --password "${password}" exec.js
'''


mongoScript = '''
cursor = db.secretRevisions.aggregate([
  {
    "$match": {
      obsolete: true
    }
  },
  {
    $lookup: {
      from: "models",
      localField: "model-uuid",
      foreignField: "_id",
      as: "model"
    }
  },
  { $unwind: "$model" },
    {
    $addFields: {
      secret: {
        $arrayElemAt: [
          { $split: [ { $arrayElemAt: [ { $split: ["$_id", ":"] }, 1 ] }, "/" ] },
          0
        ]
      }
    }
  },
  {
    $group: {
      "_id": {
        "model": "$model.name",
        secret: "$secret",
        owner: "$owner-tag"
      },
      count: {
        $sum: 1
      },
      revs: {
        $addToSet: "$revision"
      }
    }
  }
]);
print("----- start of content");
var docs = cursor.toArray();
printjson(docs);
'''


def copy_file_to_controller(opts, local_fname):
    subprocess.check_call(["juju", "scp", "-m", "controller", local_fname, f"{opts.controller}:exec.js"])

def strip_mongo_output(opts, content):
    """Turn the raw output from mongo into just the json tail"""
    start = "----- start of content\n"
    return content[content.index(start)+len(start):]

def exec_mongo_request(opts, mongo_request):
    local_file = tempfile.NamedTemporaryFile(dir='.', suffix=".js")
    local_file.write(bytes(mongoScript, 'utf-8'))
    local_file.flush()
    copy_file_to_controller(opts, local_file.name)
    local_file.close()
    content = subprocess.check_output(["juju", "ssh", "-m", "controller", opts.controller, mongo_request], text=True)
    return strip_mongo_output(opts, content)

def read_revisions(opts):
    if opts.secret_list:
        with open(opts.secret_list, "r") as f:
            return json.loads(strip_mongo_output(opts, f.read()))
    revisions = json.loads(exec_mongo_request(opts, mongoExecCmds))
    return revisions

    
def main(args):
    import argparse
    p = argparse.ArgumentParser("simple script for removing a lot of secrets")
    p.add_argument("--controller", default="0", type=str, help="Juju controller machine id")
    p.add_argument("--secret-list", default=None, type=str, help="Path to a database output, rather than shelling out to mongo")
    p.add_argument("--bash-compress", default=False, action="store_true", help="Output for loops in bash, rather than individual commands")
    p.add_argument("--batch", default=0, type=int, help="when printing out loops, do batches no larger than this (<=0 does all)")
    opts = p.parse_args(args)
    raw = read_revisions(opts)
    for r in raw:
        model = r["_id"]["model"]
        owner = r["_id"]["owner"]
        if owner.startswith("application"):
            owner = owner.replace("application-", "") + "/leader"
        elif owner.startswith("unit"):
            print(owner)
            owner = owner.replace("unit-", "") 
            tail = owner.rindex("-")
            owner = owner[:tail] + '/' + owner[tail:]
        secret = r["_id"]["secret"]
        revs = r["revs"]
        revs.sort()
        if opts.bash_compress:
            if opts.batch > 0:
                for i in range(0, len(revs), opts.batch):
                    local_revs = revs[i:i+opts.batch]
                    print(f"for r in {' '.join(map(str, local_revs))}; do juju exec -m {model} --unit {owner} -- secret-remove {secret} --revision $r; done")
            else:
                print(f"for r in {' '.join(map(str, revs))}; do juju exec -m {model} --unit {owner} -- secret-remove {secret} --revision $r; done")
        else:
            for r in revs:
                print(f"juju exec -m {model} --unit {owner} -- secret-remove {secret} --revision {r}")
            


if __name__ == "__main__":
    import sys
    main(sys.argv[1:])
