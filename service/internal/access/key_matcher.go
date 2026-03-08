package access

import (
	"regexp"
	"strings"

	"github.com/casbin/casbin/v3/util"
)

// KeyMatchEcho determines whether key1 matches the pattern of key2 using Echo's colon syntax.
// Supports :param for single path segments and :param/* for sub-resources.
// For example:
//   "/api/account/keys/ec6a9c56-2d04-4450-b6e3-1d10985e275b" matches "/api/account/keys/:keyID"
//   "/api/account/keys/123/subresource" matches "/api/account/keys/:keyID/*"
func KeyMatchEcho(key1 string, key2 string) bool {
	// Try standard KeyMatch2 first for :param syntax
	if util.KeyMatch2(key1, key2) {
		return true
	}

	// Handle :param/* patterns where the path must have at least one segment after the parameter
	key2 = strings.Replace(key2, "/*", "/.*", -1)

	// Check if pattern contains :param/* syntax
	re := regexp.MustCompile(`:([^/]+)/\.\*`)
	if !re.MatchString(key2) {
		return false
	}

	// Replace :param/.* with regex that requires at least one segment after the parameter
	key2 = re.ReplaceAllString(key2, "([^/]+)/.+$")

	// Replace other :param patterns
	re = regexp.MustCompile(`:[^/]+`)
	key2 = re.ReplaceAllString(key2, "$1[^/]+$2")

	return util.RegexMatch(key1, "^"+key2+"$")
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
