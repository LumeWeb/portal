package s5

import (
	"git.lumeweb.com/LumeWeb/portal/config"
	s5config "github.com/LumeWeb/libs5-go/config"
)

var _ config.ProtocolConfig = (*Config)(nil)

type Config struct {
	*s5config.NodeConfig `mapstructure:",squash"`
	DbPath               string `mapstructure:"db_path"`
}

func (c Config) Defaults() map[string]interface{} {

	defaults := map[string]interface{}{}

	defaults["p2p.peers.initial"] = []string{
		"wss://z2DWuWNZcdSyZLpXFK2uCU3haaWMXrDAgxzv17sDEMHstZb@s5.garden/s5/p2p",
		"wss://z2DWuPbL5pweybXnEB618pMnV58ECj2VPDNfVGm3tFqBvjF@s5.ninja/s5/p2p",
	}
	defaults["db_path"] = "s5.db"
	defaults["p2p.max_connection_attempts"] = 10

	return defaults
}
