package renter

import "github.com/go-co-op/gocron/v2"

const cronTaskRecordSiaRateName = "RecordSiaRate"
const cronTaskImportSiaPriceHistoryName = "ImportSiaPriceHistory"
const cronTaskUpdateSiaRenterPriceName = "UpdateSiaRenterPrice"

func nopArgsFactory() any {
	return nil
}

func cronTaskRecordSiaRateDefinition() gocron.JobDefinition {
	return gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(0, 0, 0)))
}
