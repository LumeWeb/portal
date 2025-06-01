package event

func init() {
	InitCoreEvents()
}

func InitCoreEvents() {
	initBootCompleteEvent()
	initDownloadCompletedEvent()
	initUserActivatedEvent()
	initUserCreatedEvent()
	initUserServiceSubdomainSetEvent()
	initStorageObjectPinnedEvent()
	initStorageObjectUnpinnedEvent()
	// Add other core event init functions here
	// Plugin events should be initialized by their respective plugins
}
