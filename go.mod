module github.com/juju/juju

go 1.24.4

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.14.0
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.7.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v3 v3.0.0-beta.2
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2 v2.0.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault v1.4.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi v1.2.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork v1.1.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.2.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions v1.3.0
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys v1.1.0
	github.com/EvilSuperstars/go-cidrman v0.0.0-20190607145828-28e79e32899a
	github.com/aws/aws-sdk-go-v2 v1.36.3
	github.com/aws/aws-sdk-go-v2/config v1.29.9
	github.com/aws/aws-sdk-go-v2/credentials v1.17.62
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.210.0
	github.com/aws/aws-sdk-go-v2/service/ecr v1.43.0
	github.com/aws/aws-sdk-go-v2/service/iam v1.40.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.78.2
	github.com/aws/smithy-go v1.22.3
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f
	github.com/canonical/go-dqlite/v2 v2.0.0
	github.com/canonical/lxd v0.0.0-20241209155119-76da976c6ee7
	github.com/canonical/pebble v1.19.2
	github.com/canonical/sqlair v0.0.0-20250120155751-a83645b9a121
	github.com/chzyer/readline v1.5.1
	github.com/coreos/go-systemd/v22 v22.5.0
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/docker/distribution v2.8.3+incompatible
	github.com/dustin/go-humanize v1.0.1
	github.com/fatih/color v1.16.0
	github.com/fsnotify/fsnotify v1.8.0
	github.com/gliderlabs/ssh v0.3.8
	github.com/go-delve/delve v1.24.0
	github.com/go-goose/goose/v5 v5.0.0-20230421180421-abaee9096e3a
	github.com/go-logr/logr v1.4.3
	github.com/go-macaroon-bakery/macaroon-bakery/v3 v3.0.2
	github.com/google/gnostic-models v0.6.8
	github.com/google/go-querystring v1.1.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/websocket v1.5.1
	github.com/gosuri/uitable v0.0.4
	github.com/hashicorp/vault/api v1.10.0
	github.com/hpcloud/tail v1.0.0
	github.com/icza/backscanner v0.0.0-20241124160932-dff01ac50250
	github.com/im7mortal/kmutex v1.0.1
	github.com/juju/ansiterm v1.0.0
	github.com/juju/clock v1.1.1
	github.com/juju/collections v1.0.4
	github.com/juju/description/v9 v9.0.0-20250618150523-6075a172088f
	github.com/juju/errors v1.0.0
	github.com/juju/gnuflag v1.0.0
	github.com/juju/gojsonschema v1.0.0
	github.com/juju/gomaasapi/v2 v2.2.0
	github.com/juju/idmclient/v2 v2.0.1
	github.com/juju/jsonschema v1.0.0
	github.com/juju/loggo/v2 v2.2.0
	github.com/juju/lumberjack/v2 v2.0.2
	github.com/juju/mgo/v3 v3.0.4
	github.com/juju/mutex/v2 v2.0.0
	github.com/juju/names/v6 v6.0.0-20250512075813-b50ca77a4137
	github.com/juju/naturalsort v1.0.0
	github.com/juju/os/v2 v2.2.5
	github.com/juju/persistent-cookiejar v1.0.0
	github.com/juju/proxy v1.0.0
	github.com/juju/pubsub/v2 v2.0.0
	github.com/juju/ratelimit v1.0.2
	github.com/juju/replicaset/v3 v3.0.1
	github.com/juju/retry v1.0.1
	github.com/juju/schema v1.2.0
	github.com/juju/tc v0.0.0-20250528095810-3bfebf94677d
	github.com/juju/testing v1.2.0
	github.com/juju/txn/v3 v3.0.2
	github.com/juju/utils/v4 v4.0.3
	github.com/juju/webbrowser v1.0.0
	github.com/juju/worker/v4 v4.2.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kr/pretty v0.3.1
	github.com/lestrrat-go/jwx/v2 v2.0.21
	github.com/mattn/go-isatty v0.0.20
	github.com/mattn/go-sqlite3 v1.14.24
	github.com/microsoft/kiota-abstractions-go v1.5.3
	github.com/microsoft/kiota-http-go v1.1.1
	github.com/microsoftgraph/msgraph-sdk-go v1.28.0
	github.com/mitchellh/go-linereader v0.0.0-20190213213312-1b945b3263eb
	github.com/mitchellh/mapstructure v1.5.0
	github.com/mittwald/vaultgo v0.1.4
	github.com/moby/sys/mountinfo v0.7.1
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/oracle/oci-go-sdk/v65 v65.55.0
	github.com/peterh/liner v1.2.2
	github.com/pkg/errors v0.9.1
	github.com/pkg/sftp v1.13.7
	github.com/prometheus/client_golang v1.21.0
	github.com/prometheus/client_model v0.6.1
	github.com/rs/xid v1.6.0
	github.com/vallerion/rscanner v0.0.0-20230822073625-4f90454447a3
	github.com/vishvananda/netlink v1.3.0
	github.com/vmware/govmomi v0.34.1
	go.opentelemetry.io/otel v1.34.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.32.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.32.0
	go.opentelemetry.io/otel/sdk v1.32.0
	go.opentelemetry.io/otel/trace v1.34.0
	go.uber.org/goleak v1.3.0
	go.uber.org/mock v0.5.0
	golang.org/x/crypto v0.39.0
	golang.org/x/net v0.41.0
	golang.org/x/oauth2 v0.28.0
	golang.org/x/sync v0.15.0
	golang.org/x/sys v0.33.0
	golang.org/x/time v0.11.0
	golang.org/x/tools v0.33.0
	google.golang.org/api v0.215.0
	google.golang.org/grpc v1.70.0
	gopkg.in/errgo.v1 v1.0.1
	gopkg.in/httprequest.v1 v1.2.1
	gopkg.in/ini.v1 v1.67.0
	gopkg.in/macaroon-bakery.v2 v2.3.0
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/retry.v1 v1.0.3
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.29.0
	k8s.io/apiextensions-apiserver v0.29.0
	k8s.io/apimachinery v0.29.0
	k8s.io/client-go v0.29.0
	k8s.io/klog/v2 v2.110.1
	k8s.io/utils v0.0.0-20241104163129-6fe5fd82f078
)

require (
	cloud.google.com/go/auth v0.13.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.6 // indirect
	cloud.google.com/go/compute/metadata v0.6.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.10.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/internal v1.1.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/managementgroups/armmanagementgroups v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/internal v1.0.1 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.2.2 // indirect
	github.com/GehirnInc/crypt v0.0.0-20230320061759-8cc1b52080c5 // indirect
	github.com/Rican7/retry v0.3.1 // indirect
	github.com/anmitsu/go-shlex v0.0.0-20200514113438-38f4b401e2be // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.10 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.34 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.7.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.18.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.25.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.29.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.33.17 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/canonical/go-flags v0.0.0-20230403090104-105d09a091b8 // indirect
	github.com/canonical/x-go v0.0.0-20230522092633-7947a7587f5b // indirect
	github.com/cenkalti/backoff/v3 v3.0.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cilium/ebpf v0.11.0 // indirect
	github.com/cjlapao/common-go v0.0.39 // indirect
	github.com/cosiner/argv v0.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/derekparker/trie v0.0.0-20230829180723-39f4de51ef7d // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/flosch/pongo2 v0.0.0-20200913210552-0d938eb266f3 // indirect
	github.com/frankban/quicktest v1.14.6 // indirect
	github.com/go-delve/liner v1.2.3-0.20231231155935-4726ab1d7f62 // indirect
	github.com/go-jose/go-jose/v3 v3.0.3 // indirect
	github.com/go-jose/go-jose/v4 v4.0.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-macaroon-bakery/macaroonpb v1.0.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/godbus/dbus/v5 v5.0.4 // indirect
	github.com/gofrs/flock v0.8.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/go-dap v0.12.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/renameio v1.0.1 // indirect
	github.com/google/s2a-go v0.1.8 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.4 // indirect
	github.com/googleapis/gax-go/v2 v2.14.1 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.24.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.7 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.6 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.2 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/juju/go4 v0.0.0-20160222163258-40d72ab9641a // indirect
	github.com/juju/gojsonpointer v0.0.0-20150204194629-afe8b77aa08f // indirect
	github.com/juju/gojsonreference v0.0.0-20150204194633-f0d24ac5ee33 // indirect
	github.com/juju/loggo v1.0.0 // indirect
	github.com/juju/lru v1.0.0 // indirect
	github.com/juju/mgo/v2 v2.0.2 // indirect
	github.com/juju/usso v1.0.1 // indirect
	github.com/juju/utils/v3 v3.2.2 // indirect
	github.com/juju/version v0.0.0-20210303051006-2015802527a8 // indirect
	github.com/juju/version/v2 v2.0.0 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lestrrat-go/blackmagic v1.0.2 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/httprc v1.0.5 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/lestrrat/go-jspointer v0.0.0-20160229021354-f4881e611bdb // indirect
	github.com/lestrrat/go-jsref v0.0.0-20160601013240-e452c7b5801d // indirect
	github.com/lestrrat/go-jsschema v0.0.0-20160903131957-b09d7650b822 // indirect
	github.com/lestrrat/go-jsval v0.0.0-20161012045717-b1258a10419f // indirect
	github.com/lestrrat/go-pdebug v0.0.0-20160817063333-2e6eaaa5717f // indirect
	github.com/lestrrat/go-structinfo v0.0.0-20160308131105-f74c056fe41f // indirect
	github.com/lunixbochs/vtclean v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/microsoft/kiota-authentication-azure-go v1.0.1 // indirect
	github.com/microsoft/kiota-serialization-form-go v1.0.0 // indirect
	github.com/microsoft/kiota-serialization-json-go v1.0.4 // indirect
	github.com/microsoft/kiota-serialization-multipart-go v1.0.0 // indirect
	github.com/microsoft/kiota-serialization-text-go v1.0.0 // indirect
	github.com/microsoftgraph/msgraph-sdk-go-core v1.0.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/muhlemmer/gu v0.3.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/term v1.1.0 // indirect
	github.com/pkg/xattr v0.4.10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/fastuuid v1.2.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/segmentio/asm v1.2.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/sony/gobreaker v0.5.0 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/std-uritemplate/std-uritemplate/go v0.0.47 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/zitadel/logging v0.6.1 // indirect
	github.com/zitadel/oidc/v3 v3.33.1 // indirect
	github.com/zitadel/schema v1.3.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.54.0 // indirect
	go.opentelemetry.io/otel/metric v1.34.0 // indirect
	go.opentelemetry.io/proto/otlp v1.4.0 // indirect
	go.starlark.net v0.0.0-20241125201518-c05ff208a98f // indirect
	golang.org/x/arch v0.11.0 // indirect
	golang.org/x/exp v0.0.0-20250305212735-054e65f0b394 // indirect
	golang.org/x/mod v0.25.0 // indirect
	golang.org/x/term v0.32.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20241209162323-e6fa225c2576 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250224174004-546df14abb99 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/fsnotify.v1 v1.4.7 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/juju/environschema.v1 v1.0.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	k8s.io/kube-openapi v0.0.0-20231010175941-2dd684a91f00 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)

replace gopkg.in/yaml.v2 => github.com/juju/yaml/v2 v2.0.0

// go.uber.org/mock v0.5.0 is a broken release. It adds named arguments to mocked
// methods, which sometimes clash with package names. Fix the package to v0.4.0
// until the issue is resolved.
replace go.uber.org/mock => go.uber.org/mock v0.4.0

replace gopkg.in/check.v1 => github.com/hpidcock/gc-compat-tc v0.0.0-20250523041742-c3a83c867edf
