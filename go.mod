module go.lumeweb.com/portal

go 1.23.1

toolchain go1.23.2

require (
	github.com/AfterShip/email-verifier v1.4.1
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/adjust/rmq/v5 v5.2.0
	github.com/amacneil/dbmate/v2 v2.26.0
	github.com/aws/aws-sdk-go-v2 v1.36.3
	github.com/aws/aws-sdk-go-v2/config v1.29.9
	github.com/aws/aws-sdk-go-v2/credentials v1.17.62
	github.com/aws/aws-sdk-go-v2/service/s3 v1.78.2
	github.com/casbin/casbin/v2 v2.104.0
	github.com/casbin/gorm-adapter/v3 v3.32.0
	github.com/docker/go-units v0.5.0
	github.com/gabriel-vasile/mimetype v1.4.8
	github.com/gammazero/workerpool v1.1.3
	github.com/go-co-op/gocron-redis-lock/v2 v2.0.1
	github.com/go-co-op/gocron/v2 v2.16.1
	github.com/go-gorm/caches/v4 v4.0.5
	github.com/go-sql-driver/mysql v1.9.0
	github.com/go-viper/mapstructure/v2 v2.2.1
	github.com/google/uuid v1.6.0
	github.com/gookit/event v1.1.2
	github.com/gorilla/mux v1.8.2-0.20240619235004-db9d1d0073d2
	github.com/knadh/koanf v1.5.0
	github.com/knadh/koanf/v2 v2.1.2
	github.com/labstack/echo/v4 v4.13.4
	github.com/multiformats/go-multihash v0.2.3
	github.com/naucon/casbin-fs-adapter v0.2.0
	github.com/pquerna/otp v1.4.0
	github.com/redis/go-redis/v9 v9.7.1
	github.com/samber/lo v1.49.1
	github.com/stretchr/testify v1.10.0
	github.com/tus/tusd/v2 v2.7.1
	github.com/urfave/cli/v3 v3.0.0-beta1
	github.com/wneessen/go-mail v0.6.2
	go.etcd.io/etcd/client/v3 v3.5.19
	go.lumeweb.com/httputil v0.4.2
	go.lumeweb.com/portal-middleware v0.2.7
	go.lumeweb.com/portal-router v0.3.7
	go.sia.tech/core v0.10.4
	go.sia.tech/coreutils v0.12.1
	go.sia.tech/renterd v1.1.1
	go.uber.org/zap v1.27.0
	go.uber.org/zap/exp v0.3.0
	golang.org/x/crypto v0.38.0
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/datatypes v1.2.5
	gorm.io/driver/mysql v1.5.7
	gorm.io/driver/sqlite v1.5.7
	gorm.io/gorm v1.25.12
)

require (
	dario.cat/mergo v1.0.1 // indirect
	github.com/Oudwins/zog v0.18.4 // indirect
	github.com/alicebob/gopher-json v0.0.0-20230218143504-906a9b012302 // indirect
	github.com/alicebob/miniredis/v2 v2.34.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmatcuk/doublestar/v4 v4.8.1 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/gammazero/deque v1.0.0 // indirect
	github.com/getkin/kin-openapi v0.132.0 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/glebarez/go-sqlite v1.22.0 // indirect
	github.com/glebarez/sqlite v1.11.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/golang-sql/civil v0.0.0-20220223132316-b832511892a9 // indirect
	github.com/golang-sql/sqlexp v0.1.0 // indirect
	github.com/google/pprof v0.0.0-20241017200806-017d972448fc // indirect
	github.com/invopop/jsonschema v0.13.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.7.2 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/microsoft/go-mssqldb v1.8.0 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/oasdiff/yaml v0.0.0-20250309154309-f31be36b4037 // indirect
	github.com/oasdiff/yaml3 v0.0.0-20250309153720-d2182401db90 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.21.1 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.63.0 // indirect
	github.com/prometheus/procfs v0.16.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.lumeweb.com/gswagger v0.17.3 // indirect
	go.lumeweb.com/queryutil v0.3.2 // indirect
	go.sia.tech/jape v0.12.1 // indirect
	golang.org/x/sync v0.14.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	google.golang.org/grpc v1.71.0 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gorm.io/driver/postgres v1.5.11 // indirect
	gorm.io/driver/sqlserver v1.5.4 // indirect
	gorm.io/plugin/dbresolver v1.5.3 // indirect
	lukechampine.com/blake3 v1.4.0 // indirect
	modernc.org/libc v1.61.13 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.9.0 // indirect
	modernc.org/sqlite v1.36.1 // indirect
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
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
	github.com/aws/smithy-go v1.22.3 // indirect
	github.com/boombuler/barcode v1.0.2 // indirect
	github.com/casbin/govaluate v1.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-redsync/redsync/v4 v4.13.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/gotd/contrib v0.21.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hbollon/go-edlib v1.6.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/klauspost/reedsolomon v1.12.4 // indirect
	github.com/mattn/go-sqlite3 v1.14.24 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	go.etcd.io/etcd/api/v3 v3.5.19
	go.etcd.io/etcd/client/pkg/v3 v3.5.19 // indirect
	go.sia.tech/mux v1.4.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20250506013437-ce4c2cf36ca6 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.25.0 // indirect
	golang.org/x/tools v0.33.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250313205543-e70fdf4c4cb4 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250313205543-e70fdf4c4cb4 // indirect
	lukechampine.com/frand v1.5.1 // indirect
)

replace (
	github.com/go-co-op/gocron-redis-lock/v2 => github.com/LumeWeb/gocron-redis-lock/v2 v2.0.0-20240722104549-387206078839
	github.com/go-co-op/gocron/v2 => github.com/LumeWeb/gocron/v2 v2.0.0-20240814201336-2d361739e9be
	github.com/go-viper/mapstructure/v2 => github.com/LumeWeb/mapstructure/v2 v2.0.0-20241213212524-92525f5828be
	github.com/tus/tusd/v2 => github.com/LumeWeb/tusd/v2 v2.2.3-0.20241020013555-e29b4c6c01b7
	gorm.io/plugin/dbresolver => gorm.io/plugin/dbresolver v1.5.3
)
