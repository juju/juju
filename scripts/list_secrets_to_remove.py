import json
import subprocess
import tempfile


mongoExecCmds = '''
conf=/var/lib/juju/agents/machine-*/agent.conf
user=$(sudo grep '^tag:' ${conf} | cut -d ' ' -f2)
password=$(sudo awk '/statepassword/ {print $2}' ${conf})
client=/snap/bin/juju-db.mongo
# We use `--eval "$(cat exec.js)"` to get around snap confinement issues
${client} 127.0.0.1:37017/juju --authenticationDatabase admin \
    --ssl --sslAllowInvalidCertificates \
    --username "${user}" --password "${password}" --eval "$(cat /home/ubuntu/exec.js)"
'''


mongoScript = '''
cursor = db.secretRevisions.aggregate([
  {
    "$match": {
      obsolete: true,
      "value-reference": { $exists: false },
      "data": { $exists: true }
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
        // _id is of the form <model-uuid>:<secret-id>/<secret-revision>
        // we want to grab the part from the ':' to the '/'.
        // so split on ':' taking the second part, and '/' taking the first part
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
        model: "$model.name",
        uuid: "$model-uuid",
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
  },
  {
    $match: {
      count: { $gte: 1 },
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


def format_juju_remove(model, uuid, secret, owner, local_revs, is_model_secret):
    local_rev_str = ' '.join(map(str, local_revs))
    if is_model_secret:
        if len(local_revs) == 1:
            print(f"juju remove-secret -m {model} secret:{secret} --revision {local_rev_str}")
        else:
            print(f"for r in {local_rev_str}; do juju remove-secret -m {model} secret:{secret} --revision $r; done")
    else:
        if len(local_revs) == 1:
            print(f"juju exec -m {model} --unit {owner} -- secret-remove {secret} --revision {local_rev_str}")
        else:
            print(f"for r in {local_rev_str}; do juju exec -m {model} --unit {owner} -- secret-remove {secret} --revision $r; done")


def format_db_remove(model, uuid, secret, owner, local_revs, is_model_secret):
    if len(local_revs) == 1:
        secret_id = f"{uuid}:{secret}/{local_revs[0]}"
        print(f'd.secretRevisions.deleteOne({{ "_id": "{secret_id}" }})')
    else:
        # This is the regex that deleteSecrets uses to delete many revs of one secret
        regexTail = '|'.join([f"{rev}" for rev in local_revs])
        regex = f"^{uuid}:{secret}/({regexTail})$"
        print(f'd.secretRevisions.deleteMany({{ "_id": {{ $regex: "{regex}" }} }})')


    
def main(args):
    import argparse
    p = argparse.ArgumentParser("simple script for removing a lot of secrets")
    p.add_argument("--controller", default="0", type=str, help="Juju controller machine id")
    p.add_argument("--secret-list", default=None, type=str, help="Path to a database output, rather than shelling out to mongo")
    p.add_argument("--batch", default=1, type=int, help="when printing out loops, do batches no larger than this (<=0 does all)")
    p.add_argument("--min", default=1, type=int, help="only request a cleanup if there are more than 'min' unused secrets")
    p.add_argument("--db", default=False, action="store_true", help="output mongodb cleanup rather than `juju remove` commands")
    opts = p.parse_args(args)
    raw = read_revisions(opts)
    total_count = 0
    formatter = format_juju_remove
    if opts.db:
        formatter = format_db_remove
        print('s = db.getMongo().startSession()')
        print('s.startTransaction()')
        print('d = s.getDatabase("juju")')
    for r in raw:
        model = r["_id"]["model"]
        uuid = r["_id"]["uuid"]
        owner = r["_id"]["owner"]
        is_model_secret = False
        if owner.startswith("application"):
            owner = owner.replace("application-", "") + "/leader"
        elif owner.startswith("unit"):
            owner = owner.replace("unit-", "") 
            tail = owner.rindex("-")
            owner = owner[:tail] + '/' + owner[tail+1:]
        elif owner.startswith("model"):
            owner = owner.replace("model-", "")
            is_model_secret = True
        secret = r["_id"]["secret"]
        revs = r["revs"]
        revs.sort()
        batch = opts.batch
        if opts.batch <= 0:
            batch = len(revs)
        if len(revs) < opts.min:
            continue
        total_count += len(revs)
        for i in range(0, len(revs), batch):
            local_revs = revs[i:i+batch]
            formatter(model, uuid, secret, owner, local_revs, is_model_secret)
    if opts.db:
        print('s.commitTransaction()')
        print('s.endSession()')
    print("Total Count:", total_count)
            


if __name__ == "__main__":
    import sys
    main(sys.argv[1:])
