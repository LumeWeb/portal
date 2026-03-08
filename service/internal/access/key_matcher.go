package access

import (
	"regexp"
	"strings"

	"github.com/casbin/casbin/v3/util"
)

var (
	paramRegex = regexp.MustCompile(`:[^/]+`)
)

// KeyMatchEcho determines whether key1 matches the pattern of key2 using Echo's colon syntax.
// Supports :param for single path segments and :param/* for sub-resources.
// For example:
//   "/api/account/keys/ec6a9c56-2d04-4450-b6e3-1d10985e275b" matches "/api/account/keys/:keyID"
//   "/api/account/keys/123/subresource" matches "/api/account/keys/:keyID/*"
func KeyMatchEcho(key1 string, key2 string) bool {
	// Escape all static path segments first
	// This prevents regex metacharacters like '.' in paths from being interpreted as wildcards
	pattern := regexp.QuoteMeta(key2)

	// Now unescape the parts we want to be regex: :param and /*
	// Replace escaped :param with actual regex
	pattern = strings.ReplaceAll(pattern, regexp.QuoteMeta(":"), ":")
	pattern = paramRegex.ReplaceAllString(pattern, "[^/]+")

	// Replace escaped /* with .*
	pattern = strings.ReplaceAll(pattern, regexp.QuoteMeta("/*"), "/.*")

	return util.RegexMatch(key1, "^"+pattern+"$")
}

// KeyMatchEchoFunc is the wrapper for KeyMatchEcho.
func KeyMatchEchoFunc(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return false, nil
	}

	name1, ok1 := args[0].(string)
	name2, ok2 := args[1].(string)

	if !ok1 || !ok2 {
		return false, nil
	}

	return bool(KeyMatchEcho(name1, name2)), nil
}
