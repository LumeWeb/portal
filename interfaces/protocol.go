package interfaces

type Protocol interface {
	Initialize(portal Portal) error
	Start() error
}

type ProtocolRegistry interface {
	Register(name string, protocol Protocol)
	Get(name string) (Protocol, error)
	All() map[string]Protocol
}
