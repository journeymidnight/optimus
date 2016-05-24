package main

import (
	"strings"
	"encoding/json"
	"net/http"
	"fmt"
	"time"
	"crypto/hmac"
	"crypto/sha1"
)

var (
	AUTH_GRACE_TIME = 5 * time.Minute
)

type TransferRequest struct {
	SourceUrls []string `json:"sourceUrls"`
	DestinationType string `json:"destinationType"`
	DestinationBaseUrl string `json:"destinationBaseUrl"`
}

func response(w http.ResponseWriter, statusCode int, message string)  {
	w.WriteHeader(statusCode)
	w.Write([]byte(message))
}

func verifyRequest(r *http.Request) bool {
	dateString := r.Header.Get("Date")
	if dateString == "" {return false}
	date, err := time.Parse("Tue, 24 May 2016 03:25:18 GMT", dateString)
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
	messageMac := segments[1]
	secretKey, err := getSecretKey(accessKey)
	if err != nil {return false}
	mac := hmac.New(sha1.New, []byte(secretKey))
	mac.Write([]byte(r.Method + '\n' + dateString + '\n' + r.URL.Host + '\n' + r.URL.Path))
	expectedMac := mac.Sum(nil)
	return hmac.Equal(expectedMac, messageMac)
}

func postTransferHandler(w http.ResponseWriter, r *http.Request)  {
	if strings.ToUpper(r.Method) != "POST" {
		response(w, http.StatusBadRequest, "Only POST method is allowed")
		return
	}
	var req TransferRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		response(w, http.StatusBadRequest, "Bad JSON body")
		return
	}
	if len(req.SourceUrls) == 0 || req.DestinationType == "" ||
	req.DestinationBaseUrl == "" {
		response(w, http.StatusBadRequest, "Missing required field")
		return
	}
	select {
	case requestBuffer <- req:
		response(w, http.StatusAccepted, "Transfer request submitted")
	default:
		response(w, http.StatusInternalServerError, "Server too busy")
	}
}

func startApiServer()  {
	http.HandleFunc("/transfer", postTransferHandler)
	logger.Println("Starting API server...")
	err := http.ListenAndServe(API_BIND_ADDRESS, nil)
	if err != nil {
		panic(fmt.Sprintf("Error starting API server: %v", err))
	}
}

