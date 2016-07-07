package main

import (
	"time"
	"sync"
)

type JobEntry struct {
	JobUuid       string `json:"jobid"`
	CreateTime    string `json:"create-time"`
	CompleteTime  string `json:"complete-time"`
	Status        string `json:"status"`
}

type Span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

var (
	ScheEntries  map[string][]Span
	Lock         sync.Mutex

	notify       chan bool
)

func getUserHitInfo(accessKey string) (bool, error) {
	var hit bool
	now := time.Now()
	hour := now.Hour()
	Lock.Lock()
	spans := ScheEntries[accessKey]
	for _, span := range spans {
		if span.Start <= hour && hour < span.End {
			hit = true
		}
	}
	Lock.Unlock()
	return hit, nil
}

func getHitInfo(hour int, hitMap map[string]bool) error {
	Lock.Lock()
	for key, spans := range ScheEntries {
		logger.Println("ak: ", key, "spans:", spans)
		hitMap[key] = false
		for _, span := range spans {
			if span.Start <= hour && hour < span.End {
				hitMap[key] = true
			}
		}
	}
	Lock.Unlock()
	return nil
}

func addNewEntry(accessKey string, spans []Span) error {
	Lock.Lock()
	ScheEntries[accessKey] = spans
	Lock.Unlock()
	return nil
}

func updateScheduleEntry(accessKey string, spans []Span) error {
	err := addNewEntry(accessKey, spans)
	if err != nil {
		return err
	}

	err = delAndInsertScheTable(accessKey, spans)
	if err != nil {
		logger.Println("Error updating schedule table with error ", err)
		return err
	}

	notify <- false
	return nil
}

func suspendOrResumeJob(hitMap map[string]bool) error {
	for key, value := range hitMap {
		var requestedJobs []JobEntry
		if !value  { // convert Pending to Suspended
			_, err := getRequestedJobs(key, &requestedJobs, "Pending")
			if err != nil {
				logger.Println("Error getting requested jobs", "with error ", err)
				return err
			}
			logger.Println("Pending jobs:", len(requestedJobs))
			for _, jobEntry := range requestedJobs {
				err = suspendJob(jobEntry.JobUuid)
				if err != nil {
					logger.Println("Error suspending job", jobEntry.JobUuid, "with error ", err)
					return err
				}
			}
		} else {  // convert Suspended to Pending
			_, err := getRequestedJobs(key, &requestedJobs, "Suspended")
			if err != nil {
				logger.Println("Error getting requested jobs", "with error ", err)
				return err
			}
			logger.Println("Suspended jobs:", len(requestedJobs))
			for _, jobEntry := range requestedJobs {
				err = resumeJob(jobEntry.JobUuid)
				if err != nil {
					logger.Println("Error resuming job", jobEntry.JobUuid, "with error ", err)
					return err
				}
			}
		}
	}
	return nil
}

func initUserSchedule() error {
	ScheEntries = make(map[string][]Span)

	err := getScheduleEntries(ScheEntries)
	if err != nil {
		logger.Println("Error getting schedule entries", "with error ", err)
		return err
	}

	now := time.Now()
	hour := now.Hour()
	hitMap := make(map[string]bool)
	err = getHitInfo(hour, hitMap)
	if err != nil {
		logger.Println("Error getting hit info", "with error ", err)
		return err
	}

	err = suspendOrResumeJob(hitMap)
	if err != nil {
		logger.Println("Error suspending or resuming for user", "with error ", err)
		return err
	}

	notify = make (chan bool, 1)
	return nil
}

func processScheduleEvent() error {
	timeout := true
	for {
		now := time.Now()
		hour := now.Hour()
		minute := now.Minute()
		if timeout {
			go func(m int) {
				time.Sleep(time.Duration(m) * time.Minute)
				notify <- true
			}(60 - minute + 1)
		}

		select {
		case timeout = <- notify:
			logger.Println("Need check schedule table!")
		}

		hitMap := make(map[string]bool)
		err := getHitInfo(hour, hitMap)
		if err != nil {
			logger.Println("Error getting hit info", "with error ", err)
			return err
		}

		err = suspendOrResumeJob(hitMap)
		if err != nil {
			logger.Println("Error suspending or resuming for user", "with error ", err)
			return err
		}
	}
	return nil
}
