package interfaces

type Service interface {
	Init() error
	Start() error
}
