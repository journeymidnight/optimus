package main

import (
	"time"
	"sync"
)

const MAX_PRI_NUMBER = 10

type Span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type schedUserInfo struct {
	ak                 string
	timeSpans          []Span
}

var (
	schedUsers         [MAX_PRI_NUMBER][]schedUserInfo
	currUser           string
	lock               sync.Mutex
)

func getNextUser() (string, error) {
	now := time.Now()
	hour := now.Hour()

	lock.Lock()
	defer lock.Unlock()
	prevUser := currUser
	currUser = ""
	for pri := 0; pri < MAX_PRI_NUMBER; pri++ {
		if schedUsers[pri] == nil {
			continue
		}

		var idx int
		var unHitUsers []string
		for idx = 0; idx < len(schedUsers[pri]); idx++ {
			var hit bool
			for _, span := range schedUsers[pri][idx].timeSpans {
				if hour >= span.Start && hour < span.End {
					hit = true
					break
				}
			}
		    if hit {
				continue
			}
			unHitUsers = append(unHitUsers, schedUsers[pri][idx].ak)
		}
		if len(unHitUsers) == 0 {
			continue
		}

		for idx = 0; idx < len(unHitUsers); idx++ {
			if unHitUsers[idx] == prevUser {
				break
			}
		}

		if idx == len(unHitUsers) {
			currUser = unHitUsers[0]
		} else {
			currUser = unHitUsers[(idx + 1) % len(unHitUsers)]
		}
		break
	}
	//logger.Println("getNextUser: user:", currUser)
	return currUser, nil
}

func removeSchedUser(ak string) error {
	lock.Lock()
	defer lock.Unlock()
	for pri := 0; pri < MAX_PRI_NUMBER; pri++ {
		if schedUsers[pri] == nil {
			continue
		}

		var idx int
		for idx = 0; idx < len(schedUsers[pri]); idx++ {
			if ak == schedUsers[pri][idx].ak {
				break
			}
		}
		if idx < len(schedUsers[pri]) {
			schedUsers[pri] = append(schedUsers[pri][:idx],schedUsers[pri][idx + 1:]...)
			logger.Println("removeSchedUser: user:", ak)
			break
		}
	}
	return nil
}

func isSchedUserExist(ak string) (bool, error) {
	lock.Lock()
	defer lock.Unlock()
	for pri := 0; pri < MAX_PRI_NUMBER; pri++ {
		if schedUsers[pri] == nil {
			continue
		}
		for idx := 0; idx < len(schedUsers[pri]); idx++ {
			if schedUsers[pri][idx].ak == ak {
				return true, nil
			}
		}
	}
	return false, nil
}

func addSchedUser(pri int, schedUser schedUserInfo) error{
	lock.Lock()
	defer lock.Unlock()
	var idx int
	for idx = 0; idx < len(schedUsers[pri]); idx++ {
		if schedUsers[pri][idx].ak == schedUser.ak {
			break
		}
	}
	if idx < len(schedUsers[pri]) {
		schedUsers[pri] = append(schedUsers[pri][:idx], schedUsers[pri][idx+1:]...)
	}
	schedUsers[pri] = append(schedUsers[pri], schedUser)
	for idx = 0; idx < len(schedUsers[pri]); idx++ {
		logger.Println("addSchedUser(): user:", schedUser.ak, "pri:", pri, "spans:", schedUser.timeSpans)
	}
	return nil
}

func chkAndAddSchedUser(ak string) error {
	exist, err := isSchedUserExist(ak)
	if err != nil {
		logger.Println("Error checking if scheduler user is exist:", err)
		return err
	}
	if exist {
		return nil
	}
	pri, err := getUserPriority(ak)
	if err != nil {
		logger.Println("Error get user priority from db:", err)
		return err
	}
	var schedUser schedUserInfo
	err = getUserTimeSpans(ak, &schedUser.timeSpans)
	if err != nil {
		logger.Println("Error get user time spans: ", err)
		return err
	}
	schedUser.ak = ak
	err = addSchedUser(pri, schedUser)
	if err != nil {
		logger.Println("Error add sched user: ", err)
		return err
	}
	return nil
}

func chkAndUpdateSchedUser(ak string, spans []Span) error {
	exist, err := isSchedUserExist(ak)
	if err != nil {
		logger.Println("Error checking if scheduler user is exist:", err)
		return err
	}
	if !exist {
		return nil
	}
	pri, err := getUserPriority(ak)
	if err != nil {
		logger.Println("Error get user priority from db:", err)
		return err
	}
	var schedUser schedUserInfo
	schedUser.ak = ak
	if len(spans) != 0 {
		schedUser.timeSpans = spans
	}
	logger.Println("chkAndUpdateSchedUser(): schedUser.timeSpans:", schedUser.timeSpans)
	err = addSchedUser(pri, schedUser)
	if err != nil {
		logger.Println("Error add sched user: ", err)
		return err
	}
	return nil
}

func updateScheduleEntry(accessKey string, spans []Span) error {
	err := chkAndUpdateSchedUser(accessKey, spans)
	if err != nil {
		logger.Println("Error check and update sched user: ", err)
		return err
	}
	err = delAndInsertScheTable(accessKey, spans)
	if err != nil {
		logger.Println("Error updating schedule table with error ", err)
		return err
	}
	return nil
}

func initScheduledUsers() error {
	var aks []string
	err := getPendingUsers(&aks)
	if err != nil {
		logger.Println("Error get pending user: ", err)
		return err
	}
	for _, ak := range aks {
		var schedUser schedUserInfo
		pri, err := getUserPriority(ak)
		if err != nil {
			logger.Println("Error get user priority ", err)
			continue
		}
		err = getUserTimeSpans(ak, &schedUser.timeSpans)
		if err != nil {
			logger.Println("Error get user time spans ", err)
			continue
		}
		logger.Println("user:", ak, "pri:", pri, "spans:", schedUser.timeSpans)
		schedUser.ak = ak
		schedUsers[pri] = append(schedUsers[pri], schedUser)
	}
	currUser = ""
	return nil
}
