package interfaces

type Protocol interface {
	Initialize(portal Portal) error
	Start() error
}
