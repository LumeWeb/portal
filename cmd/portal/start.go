package main

import "git.lumeweb.com/LumeWeb/portal/interfaces"

type startFunc func(p interfaces.Portal) error

func startProtocolRegistry(p interfaces.Portal) error {
	for _, _func := range p.ProtocolRegistry().All() {
		err := _func.Start()
		if err != nil {
			return err
		}
	}

	return nil
}

func startDatabase(p interfaces.Portal) error {
	return p.DatabaseService().Start()
}

func getStartList() []startFunc {
	return []startFunc{
		startProtocolRegistry,
		startDatabase,
	}
}
