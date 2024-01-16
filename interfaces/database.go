package interfaces

type Database interface {
	Init(p Portal) error
	Start() error
}
