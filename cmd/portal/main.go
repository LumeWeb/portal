package main

import (
	portalcmd "go.lumeweb.com/portal/cmd"
	_ "go.lumeweb.com/portal/plugins/standard"
	_ "go.lumeweb.com/portal/service"
)

func main() {
	portalcmd.Main()
}
