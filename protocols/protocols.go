package protocols

import "git.lumeweb.com/LumeWeb/portal/interfaces"

func Init(registry interfaces.ProtocolRegistry) error {
	registry.Register("s5", NewS5Protocol())
	return nil
}
