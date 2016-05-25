package common

type TransferTask struct  {
	Id int64 `json:"id"`
	JobUuid string `json:"jobUuid"`
	OriginUrls []string `json:"originUrls"`
	TargetType string `json:"targetType"`
	TargetBucket string `json:"targetBucket"`
	TargetAcl string `json:"targetAcl"`
	Status string `json:"status"`// status is in Pending/Scheduled/Failed/Finished
}
