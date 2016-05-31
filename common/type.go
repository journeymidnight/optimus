package common

type TransferTask struct  {
	Id int64 `json:"id"`
	JobUuid string `json:"jobUuid"`
	OriginUrls []string `json:"originUrls"`
	TargetType string `json:"targetType"`
	TargetBucket string `json:"targetBucket"`
	TargetAcl string `json:"targetAcl"`
	Status string `json:"status"`// status is in Pending/Scheduled/Failed/Finished
	// keys for specific service, like S3 or Vass
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

type UrlUpdate struct {
	OriginUrl string `json:"originUrl"`
	TargetUrl string `json:"targetUrl"`
	TaskId int64 `json:"taskId"`
	Status string `json:"status"` // status is in Pending/Finished/Failed
}
