#!/bin/bash

set -e

#This script script is intended to run only against controller machines that
#have upgraded from a MAAS 1.9 provider to a MAAS 2.x provider. Do not use this
#script otherwise. The javascript below is horrible. It is designed to run
#operations in bulk targeting a minimum version of mongo 3.2 which has little
#functionality in the way of aggregation.

usage() { echo "usage: $0 [-rh]" 1>&2; exit 1; }

while getopts 'rh' flag; do
    case "${flag}" in
        r) rollback_flag=true
           ;;
        h) usage
           ;;
        *) usage
           ;;
    esac
done

if [ $rollback_flag ]
then
    #This script warps a MAAS 1.9 style URL around the instance_id. You can run
    #this script with the -r flag for convenience but you should really just
    #reload the backup that you have created with mongoexport or otherwise if
    #you run into trouble.
    mongo_script=$(cat <<'JAVASCRIPT'
db.instanceData.aggregate([{
    "$project": {
        instanceid: {
            "$cond": {
                if: {
                    "$ne": [{
                        "$substr": ["$instanceid", 0, 20]
                    }, "/MAAS/api/1.0/nodes/"]
                }
                , then: {
                    "$concat": ["/MAAS/api/1.0/nodes/", "$instanceid", "/"]
                }
                , else: "$instanceid"
            }
        }
        , document: "$$ROOT"
    }
}, {
    "$out": "tmp"
}]);
ops = db.instanceData.find().map(function (item) {
    newid = db.tmp.findOne({
        "document.instanceid": item.instanceid
    });
    return {
        "updateOne": {
            "filter": {
                instanceid: newid.document.instanceid
            }
            , "update": {
                "$set": {
                    instanceid: newid.instanceid
                }
            }
        }
    }
});
db.instanceData.bulkWrite(ops);
JAVASCRIPT
)
else
    #This script drops the URL wrapping the instance which coerces the data into
    #MAAS 2.0 format
    mongo_script=$(cat <<'JAVASCRIPT'
db.instanceData.aggregate([{
    "$project": {
        instanceid: {
            "$cond": {
                if: {
                    "$eq": [{
                        "$substr": ["$instanceid", 0, 20]
                    }, "/MAAS/api/1.0/nodes/"]
                }
                , then: {
                    "$substr": ["$instanceid", 20, -1]
                }
                , else: "$instanceid"
            }
        }
        , document: "$$ROOT"
    }
}, {
    "$out": "tmp"
}]);
ops = db.instanceData.find().map(function (item) {
    newid = db.tmp.findOne({
        "document.instanceid": item.instanceid
    });
    if (item.instanceid.slice(-1) === "/") {
        return {
            "updateOne": {
                "filter": {
                    instanceid: newid.document.instanceid
                }
                , "update": {
                    "$set": {
                        instanceid: newid.instanceid.slice(0, -1)
                    }
                }
            }
        }
    } else {
        return {}
    }
});
db.instanceData.bulkWrite(ops);
JAVASCRIPT
)
fi

#prepare mongoscript for evaluation
script=$(echo $mongo_script | tr -d ' ' | tr -d '[:blank:]' | tr -d '[:space:]')

agent=$(cd /var/lib/juju/agents; echo machine-*)
pw=$(sudo grep statepassword /var/lib/juju/agents/${agent}/agent.conf | cut '-d ' -sf2)
/usr/lib/juju/mongo3.2/bin/mongo --ssl -u ${agent} -p $pw --authenticationDatabase admin --sslAllowInvalidHostnames --sslAllowInvalidCertificates localhost:37017/juju --eval "$script"
