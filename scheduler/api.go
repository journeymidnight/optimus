package main

import (
	"strings"
	"encoding/json"
	"net/http"
	"fmt"
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

