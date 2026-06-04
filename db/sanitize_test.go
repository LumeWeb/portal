package db

import (
	"regexp"
	"testing"
	"unicode/utf8"

	"gorm.io/datatypes"
)

var (
	testGORMBinaryPattern = regexp.MustCompile(`<binary>`)
	testGORMHexPattern    = regexp.MustCompile(`X'[0-9a-fA-F]+'`)
)

func TestSanitizeTracingQuery(t *testing.T) {
	binaryUUIDStr := datatypes.NewBinUUIDv4().String()
	hexUUID := "deadbeef1234567890abcdef1234567890"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "GORM binary placeholder",
			input:    `SELECT * FROM cron_jobs WHERE uuid = "<binary>" AND deleted_at IS NULL`,
			expected: `SELECT * FROM cron_jobs WHERE uuid = "<uuid>" AND deleted_at IS NULL`,
		},
		{
			name:     "Hex-encoded binary",
			input:    `SELECT * FROM cron_jobs WHERE uuid = X'` + hexUUID + `'`,
			expected: `SELECT * FROM cron_jobs WHERE uuid = "<uuid>"`,
		},
		{
			name:     "Multiple binary values",
			input:    `SELECT * FROM cron_jobs WHERE uuid = "<binary>" AND origin = "<binary>"`,
			expected: `SELECT * FROM cron_jobs WHERE uuid = "<uuid>" AND origin = "<uuid>"`,
		},
		{
			name:     "Normal query without binary",
			input:    `SELECT * FROM access_rules ORDER BY ID`,
			expected: `SELECT * FROM access_rules ORDER BY ID`,
		},
		{
			name:     "Query with normal UUID string",
			input:    `SELECT * FROM cron_jobs WHERE uuid = '` + binaryUUIDStr + `'`,
			expected: `SELECT * FROM cron_jobs WHERE uuid = '` + binaryUUIDStr + `'`,
		},
		{
			name:     "INSERT query with binary placeholder",
			input:    `INSERT INTO cron_jobs (uuid, origin) VALUES ("<binary>", "core") RETURNING id`,
			expected: `INSERT INTO cron_jobs (uuid, origin) VALUES ("<uuid>", "core") RETURNING id`,
		},
		{
			name:     "Complex INSERT from actual log",
			input:    `INSERT INTO cron_jobs (created_at,updated_at,deleted_at,uuid,origin,source_id,job_type,args,sched_def,schedule_type,state,last_run,last_heartbeat,failures,retry_policy,version) VALUES ("2025-12-27 01:56:36.142","2025-12-27 01:56:36.142",NULL,"<binary>","core","user.process_account_deletion_requests","core.user.process_account_deletion_requests",NULL,"{\"type\":\"daily\",\"at_time\":\"0001-01-01T00:00:00Z\"}","","queued",NULL,NULL,0,"{\"max_retries\":3,\"initial_delay\":300000000000,\"backoff_factor\":1.5}",1) RETURNING id`,
			expected: `INSERT INTO cron_jobs (created_at,updated_at,deleted_at,uuid,origin,source_id,job_type,args,sched_def,schedule_type,state,last_run,last_heartbeat,failures,retry_policy,version) VALUES ("2025-12-27 01:56:36.142","2025-12-27 01:56:36.142",NULL,"<uuid>","core","user.process_account_deletion_requests","core.user.process_account_deletion_requests",NULL,"{\"type\":\"daily\",\"at_time\":\"0001-01-01T00:00:00Z\"}","","queued",NULL,NULL,0,"{\"max_retries\":3,\"initial_delay\":300000000000,\"backoff_factor\":1.5}",1) RETURNING id`,
		},
		{
			name:     "Unicode but printable characters should not be replaced",
			input:    `SELECT * FROM cron_jobs WHERE name = 'café'`,
			expected: `SELECT * FROM cron_jobs WHERE name = 'café'`,
		},
		{
			name:     "Single-quoted binary placeholder",
			input:    `SELECT * FROM cron_jobs WHERE uuid = '<binary>'`,
			expected: `SELECT * FROM cron_jobs WHERE uuid = '<uuid>'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeTracingQuery(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeTracingQuery() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSanitizeTracingQuery_UTF8Validation(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Sanitized GORM placeholder is valid UTF-8",
			input: `SELECT * FROM cron_jobs WHERE uuid = "<binary>" AND deleted_at IS NULL`,
		},
		{
			name:  "Hex-encoded binary sanitized to valid UTF-8",
			input: `SELECT * FROM cron_jobs WHERE uuid = X'deadbeef1234567890abcdef1234567890'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeTracingQuery(tt.input)
			if !isValidUTF8(result) {
				t.Errorf("sanitizeTracingQuery() produced invalid UTF-8: %q", result)
			}
		})
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == utf8.RuneError {
			return false
		}
	}
	return true
}

func TestGORMExplainBehavior(t *testing.T) {
	binaryBytes := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x80}

	tests := []struct {
		re    *regexp.Regexp
		input string
		want  bool
	}{
		{testGORMBinaryPattern, `"<binary>"`, true},
		{testGORMHexPattern, `X'deadbeef'`, true},
	}

	for _, tt := range tests {
		matched := tt.re.MatchString(tt.input)
		if matched != tt.want {
			t.Errorf("Pattern matching %q: got %v, want %v", tt.input, matched, tt.want)
		}
	}

	if isPrintable(string(binaryBytes)) {
		t.Error("Binary bytes should not be printable")
	}
}

func isPrintable(s string) bool {
	for _, r := range s {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}
