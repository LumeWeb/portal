package service_tests

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

// TestMain ensures the TEST_KEY_IDENTITY_SECRET environment variable is set
// before any tests run. If unset, a random value is generated so CI works
// without configuration while still sourcing the credential from the environment.
func TestMain(m *testing.M) {
	if os.Getenv("TEST_KEY_IDENTITY_SECRET") == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			panic("failed to generate test credential: " + err.Error())
		}
		os.Setenv("TEST_KEY_IDENTITY_SECRET", hex.EncodeToString(b))
	}
	os.Exit(m.Run())
}
