package common

type TransferTask struct {
	Id           int64    `json:"id"`
	UId          string   `json:"uid"`
	JobUuid      string   `json:"jobUuid"`
	OriginUrls   []string `json:"originUrls"`
	TargetType   string   `json:"targetType"`
	TargetBucket string   `json:"targetBucket"`
	TargetAcl    string   `json:"targetAcl"`
	Status       string   `json:"status"` // status is in Pending/Scheduled/Running/Failed/Finished
	// keys for specific service, like S3 or Vass
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
	TargetCluster string `json:"targetCluster"`
}

type UrlUpdate struct {
	OriginUrl string `json:"originUrl"`
	TargetUrl string `json:"targetUrl"`
	TaskId    int64  `json:"taskId"`
	Status    string `json:"status"` // status is in Pending/Finished/Failed
	Size      int64  `json:"size"`
}

type UrlInfo struct {
	Url         string   `json:"url"`
	Size        int64    `json:"size"`
	Speed       int      `json:"speed"`
	Percentage  int      `json:"percentage"`
}
