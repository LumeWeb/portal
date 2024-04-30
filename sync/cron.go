package sync

const cronTaskVerifyObjectName = "SyncVerifyObject"

type cronTaskVerifyObjectArgs struct {
	Object []FileMeta `json:"object"`
}

func cronTaskVerifyObjectArgsFactory() any {
	return &cronTaskVerifyObjectArgs{}
}

func cronTaskVerifyObject(args *cronTaskVerifyObjectArgs, sync *SyncServiceDefault) error {
	return nil
}
