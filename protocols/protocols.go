package protocols

func Init(registry ProtocolRegistry) error {
	registry.Register("s5", NewS5Protocol())
	return nil
}
