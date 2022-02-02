module github.com/juju/juju

go 1.17

require (
	github.com/Azure/azure-sdk-for-go v46.4.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.6
	github.com/Azure/go-autorest/autorest/adal v0.9.4
	github.com/Azure/go-autorest/autorest/date v0.3.0
	github.com/Azure/go-autorest/autorest/mocks v0.4.1
	github.com/Azure/go-autorest/autorest/to v0.3.0
	github.com/EvilSuperstars/go-cidrman v0.0.0-20170211231153-4e5a4a63d9b7
	github.com/altoros/gosigma v0.0.0-20150408145232-31228935eec6
	github.com/armon/go-metrics v0.3.3
	github.com/aws/aws-sdk-go-v2 v1.9.1
	github.com/aws/aws-sdk-go-v2/config v1.3.0
	github.com/aws/aws-sdk-go-v2/credentials v1.2.1
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.9.0
	github.com/aws/aws-sdk-go-v2/service/ecr v1.6.0
	github.com/aws/aws-sdk-go-v2/service/iam v1.9.0
	github.com/aws/smithy-go v1.8.0
	github.com/bmizerany/pat v0.0.0-20160217103242-c068ca2f0aac
	github.com/canonical/pebble v0.0.0-20220105032148-3a48156fcbf0
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e
	github.com/coreos/go-systemd/v22 v22.0.0-20200316104309-cb8b64719ae3
	github.com/docker/distribution v2.7.1+incompatible
	github.com/dustin/go-humanize v1.0.0
	github.com/go-goose/goose/v4 v4.0.0-20211105130700-d0015d23b748
	github.com/go-logr/logr v0.2.0
	github.com/go-macaroon-bakery/macaroon-bakery/v3 v3.0.0-20210309064400-d73aa8f92aa2
	github.com/golang/mock v1.5.0
	github.com/google/go-querystring v1.0.0
	github.com/googleapis/gnostic v0.4.1
	github.com/gorilla/schema v0.0.0-20160426231512-08023a0215e7
	github.com/gorilla/websocket v1.4.2
	github.com/gosuri/uitable v0.0.1
	github.com/hashicorp/go-hclog v0.9.1
	github.com/hashicorp/go-msgpack v0.5.5
	github.com/hashicorp/raft v1.3.2-0.20210825230038-1a621031eb2b
	github.com/hashicorp/raft-boltdb v0.0.0-20171010151810-6e5ba93211ea
	github.com/im7mortal/kmutex v1.0.1
	github.com/juju/ansiterm v0.0.0-20210929141451-8b71cc96ebdc
	github.com/juju/blobstore/v2 v2.0.0-20210302045357-edd2b24570b7
	github.com/juju/charm/v9 v9.0.0-20220131034618-d197db1afa3d
	github.com/juju/charmrepo/v7 v7.0.0-20210923152136-0ae2f26643bf
	github.com/juju/clock v0.0.0-20190205081909-9c5c9712527c
	github.com/juju/cmd/v3 v3.0.0-20210809234809-65029dab4cd0
	github.com/juju/collections v0.0.0-20200605021417-0d0ec82b7271
	github.com/juju/description/v3 v3.0.0-20220125161928-cc5932c29986
	github.com/juju/errors v0.0.0-20210818161939-5560c4c073ff
	github.com/juju/featureflag v0.0.0-20200423045028-e2f9e1cb1611
	github.com/juju/gnuflag v0.0.0-20171113085948-2ce1bb71843d
	github.com/juju/gojsonschema v0.0.0-20150312170016-e1ad140384f2
	github.com/juju/gomaasapi/v2 v2.0.0-20210323144809-92beddd020fe
	github.com/juju/http/v2 v2.0.0-20210623153641-418e83b0dab4
	github.com/juju/idmclient/v2 v2.0.0-20210309081103-6b4a5212f851
	github.com/juju/jsonschema v0.0.0-20210422141032-b0ff291abe9c
	github.com/juju/jsonschema-gen v0.0.0-20200416014454-d924343d72b2
	github.com/juju/loggo v0.0.0-20210728185423-eebad3a902c4
	github.com/juju/lumberjack v2.0.0-20200420012306-ddfd864a6ade+incompatible
	github.com/juju/mgo/v2 v2.0.0-20220111072304-f200228f1090
	github.com/juju/mutex/v2 v2.0.0-20220128011612-57176ebdcfa3
	github.com/juju/names/v4 v4.0.0-20200929085019-be23e191fee0
	github.com/juju/naturalsort v0.0.0-20180423034842-5b81707e882b
	github.com/juju/os/v2 v2.2.0
	github.com/juju/packaging/v2 v2.0.0-20210628104420-5487e24f1350
	github.com/juju/persistent-cookiejar v0.0.0-20170428161559-d67418f14c93
	github.com/juju/proxy v0.0.0-20210817195502-c6015cfe0258
	github.com/juju/pubsub/v2 v2.0.0-20220104155641-7af8a09f58f0
	github.com/juju/ratelimit v1.0.2-0.20191002062651-f60b32039441
	github.com/juju/replicaset/v2 v2.0.1-0.20211125220232-7967ce535201
	github.com/juju/retry v0.0.0-20210818141810-5526f6f6ff07
	github.com/juju/rfc/v2 v2.0.0-20210319034215-ed820200fad3
	github.com/juju/romulus v0.0.0-20210309074704-4fa3bbd32568
	github.com/juju/rpcreflect v0.0.0-20200416001309-bb46e9ba1476
	github.com/juju/schema v1.0.1-0.20190814234152-1f8aaeef0989
	github.com/juju/terms-client/v2 v2.0.0-20210422053140-27f71c100676
	github.com/juju/testing v0.0.0-20210324180055-18c50b0c2098
	github.com/juju/txn/v2 v2.0.0-20210407000251-11166e89894c
	github.com/juju/utils/v3 v3.0.0-20220130232349-cd7ecef0e94a
	github.com/juju/version/v2 v2.0.0-20211007103408-2e8da085dc23
	github.com/juju/webbrowser v1.0.0
	github.com/juju/worker/v3 v3.0.0-20211202172815-a08892ace206
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kr/pretty v0.2.1
	github.com/lxc/lxd v0.0.0-20210607205159-a7c206b5233d
	github.com/mattn/go-isatty v0.0.14
	github.com/mitchellh/go-linereader v0.0.0-20190213213312-1b945b3263eb
	github.com/oracle/oci-go-sdk/v47 v47.1.0
	github.com/packethost/packngo v0.19.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/client_model v0.2.0
	github.com/satori/go.uuid v1.2.0
	github.com/vishvananda/netlink v1.1.0
	github.com/vmware/govmomi v0.21.1-0.20191008161538-40aebf13ba45
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
	golang.org/x/net v0.0.0-20210917221730-978cfadd31cf
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	golang.org/x/sys v0.0.0-20210921065528-437939a70204
	golang.org/x/tools v0.0.0-20210105210202-9ed45478a130
	google.golang.org/api v0.29.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/httprequest.v1 v1.2.1
	gopkg.in/ini.v1 v1.51.0
	gopkg.in/juju/environschema.v1 v1.0.1-0.20201027142642-c89a4490670a
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
	gopkg.in/retry.v1 v1.0.3
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/api v0.19.6
	k8s.io/apiextensions-apiserver v0.19.6
	k8s.io/apimachinery v0.19.6
	k8s.io/client-go v0.19.6
	k8s.io/klog/v2 v2.3.0
	k8s.io/utils v0.0.0-20200729134348-d5654de09c73
)

require (
	cloud.google.com/go v0.56.0 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/Azure/go-autorest/logger v0.2.0 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/Azure/go-ntlmssp v0.0.0-20180810175552-4a21cbd618b4 // indirect
	github.com/ChrisTrenkamp/goxpath v0.0.0-20170922090931-c385f95c6022 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.1.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.0.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.1.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.2.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.4.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgrijalva/jwt-go v3.2.0+incompatible // indirect
	github.com/docker/spdystream v0.0.0-20160310174837-449fdfce4d96 // indirect
	github.com/evanphx/json-patch v4.9.0+incompatible // indirect
	github.com/flosch/pongo2 v0.0.0-20200913210552-0d938eb266f3 // indirect
	github.com/go-macaroon-bakery/macaroonpb v1.0.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/godbus/dbus/v5 v5.0.3 // indirect
	github.com/gofrs/uuid v3.2.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/golang/protobuf v1.5.0 // indirect
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/google/uuid v1.1.2 // indirect
	github.com/googleapis/gax-go/v2 v2.0.5 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.0.0 // indirect
	github.com/hashicorp/golang-lru v0.5.1 // indirect
	github.com/imdario/mergo v0.3.5 // indirect
	github.com/jessevdk/go-flags v1.4.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/json-iterator/go v1.1.10 // indirect
	github.com/juju/cmd v0.0.0-20200108104440-8e43f3faa5c9 // indirect
	github.com/juju/go4 v0.0.0-20160222163258-40d72ab9641a // indirect
	github.com/juju/gojsonpointer v0.0.0-20150204194629-afe8b77aa08f // indirect
	github.com/juju/gojsonreference v0.0.0-20150204194633-f0d24ac5ee33 // indirect
	github.com/juju/lru v0.0.0-20190314140547-92a0afabdc41 // indirect
	github.com/juju/mutex v0.0.0-20180619145857-d21b13acf4bf // indirect
	github.com/juju/txn v0.0.0-20210302043154-251cea9e140a // indirect
	github.com/juju/usso v1.0.1 // indirect
	github.com/juju/utils v0.0.0-20200423035217-b0a7da72a5fa // indirect
	github.com/juju/utils/v2 v2.0.0-20210305225158-eedbe7b6b3e2 // indirect
	github.com/juju/version v0.0.0-20210303051006-2015802527a8 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lestrrat/go-jspointer v0.0.0-20160229021354-f4881e611bdb // indirect
	github.com/lestrrat/go-jsref v0.0.0-20160601013240-e452c7b5801d // indirect
	github.com/lestrrat/go-jsschema v0.0.0-20160903131957-b09d7650b822 // indirect
	github.com/lestrrat/go-jsval v0.0.0-20161012045717-b1258a10419f // indirect
	github.com/lestrrat/go-pdebug v0.0.0-20160817063333-2e6eaaa5717f // indirect
	github.com/lestrrat/go-structinfo v0.0.0-20160308131105-f74c056fe41f // indirect
	github.com/lunixbochs/vtclean v1.0.0 // indirect
	github.com/masterzen/simplexml v0.0.0-20160608183007-4572e39b1ab9 // indirect
	github.com/masterzen/winrm v0.0.0-20200615185753-c42b5136ff88 // indirect
	github.com/mattn/go-colorable v0.1.10 // indirect
	github.com/mattn/go-runewidth v0.0.2 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pborman/uuid v1.2.0 // indirect
	github.com/pkg/term v1.1.0 // indirect
	github.com/prometheus/common v0.10.0 // indirect
	github.com/prometheus/procfs v0.1.3 // indirect
	github.com/rogpeppe/fastuuid v0.0.0-20150106093220-6724a57986af // indirect
	github.com/smartystreets/goconvey v1.7.2 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/vishvananda/netns v0.0.0-20191106174202-0a2b9b5464df // indirect
	github.com/xdg-go/stringprep v1.0.2 // indirect
	go.etcd.io/bbolt v1.3.5 // indirect
	go.opencensus.io v0.22.3 // indirect
	golang.org/x/mod v0.3.0 // indirect
	golang.org/x/term v0.0.0-20201126162022-7de9c90e9dd1 // indirect
	golang.org/x/text v0.3.6 // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/appengine v1.6.5 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/grpc v1.33.2 // indirect
	google.golang.org/protobuf v1.26.0 // indirect
	gopkg.in/errgo.v1 v1.0.1 // indirect
	gopkg.in/gobwas/glob.v0 v0.2.3 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/juju/names.v2 v2.0.0-20190813004204-e057c73bd1be // indirect
	gopkg.in/macaroon-bakery.v2 v2.3.0 // indirect
	gopkg.in/robfig/cron.v2 v2.0.0-20150107220207-be2e0b0deed5 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	k8s.io/kube-openapi v0.0.0-20200805222855-6aeccd4b50c6 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.0.1 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
)

replace github.com/altoros/gosigma => github.com/juju/gosigma v0.0.0-20200420012028-063911838a9e

replace gopkg.in/yaml.v2 => github.com/juju/yaml v0.0.0-20200420012109-12a32b78de07

replace github.com/dustin/go-humanize v1.0.0 => github.com/dustin/go-humanize v0.0.0-20141228071148-145fabdb1ab7

replace github.com/hashicorp/raft-boltdb => github.com/juju/raft-boltdb v0.0.0-20200518034108-40b112c917c5

replace (
	k8s.io/api v0.0.0 => k8s.io/api v0.19.6
	k8s.io/apiextensions-apiserver v0.0.0 => k8s.io/apiextensions-apiserver v0.19.6
	k8s.io/apimachinery v0.0.0 => k8s.io/apimachinery v0.19.6
	k8s.io/client-go v0.0.0 => k8s.io/client-go v0.19.6
)
