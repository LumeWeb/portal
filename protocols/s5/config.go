package s5

import (
	s5config "git.lumeweb.com/LumeWeb/libs5-go/config"
	"git.lumeweb.com/LumeWeb/portal/config"
)

var _ config.ProtocolConfig = (*Config)(nil)

type Config struct {
	s5config.NodeConfig
	DbPath string `mapstructure:"db_path"`
}

func (c Config) Defaults() map[string]interface{} {

	defaults := map[string]interface{}{}

	defaults["p2p.network.peers"] = []string{
		"ss://z2DWuWNZcdSyZLpXFK2uCU3haaWMXrDAgxzv17sDEMHstZb@s5.garden/s5/p2p",
		"wss://z2DWuPbL5pweybXnEB618pMnV58ECj2VPDNfVGm3tFqBvjF@s5.ninja/s5/p2p",
	}
	defaults["db_path"] = "s5.db"

	return defaults
}
