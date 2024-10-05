package main

import (
	_ "go.lumeweb.com/portal-plugin-dashboard"
	_ "go.lumeweb.com/portal-plugin-ipfs"
	portalcmd "go.lumeweb.com/portal/cmd"
	_ "go.lumeweb.com/portal/service"
)

func main() {
	portalcmd.Main()
}
