module github.com/juju/juju/v2

go 1.18

require (
	github.com/Azure/azure-sdk-for-go v65.0.0+incompatible
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.1.0
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.1.0
	github.com/Azure/azure-sdk-for-go/sdk/keyvault/azkeys v0.5.1
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2 v2.0.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault v1.0.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork v1.0.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.0.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions v1.0.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage v1.0.0
	github.com/Azure/go-autorest/autorest v0.11.18
	github.com/Azure/go-autorest/autorest/adal v0.9.13
	github.com/Azure/go-autorest/autorest/date v0.3.0
	github.com/Azure/go-autorest/autorest/mocks v0.4.1
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
	github.com/canonical/pebble v0.0.0-20220620222205-a30d4c54906d
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e
	github.com/coreos/go-systemd/v22 v22.3.2
	github.com/docker/distribution v2.7.1+incompatible
	github.com/dustin/go-humanize v1.0.0
	github.com/go-goose/goose/v5 v5.0.0-20220419055500-7227d44183a5
	github.com/go-logr/logr v1.2.2
	github.com/go-macaroon-bakery/macaroon-bakery/v3 v3.0.0-20220204130128-afeebcc9521d
	github.com/gofrs/uuid v4.2.0+incompatible
	github.com/golang/mock v1.6.0
	github.com/google/go-querystring v1.0.0
	github.com/googleapis/gnostic v0.5.5
	github.com/gorilla/handlers v0.0.0-20170224193955-13d73096a474
	github.com/gorilla/schema v0.0.0-20160426231512-08023a0215e7
	github.com/gorilla/websocket v1.4.2
	github.com/gosuri/uitable v0.0.1
	github.com/hashicorp/go-hclog v0.9.1
	github.com/hashicorp/go-msgpack v0.5.5
	github.com/hashicorp/raft v1.3.2-0.20210825230038-1a621031eb2b
	github.com/hashicorp/raft-boltdb v0.0.0-20171010151810-6e5ba93211ea
	github.com/im7mortal/kmutex v1.0.1
	github.com/juju/ansiterm v0.0.0-20210929141451-8b71cc96ebdc
	github.com/juju/blobstore/v2 v2.0.0-20220204135513-4876189e56a4
	github.com/juju/charm/v8 v8.0.0-20220509231111-ed6d505a46f4
	github.com/juju/charmrepo/v6 v6.0.0-20220207014006-e6af52d614e4
	github.com/juju/clock v0.0.0-20220203021603-d9deb868a28a
	github.com/juju/cmd/v3 v3.0.0-20220203030511-039f3566372a
	github.com/juju/collections v0.0.0-20220203020748-febd7cad8a7a
	github.com/juju/description/v3 v3.0.0-20220207013250-e60bc7b1c242
	github.com/juju/errors v0.0.0-20220316043928-e10eb17a9eeb
	github.com/juju/featureflag v0.0.0-20220207005600-a9676d92ad24
	github.com/juju/gnuflag v0.0.0-20171113085948-2ce1bb71843d
	github.com/juju/gojsonschema v0.0.0-20150312170016-e1ad140384f2
	github.com/juju/gomaasapi/v2 v2.0.0-20220215192228-f1f389204471
	github.com/juju/http/v2 v2.0.0-20220615041531-27607e3f581a
	github.com/juju/idmclient/v2 v2.0.0-20220207024613-525e1ac3a890
	github.com/juju/jsonschema v0.0.0-20220207021905-6f321f9146b3
	github.com/juju/jsonschema-gen v0.0.0-20220207021829-b1697c753155
	github.com/juju/loggo v0.0.0-20210728185423-eebad3a902c4
	github.com/juju/lumberjack v2.0.0-20200420012306-ddfd864a6ade+incompatible
	github.com/juju/mgo/v2 v2.0.0-20220111072304-f200228f1090
	github.com/juju/mutex/v2 v2.0.0-20220203023141-11eeddb42c6c
	github.com/juju/names/v4 v4.0.0-20220518060443-d77cb46f6093
	github.com/juju/naturalsort v0.0.0-20180423034842-5b81707e882b
	github.com/juju/os/v2 v2.2.3
	github.com/juju/packaging/v2 v2.0.0-20220207023655-2ed4de2dc5b4
	github.com/juju/persistent-cookiejar v0.0.0-20170428161559-d67418f14c93
	github.com/juju/proxy v0.0.0-20220207021845-4d37a2e6a78f
	github.com/juju/pubsub/v2 v2.0.0-20220207005728-39d68caef4a7
	github.com/juju/ratelimit v1.0.2-0.20191002062651-f60b32039441
	github.com/juju/replicaset/v2 v2.0.1-0.20220330042111-ec94259ca14f
	github.com/juju/retry v0.0.0-20220204093819-62423bf33287
	github.com/juju/rfc/v2 v2.0.0-20220207021814-ffb92bc8e9eb
	github.com/juju/romulus v0.0.0-20220207004956-1a3bcf86b836
	github.com/juju/rpcreflect v0.0.0-20200416001309-bb46e9ba1476
	github.com/juju/schema v1.0.1-0.20190814234152-1f8aaeef0989
	github.com/juju/terms-client/v2 v2.0.0-20220207005045-25aa57bf93cd
	github.com/juju/testing v0.0.0-20220203020004-a0ff61f03494
	github.com/juju/txn/v2 v2.0.0-20220204100656-93c9d8a9b89f
	github.com/juju/utils/v3 v3.0.0-20220203023959-c3fbc78a33b0
	github.com/juju/version/v2 v2.0.0-20220204124744-fc9915e3d935
	github.com/juju/webbrowser v1.0.0
	github.com/juju/worker/v3 v3.0.0-20220204100750-e23db69a42d2
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kr/pretty v0.2.1
	github.com/lxc/lxd v0.0.0-20210607205159-a7c206b5233d
	github.com/mattn/go-isatty v0.0.14
	github.com/mitchellh/go-linereader v0.0.0-20190213213312-1b945b3263eb
	github.com/oracle/oci-go-sdk/v47 v47.1.0
	github.com/packethost/packngo v0.19.0
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/client_model v0.2.0
	github.com/vishvananda/netlink v1.1.1-0.20201029203352-d40f9887b852
	github.com/vmware/govmomi v0.21.1-0.20191008161538-40aebf13ba45
	golang.org/x/crypto v0.0.0-20220525230936-793ad666bf5e
	golang.org/x/net v0.0.0-20220607020251-c690dde0001d
	golang.org/x/oauth2 v0.0.0-20220608161450-d0670ef3b1eb
	golang.org/x/sync v0.0.0-20220601150217-0de741cfad7f
	golang.org/x/sys v0.0.0-20220608164250-635b8c9b7f68
	golang.org/x/tools v0.1.11
	google.golang.org/api v0.78.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/httprequest.v1 v1.2.1
	gopkg.in/ini.v1 v1.51.0
	gopkg.in/juju/environschema.v1 v1.0.1-0.20201027142642-c89a4490670a
	gopkg.in/macaroon-bakery.v2 v2.3.0
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
	gopkg.in/retry.v1 v1.0.3
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.23.4
	k8s.io/apiextensions-apiserver v0.21.10
	k8s.io/apimachinery v0.23.4
	k8s.io/client-go v0.23.4
	k8s.io/klog/v2 v2.40.1
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
)

require (
	cloud.google.com/go/compute v1.6.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.0.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/keyvault/internal v0.5.0 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/Azure/go-ntlmssp v0.0.0-20211209120228-48547f28849e // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v0.5.1 // indirect
	github.com/ChrisTrenkamp/goxpath v0.0.0-20210404020558-97928f7e12b6 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.1.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.0.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.1.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.2.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.4.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/flosch/pongo2 v0.0.0-20200913210552-0d938eb266f3 // indirect
	github.com/form3tech-oss/jwt-go v3.2.3+incompatible // indirect
	github.com/go-macaroon-bakery/macaroonpb v1.0.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/godbus/dbus/v5 v5.0.4 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt v3.2.1+incompatible // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.8 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/google/uuid v1.2.0 // indirect
	github.com/googleapis/gax-go/v2 v2.3.0 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.0 // indirect
	github.com/hashicorp/go-uuid v1.0.2 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.0.0 // indirect
	github.com/jcmturner/goidentity/v6 v6.0.1 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.2 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/jessevdk/go-flags v1.4.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/juju/go4 v0.0.0-20160222163258-40d72ab9641a // indirect
	github.com/juju/gojsonpointer v0.0.0-20150204194629-afe8b77aa08f // indirect
	github.com/juju/gojsonreference v0.0.0-20150204194633-f0d24ac5ee33 // indirect
	github.com/juju/juju v0.0.0-20220701051721-8ae5b8147274 // indirect
	github.com/juju/lru v0.0.0-20190314140547-92a0afabdc41 // indirect
	github.com/juju/usso v1.0.1 // indirect
	github.com/juju/version v0.0.0-20210303051006-2015802527a8 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lestrrat/go-jspointer v0.0.0-20160229021354-f4881e611bdb // indirect
	github.com/lestrrat/go-jsref v0.0.0-20160601013240-e452c7b5801d // indirect
	github.com/lestrrat/go-jsschema v0.0.0-20160903131957-b09d7650b822 // indirect
	github.com/lestrrat/go-jsval v0.0.0-20161012045717-b1258a10419f // indirect
	github.com/lestrrat/go-pdebug v0.0.0-20160817063333-2e6eaaa5717f // indirect
	github.com/lestrrat/go-structinfo v0.0.0-20160308131105-f74c056fe41f // indirect
	github.com/lunixbochs/vtclean v1.0.0 // indirect
	github.com/masterzen/simplexml v0.0.0-20190410153822-31eea3082786 // indirect
	github.com/masterzen/winrm v0.0.0-20211231115050-232efb40349e // indirect
	github.com/mattn/go-colorable v0.1.10 // indirect
	github.com/mattn/go-runewidth v0.0.2 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/onsi/ginkgo v1.14.2 // indirect
	github.com/onsi/gomega v1.10.4 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pkg/browser v0.0.0-20210115035449-ce105d075bb4 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/term v1.1.0 // indirect
	github.com/prometheus/common v0.10.0 // indirect
	github.com/prometheus/procfs v0.6.0 // indirect
	github.com/rogpeppe/fastuuid v1.2.0 // indirect
	github.com/smartystreets/goconvey v1.7.2 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/vishvananda/netns v0.0.0-20200728191858-db3c7e526aae // indirect
	github.com/xdg-go/stringprep v1.0.2 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	go.etcd.io/bbolt v1.3.5 // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4 // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220429170224-98d788798c3e // indirect
	google.golang.org/grpc v1.46.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/errgo.v1 v1.0.1 // indirect
	gopkg.in/gobwas/glob.v0 v0.2.3 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/robfig/cron.v2 v2.0.0-20150107220207-be2e0b0deed5 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65 // indirect
	sigs.k8s.io/json v0.0.0-20211020170558-c049b76a60c6 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.1 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
)

replace github.com/altoros/gosigma => github.com/juju/gosigma v0.0.0-20200420012028-063911838a9e

replace gopkg.in/yaml.v2 => github.com/juju/yaml v0.0.0-20200420012109-12a32b78de07

replace github.com/dustin/go-humanize v1.0.0 => github.com/dustin/go-humanize v0.0.0-20141228071148-145fabdb1ab7

replace github.com/hashicorp/raft-boltdb => github.com/juju/raft-boltdb v0.0.0-20200518034108-40b112c917c5
