package cli

import "go.lumeweb.com/portal/build"

func getVersion() string {
	return build.Default.GetVersion()
}
