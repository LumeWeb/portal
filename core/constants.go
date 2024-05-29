package core

import "github.com/docker/go-units"

const PROOF_EXTENSION = ".obao"
const S3_MULTIPART_MAX_PARTS = 9500
const S3_MULTIPART_MIN_PART_SIZE = uint64(5 * units.MiB)
