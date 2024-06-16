package main

import (
	portalcmd "go.lumeweb.com/portal/cmd"
	_ "go.lumeweb.com/portal/plugins/standard"
)

func main() {
	portalcmd.Main()
}
