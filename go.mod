module github.com/LumeWeb/portal

go 1.22.1

require (
	github.com/AfterShip/email-verifier v1.4.0
	github.com/LumeWeb/siacentral-api v0.0.0-20240311114304-4ff40c07bce5
	github.com/aws/aws-sdk-go-v2 v1.26.1
	github.com/aws/aws-sdk-go-v2/config v1.27.13
	github.com/aws/aws-sdk-go-v2/credentials v1.17.13
	github.com/aws/aws-sdk-go-v2/service/s3 v1.53.1
	github.com/casbin/casbin/v2 v2.87.1
	github.com/docker/go-units v0.5.0
	github.com/gabriel-vasile/mimetype v1.4.3
	github.com/getkin/kin-openapi v0.118.0
	github.com/go-co-op/gocron-redis-lock/v2 v2.0.1
	github.com/go-co-op/gocron/v2 v2.2.9
	github.com/go-gorm/caches/v4 v4.0.4
	github.com/go-sql-driver/mysql v1.8.1
	github.com/go-viper/mapstructure/v2 v2.0.0
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/handlers v1.5.2
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/go-plugin v1.6.0
	github.com/knadh/koanf v1.5.0
	github.com/knadh/koanf/v2 v2.1.1
	github.com/naucon/casbin-fs-adapter v0.2.0
	github.com/pquerna/otp v1.4.0
	github.com/redis/go-redis/v9 v9.5.1
	github.com/samber/lo v1.39.0
	github.com/shopspring/decimal v1.4.0
	github.com/wneessen/go-mail v0.4.1
	go.etcd.io/etcd/client/v3 v3.5.13
	go.sia.tech/core v0.2.5
	go.sia.tech/coreutils v0.0.4
	go.sia.tech/jape v0.11.2-0.20240228204811-29a0f056d231
	go.sia.tech/renterd v1.0.7
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.23.0
	google.golang.org/grpc v1.63.2
	google.golang.org/protobuf v1.34.1
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/driver/mysql v1.5.6
	gorm.io/driver/sqlite v1.5.5
	gorm.io/gorm v1.25.9
	lukechampine.com/blake3 v1.3.0
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.2 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.5 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.5 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.11.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.3.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.11.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.17.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.20.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.24.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.28.7 // indirect
	github.com/aws/smithy-go v1.20.2 // indirect
	github.com/boombuler/barcode v1.0.1-0.20190219062509-6c824513bacc // indirect
	github.com/casbin/govaluate v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dchest/threefish v0.0.0-20120919164726-3ecf4c494abf // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fatih/color v1.14.1 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-openapi/jsonpointer v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.8 // indirect
	github.com/go-redsync/redsync/v4 v4.10.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/gotd/contrib v0.19.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-hclog v1.6.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/yamux v0.1.1 // indirect
	github.com/hbollon/go-edlib v1.6.0 // indirect
	github.com/invopop/yaml v0.2.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/jonboulle/clockwork v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/klauspost/reedsolomon v1.12.1 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-testing-interface v1.0.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/oklog/run v1.0.0 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rogpeppe/go-internal v1.12.0 // indirect
	gitlab.com/NebulousLabs/bolt v1.4.4 // indirect
	gitlab.com/NebulousLabs/encoding v0.0.0-20200604091946-456c3dc907fe // indirect
	gitlab.com/NebulousLabs/entropy-mnemonics v0.0.0-20181018051301-7532f67e3500 // indirect
	gitlab.com/NebulousLabs/errors v0.0.0-20200929122200-06c536cf6975 // indirect
	gitlab.com/NebulousLabs/fastrand v0.0.0-20181126182046-603482d69e40 // indirect
	gitlab.com/NebulousLabs/go-upnp v0.0.0-20211002182029-11da932010b6 // indirect
	gitlab.com/NebulousLabs/log v0.0.0-20210609172545-77f6775350e2 // indirect
	gitlab.com/NebulousLabs/merkletree v0.0.0-20200118113624-07fbf710afc4 // indirect
	gitlab.com/NebulousLabs/persist v0.0.0-20200605115618-007e5e23d877 // indirect
	gitlab.com/NebulousLabs/ratelimit v0.0.0-20200811080431-99b8f0768b2e // indirect
	gitlab.com/NebulousLabs/siamux v0.0.2-0.20220630142132-142a1443a259 // indirect
	gitlab.com/NebulousLabs/threadgroup v0.0.0-20200608151952-38921fbef213 // indirect
	go.etcd.io/etcd/api/v3 v3.5.13 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.13 // indirect
	go.sia.tech/mux v1.2.0 // indirect
	go.sia.tech/siad v1.5.10-0.20230228235644-3059c0b930ca // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20231219180239-dc181d75b848 // indirect
	golang.org/x/net v0.25.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	golang.org/x/text v0.15.0 // indirect
	golang.org/x/tools v0.20.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240509183442-62759503f434 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240509183442-62759503f434 // indirect
	lukechampine.com/frand v1.4.2 // indirect
)

replace (
	github.com/go-co-op/gocron/v2 => github.com/LumeWeb/gocron/v2 v2.0.0-20240414080020-1f776248b980
	github.com/go-viper/mapstructure/v2 v2.0.0 => github.com/LumeWeb/mapstructure/v2 v2.0.0-20240603224933-c63fee0297e6
	github.com/gorilla/mux v1.8.1 => github.com/cornejong/gormux v0.0.0-20240526072501-ce1c97b033ec
	github.com/tus/tusd-etcd3-locker v0.0.0-20200405122323-74aeca810256 => github.com/LumeWeb/tusd-etcd3-locker v0.0.0-20240510103936-0d66760cf053
	github.com/tus/tusd/v2 => github.com/LumeWeb/tusd/v2 v2.2.3-0.20240224143554-96925dd43120
)
