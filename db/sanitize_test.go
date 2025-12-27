package db

import (
	"regexp"
	"testing"
	"unicode/utf8"

	"gorm.io/datatypes"
)

// Pre-compiled regex patterns for testing GORM behavior
var (
	testGORMBinaryPattern       = regexp.MustCompile(`<binary>`)
	testGORMHexPattern          = regexp.MustCompile(`X'[0-9a-fA-F]+'`)
	testGORMInvalidUTF8Pattern  = regexp.MustCompile(`"[^"]*�[^"]*"`)
)

func TestSanitizeTracingQuery(t *testing.T) {
	// Generate test data dynamically
	binaryUUID := datatypes.NewBinUUIDv4()
	binaryUUIDStr := binaryUUID.String() // Standard UUID string format

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
			name:     "Raw binary in double quotes",
			input:    `INSERT INTO cron_jobs (uuid, name) VALUES ("` + string(binaryUUID.Bytes()) + `", "test")`,
			expected: `INSERT INTO cron_jobs (uuid, name) VALUES ("<uuid>", "test")`,
		},
		{
			name:     "Raw binary in single quotes",
			input:    `SELECT * FROM cron_jobs WHERE uuid = '` + string(binaryUUID.Bytes()) + `'`,
			expected: `SELECT * FROM cron_jobs WHERE uuid = '<uuid>'`,
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
	// Generate test data dynamically
	binaryUUID := datatypes.NewBinUUIDv4()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Sanitized query is valid UTF-8",
			input:   `SELECT * FROM cron_jobs WHERE uuid = "<binary>" AND deleted_at IS NULL`,
			wantErr: false,
		},
		{
			name:    "Sanitized query with raw binary is valid UTF-8",
			input:   `SELECT * FROM cron_jobs WHERE uuid = '` + string(binaryUUID.Bytes()) + `'`,
			wantErr: false,
		},
		{
			name:    "Hex-encoded binary sanitized to valid UTF-8",
			input:   `SELECT * FROM cron_jobs WHERE uuid = X'deadbeef1234567890abcdef1234567890'`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeTracingQuery(tt.input)

			// Verify the result is valid UTF-8
			isValid := isValidUTF8(result)
			if !isValid && !tt.wantErr {
				t.Errorf("sanitizeTracingQuery() produced invalid UTF-8, want valid: %q", result)
			}
			if isValid && tt.wantErr {
				t.Errorf("sanitizeTracingQuery() produced valid UTF-8, want error: %q", result)
			}
		})
	}
}

// isValidUTF8 checks if a string contains only valid UTF-8 characters
func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == utf8.RuneError {
			return false
		}
	}
	return true
}

// TestGORMExplainBehavior demonstrates how GORM produces the "<binary>" placeholder
func TestGORMExplainBehavior(t *testing.T) {
	// This test documents the behavior we're working with
	// GORM's ExplainSQL produces "<binary>" for non-printable byte slices

	// Case 1: Printable bytes are kept as-is
	printableBytes := []byte("hello world")
	if isPrintable(string(printableBytes)) {
		t.Logf("Printable bytes: %q -> printable", printableBytes)
	}

	// Case 2: Binary UUID bytes produce "<binary>"
	binaryUUID := datatypes.NewBinUUIDv4()
	binaryBytes := binaryUUID.Bytes()
	if !isPrintable(string(binaryBytes)) {
		t.Logf("Binary UUID bytes: produce <binary> (non-printable)")
	}

	// Case 3: Verify our regex patterns match the expected inputs
	tests := []struct {
		re   *regexp.Regexp
		input string
		want bool
	}{
		{
			re:    testGORMBinaryPattern,
			input: `"<binary>"`,
			want:  true,
		},
		{
			re:    testGORMHexPattern,
			input: `X'deadbeef'`,
			want:  true,
		},
		{
			re:    testGORMInvalidUTF8Pattern,
			input: `"` + string(binaryBytes) + `"`,
			want:  true,
		},
	}

	for _, tt := range tests {
		matched := tt.re.MatchString(tt.input)
		if matched != tt.want {
			t.Errorf("Pattern matching %q: got %v, want %v", tt.input, matched, tt.want)
		}
	}
}

// isPrintable mimics GORM's isPrintable function
func isPrintable(s string) bool {
	for _, r := range s {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}
