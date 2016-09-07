package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
	"strconv"
)

func response(w http.ResponseWriter, statusCode int, message string) {
	w.WriteHeader(statusCode)
	w.Write([]byte(message))
}

// See api.markdown for details
func verifyRequest(r *http.Request, requestBody []byte) (accessKey string, _ bool) {
	dateString := r.Header.Get("x-date")
	if dateString == "" {
		return "", false
	}
	date, err := time.Parse("Mon, 02 Jan 2006 15:04:05 MST", dateString)
	if err != nil {
		return "", false
	}
	now := time.Now()
	diff := now.Sub(date)
	if diff > CONFIG.ApiAuthGraceTime || diff < -1*CONFIG.ApiAuthGraceTime {
		return "", false
	}
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", false
	}
	segments := strings.Split(authHeader, ":")
	accessKey = segments[0]
	if len(segments) < 2 {
		return "", false
	}
	messageMac, err := base64.StdEncoding.DecodeString(segments[1])
	if err != nil {
		return "", false
	}
	secretKey, err := getSecretKey(accessKey)
	if err != nil {
		return "", false
	}
	if err != nil {
		return "", false
	}
	hasher := md5.New()
	hasher.Write(requestBody)
	bodyMd5 := hex.EncodeToString(hasher.Sum(nil))
	mac := hmac.New(sha1.New, []byte(secretKey))
	mac.Write([]byte(r.Method + "\n" + dateString + "\n" + bodyMd5 + "\n" + r.URL.Path))
	expectedMac := mac.Sum(nil)
	return accessKey, hmac.Equal(expectedMac, messageMac)
}

type TransferRequest struct {
	accessKey     string
	OriginUrls    []string `json:"origin-files"`
	TargetType    string   `json:"target-type"` // in s3s/Vaas
	TargetBucket  string   `json:"target-bucket"`
	TargetAcl     string   `json:"target-acl"`
	uuid          string
	callbackToken string
	callbackUrl   string
	priority      int
}

type TransferResponse struct {
	JobId string `json:"jobid"`
}

func putTransferJobHandler(w http.ResponseWriter, r *http.Request) {
	if strings.ToUpper(r.Method) != "PUT" {
		w.Header().Set("Allow", "PUT")
		response(w, http.StatusMethodNotAllowed, "Only PUT method is allowed")
		return
	}
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		response(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	accessKey, verified := verifyRequest(r, requestBody)
	if !verified {
		response(w, http.StatusUnauthorized, "Failed to authenticate request")
		return
	}
	var priority int64
	priStr := r.URL.Query().Get("priority")
	if priStr == "" {
		priority = 16
	} else {
		priority, err = strconv.ParseInt(priStr, 10, 32)
		if err != nil {
			priority = 16
		}
	}
	if priority > 16 || priority < 0 {
		priority = 16
	}
	var req TransferRequest
	req.priority = int(priority)
	req.accessKey = accessKey
	err = json.NewDecoder(bytes.NewReader(requestBody)).Decode(&req)
	if err != nil {
		response(w, http.StatusBadRequest, "Bad JSON body")
		return
	}
	if len(req.OriginUrls) == 0 || req.TargetType == "" {
		response(w, http.StatusBadRequest, "Missing required field")
		return
	}
	if req.TargetType == "s3s" {
		if req.TargetBucket == "" || req.TargetAcl == "" {
			response(w, http.StatusBadRequest, "Missing required field")
			return
		}
	}

	query := r.URL.Query()
	req.callbackUrl = query.Get("callback")
	req.callbackToken = query.Get("token")

	req.uuid = newUuid()

	resp := TransferResponse{
		JobId: req.uuid,
	}
	respJson, err := json.Marshal(resp)
	if err != nil {
		response(w, http.StatusInternalServerError, "Server error")
		return
	}
	select {
	case requestBuffer <- req:
		w.Header().Set("Content-Type", "application/json")
		response(w, http.StatusAccepted, string(respJson))
	default:
		response(w, http.StatusInternalServerError, "Server too busy")
	}
}

type JobResult struct {
	JobUuid       string   `json:"jobid"`
	SuccessUrls   []string `json:"success-files"`
	FailedUrls    []string `json:"failed-files"`
	PendingUrls   []string `json:"queued-files"`
	SuspendedUrls []string `json:"suspended-files"`
}

type JobUrlResult struct {
	Url           string   `json:"url"`
	Size          int64    `json:"size"`
	Status        string   `json:"status"`
	Speed         int      `json:"speed"`
	Percentage    int      `json:"percentage"`
}

type JobList struct {
    JobUuid       string    `json:"jobid"`
	CreateTime    int64     `json:"create-time"`
	CompleteTime  int64     `json:"complete-time"`
	Status        string    `json:"satus"`
}

type FinishedSize struct {
	FinishedSize  int64     `json:"finished-size"`
}

type CurrentSpeed struct {
	UploadSpeed   int64     `json:"upload-speed"`
	DownloadSpeed int64     `json:"download-speed"`
}

func putJobCallback(url string, summary *JobResult) {
	jsonSummary, err := json.Marshal(summary)
	if err != nil {
		logger.Println("Error marshalling json: ", err)
		return
	}
	request, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonSummary))
	if err != nil {
		logger.Println("Error creating request: ", err)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		logger.Println("Error sending PUT request: ", err)
		return
	}
	logger.Println("Callback has been sent to ", url, "with response code ", response.Status)
}

func getJobStatusHandler(w http.ResponseWriter, r *http.Request) {
	if strings.ToUpper(r.Method) != "GET" {
		w.Header().Set("Allow", "GET")
		response(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		response(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	accessKey, verified := verifyRequest(r, requestBody)
	if !verified {
		response(w, http.StatusUnauthorized, "Failed to authenticate request")
		return
	}
	jobUuid := r.URL.Query().Get("jobid")
	if jobUuid == "" {
		response(w, http.StatusBadRequest, "Missing parameter jobid")
		return
	}
	if !userOwnsJob(accessKey, jobUuid) {
		response(w, http.StatusForbidden, "Your key has no access to job "+jobUuid)
		return
	}
	summary, err := getJobSummary(jobUuid)
	if err != nil {
		response(w, http.StatusInternalServerError, "Cannot get job status")
		return
	}
	jsonSummary, err := json.Marshal(summary)
	if err != nil {
		response(w, http.StatusInternalServerError, "Cannot get job status")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	response(w, http.StatusOK, string(jsonSummary))
}

func postSuspendJobHandler(w http.ResponseWriter, r *http.Request) {
	if strings.ToUpper(r.Method) != "POST" {
		w.Header().Set("Allow", "POST")
		response(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		response(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	accessKey, verified := verifyRequest(r, requestBody)
	if !verified {
		response(w, http.StatusUnauthorized, "Failed to authenticate request")
		return
	}
	jobUuid := r.URL.Query().Get("jobid")
	if jobUuid == "" {
		response(w, http.StatusBadRequest, "Missing parameter jobid")
		return
	}
	if !userOwnsJob(accessKey, jobUuid) {
		response(w, http.StatusForbidden, "Your key has no access to job "+jobUuid)
		return
	}

	err = suspendJob(jobUuid)
	if err != nil {
		response(w, http.StatusInternalServerError, "Cannot suspend job")
		return
	}
	response(w, http.StatusOK, string(""))
}

func postResumeJobHandler(w http.ResponseWriter, r *http.Request) {
	if strings.ToUpper(r.Method) != "POST" {
		w.Header().Set("Allow", "POST")
		response(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		response(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	accessKey, verified := verifyRequest(r, requestBody)
	if !verified {
		response(w, http.StatusUnauthorized, "Failed to authenticate request")
		return
	}
	jobUuid := r.URL.Query().Get("jobid")
	if jobUuid == "" {
		response(w, http.StatusBadRequest, "Missing parameter jobid")
		return
	}
	if !userOwnsJob(accessKey, jobUuid) {
		response(w, http.StatusForbidden, "Your key has no access to job "+jobUuid)
		return
	}

	err = resumeJob(jobUuid)
	if err != nil {
		response(w, http.StatusInternalServerError, "Cannot resume job")
		return
	}
	response(w, http.StatusOK, string(""))
}

func putUserSchedule(w http.ResponseWriter, r *http.Request) {
	if strings.ToUpper(r.Method) != "PUT" {
		w.Header().Set("Allow", "PUT")
		response(w, http.StatusMethodNotAllowed, "Only PUT method is allowed")
		return
	}
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		response(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	accessKey, verified := verifyRequest(r, requestBody)
	if !verified {
		response(w, http.StatusUnauthorized, "Failed to authenticate request")
		return
	}
	var spans []Span
	err = json.NewDecoder(bytes.NewReader(requestBody)).Decode(&spans)
	if err != nil {
		response(w, http.StatusBadRequest, "Bad JSON body")
		return
	}
	length := len(spans)
	if length > 5 {
		response(w, http.StatusBadRequest, "Maximum number of entries are 5")
		return
	}
	for i := 0; i < length; i++ {
		if spans[i].Start >= spans[i].End {
			response(w, http.StatusBadRequest, "The start time is greater than end time")
			return
		}
	}
	for i := 0; i < length; i++ {
		for j := i + 1; j < length; j++ {
			if spans[i].Start <= spans[j].End && spans[i].End >= spans[j].Start {
				response(w, http.StatusBadRequest, "There are overlaps in the entries")
				return
			}
		}
	}
	err = updateScheduleEntry(accessKey, spans)
	if err != nil {
		response(w, http.StatusBadRequest, "Cannot set schedule table")
		return
	}

	response(w, http.StatusOK, "")
}

func getJobDetail(w http.ResponseWriter, r *http.Request) {
	if strings.ToUpper(r.Method) != "GET" {
		w.Header().Set("Allow", "GET")
		response(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		response(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	accessKey, verified := verifyRequest(r, requestBody)
	if !verified {
		response(w, http.StatusUnauthorized, "Failed to authenticate request")
		return
	}
	jobUuid := r.URL.Query().Get("jobid")
	if jobUuid == "" {
		response(w, http.StatusBadRequest, "Missing parameter jobid")
		return
	}
	if !userOwnsJob(accessKey, jobUuid) {
		response(w, http.StatusForbidden, "Your key has no access to job "+jobUuid)
		return
	}
	var result []JobUrlResult
	err = getJobUrlDetail(jobUuid, &result)
	if err != nil {
		response(w, http.StatusInternalServerError, "Cannot get url detail")
		return
	}
	jsonResult, err := json.Marshal(result)
	if err != nil {
		response(w, http.StatusInternalServerError, "Cannot get url detail")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	response(w, http.StatusOK, string(jsonResult))
}

func getJobList(w http.ResponseWriter, r *http.Request) {
	if strings.ToUpper(r.Method) != "GET" {
		w.Header().Set("Allow", "GET")
		response(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		response(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	accessKey, verified := verifyRequest(r, requestBody)
	if !verified {
		response(w, http.StatusUnauthorized, "Failed to authenticate request")
		return
	}
	stime := r.URL.Query().Get("stime")
	etime := r.URL.Query().Get("etime")
	statusStr := r.URL.Query().Get("status")
	jobid := r.URL.Query().Get("jobid")
	status := 0
	if len(statusStr) != 0 {
		status, err = strconv.Atoi(statusStr)
		if err != nil {
			logger.Println("Failed to convert to int")
		}
	}
	logger.Println("stime", stime, "etime", etime, "status", statusStr, "jobid", jobid)

	var result []JobList
	err = queryJobList(accessKey, stime, etime, status, jobid, &result)
	if err != nil {
		response(w, http.StatusInternalServerError, "Cannot get url detail")
		return
	}
	jsonResult, err := json.Marshal(result)
	if err != nil {
		response(w, http.StatusInternalServerError, "Cannot get url detail")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	response(w, http.StatusOK, string(jsonResult))
	//response(w, http.StatusOK, "")
}

func getFinishedSize(w http.ResponseWriter, r *http.Request) {
	if strings.ToUpper(r.Method) != "GET" {
		w.Header().Set("Allow", "GET")
		response(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		response(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	accessKey, verified := verifyRequest(r, requestBody)
	if !verified {
		response(w, http.StatusUnauthorized, "Failed to authenticate request")
		return
	}

	totalSize, err := queryFinishedSize(accessKey)
	if err != nil {
		response(w, http.StatusInternalServerError, "Cannot get total size")
		return
	}

	jsonResult, err := json.Marshal(FinishedSize{totalSize})
	if err != nil {
		response(w, http.StatusInternalServerError, "Cannot get url detail")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	response(w, http.StatusOK, string(jsonResult))
}

func getCurrSpeed(w http.ResponseWriter, r *http.Request) {
	if strings.ToUpper(r.Method) != "GET" {
		w.Header().Set("Allow", "GET")
		response(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		response(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	accessKey, verified := verifyRequest(r, requestBody)
	if !verified {
		response(w, http.StatusUnauthorized, "Failed to authenticate request")
		return
	}

	var jobUuids []string
	err = queryScheduledJobUuids(accessKey, &jobUuids)
	if err != nil {
		response(w, http.StatusInternalServerError, "Cannot get total size")
		return
	}

	var uSpeed, dSpeed, finishedSize int64
	for _, uuid := range jobUuids {
		var result []JobUrlResult
		err = getJobUrlDetail(uuid, &result)
		if err != nil {
			response(w, http.StatusInternalServerError, "Cannot get url detail")
			return
		}
		for _, urlInfo := range result {
			if urlInfo.Percentage == 100 {
				finishedSize += int64(urlInfo.Size)
			} else if urlInfo.Percentage > 50 {
				uSpeed += int64(urlInfo.Speed)
			} else if urlInfo.Percentage > 0 {
				dSpeed += int64(urlInfo.Speed)
			}
		}
	}

	jsonResult, err := json.Marshal(CurrentSpeed{uSpeed, dSpeed})
	if err != nil {
		response(w, http.StatusInternalServerError, "Cannot get url detail")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	response(w, http.StatusOK, string(jsonResult))
}

func startApiServer() {
	http.HandleFunc("/transferjob", putTransferJobHandler)
	http.HandleFunc("/status", getJobStatusHandler)
	http.HandleFunc("/suspendjob", postSuspendJobHandler)
	http.HandleFunc("/resumejob", postResumeJobHandler)
	http.HandleFunc("/schedule", putUserSchedule)
	http.HandleFunc("/jobdetail", getJobDetail)
	http.HandleFunc("/joblist", getJobList)
	http.HandleFunc("/finishedsize", getFinishedSize)
	http.HandleFunc("/currentspeed", getCurrSpeed)
	http.Handle("/", http.FileServer(http.Dir(CONFIG.WebRoot)))
	logger.Println("Starting API server...")
	err := http.ListenAndServe(CONFIG.ApiBindAddress, nil)
	if err != nil {
		panic(fmt.Sprintf("Error starting API server: %v", err))
	}
}
