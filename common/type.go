package common

import "database/sql"

type TransferTask struct  {
	Id int64 `json:"id"`
	RequestId int64 `json:"requestId"`
	SourceUrls []string `json:"sourceUrls"`
	DestinationType string `json:"destinationType"`
	DestinationBaseUrl string `json:"destinationBaseUrl"`
	ExecutorId sql.NullInt64 `json:"executorId"`
	Status string `json:"status"`// status is in Pending/Scheduled/Failed/Finished
}
