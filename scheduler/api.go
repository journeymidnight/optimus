package main

import (
	"strings"
	"encoding/json"
	"net/http"
	"fmt"
	"time"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
)

var (
	AUTH_GRACE_TIME = 5 * time.Minute
)

func response(w http.ResponseWriter, statusCode int, message string)  {
	w.WriteHeader(statusCode)
	w.Write([]byte(message))
}

func verifyRequest(r *http.Request) bool {
	dateString := r.Header.Get("x-date")
	if dateString == "" {return false}
	date, err := time.Parse("Mon, 02 Jan 2006 15:04:05 GMT", dateString)
	if err != nil {return false}
	now := time.Now()
	diff := now.Sub(date)
	if diff > AUTH_GRACE_TIME || diff < -1 * AUTH_GRACE_TIME {
		return false
	}
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {return false}
	segments := strings.Split(authHeader, ":")
	accessKey := segments[0]
	if len(segments) < 2 {return false}
	messageMac, err := base64.StdEncoding.DecodeString(segments[1])
	if err != nil {return false}
	secretKey, err := getSecretKey(accessKey)
	if err != nil {return false}
	mac := hmac.New(sha1.New, []byte(secretKey))
	mac.Write([]byte(r.Method + "\n" + dateString + "\n" + r.Host + "\n" + r.URL.Path))
	expectedMac := mac.Sum(nil)
	return hmac.Equal(expectedMac, messageMac)
}

type TransferRequest struct {
	OriginUrls []string `json:"origin-files"`
	TargetType string `json:"target-type"` // in s3s/Vaas
	TargetBucket string `json:"target-bucket"`
	TargetAcl string `json:"target-acl"`
	uuid string
	callbackToken string
	callbackUrl string
}

type TransferResponse struct {
	JobId string `json:"jobid"`
}

func putTransferJobHandler(w http.ResponseWriter, r *http.Request)  {
	if strings.ToUpper(r.Method) != "PUT" {
		response(w, http.StatusBadRequest, "Only PUT method is allowed")
		return
	}
	if !verifyRequest(r) {
		response(w, http.StatusForbidden, "Failed to authenticate request")
		return
	}
	var req TransferRequest
	err := json.NewDecoder(r.Body).Decode(&req)
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

func startApiServer()  {
	http.HandleFunc("/transferjob", putTransferJobHandler)
	http.Handle("/", http.FileServer(http.Dir("../web")))
	logger.Println("Starting API server...")
	err := http.ListenAndServe(API_BIND_ADDRESS, nil)
	if err != nil {
		panic(fmt.Sprintf("Error starting API server: %v", err))
	}
}

