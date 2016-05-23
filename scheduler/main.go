package main

import (
	"github.com/mesos/mesos-go/mesosproto"
	"github.com/gogo/protobuf/proto"
	"github.com/mesos/mesos-go/scheduler"
	"os"
	"log"
	"flag"
	"database/sql"
)

var (
	// TODO: replace global variables with config parameters
	MASTER = "127.0.0.1:5050"
	EXECUTOR_URL = "http://127.0.0.1:8000/main_6"
	EXECUTE_CMD = "./main_6"
	API_BIND_ADDRESS = "0.0.0.0:8080"
	DB_CONNECTION_STRING = "root@tcp(127.0.0.1:3306)/optimus"
	REQUEST_BUFFER = 10000
	FILES_PER_TASK = 10
	EXECUTOR_IDLE_THRESHOLD = 3 // if an executor has taskRunning < THRESHOLD, treat it as idle
	CPU_PER_EXECUTOR = .5
	MEM_PER_TASK = 500.
	DISK_PER_TASK = 1024.

	logger *log.Logger
	db *sql.DB
	requestBuffer chan TransferRequest
)

type TransferTask struct  {
	Id int64 `json:"id"`
	RequestId int64 `json:"requestId"`
	SourceUrls []string `json:"sourceUrls"`
	DestinationType string `json:"destinationType"`
	DestinationBaseUrl string `json:"destinationBaseUrl"`
	ExecutorId sql.NullInt64 `json:"executorId"`
	Status string `json:"status"`// status is in Pending/Scheduled/Failed/Finished
}

func requestHandler()  {
	for {
		request := <- requestBuffer
		requestId, err := insertRequest(&request)
		if err != nil {
			logger.Println("Error inserting request: ", request, "with error: ", err)
			continue
		}
		tasks := []*TransferTask{}
		cursor := 0
		length := len(request.SourceUrls)
		for {
			t := TransferTask{
				RequestId: requestId,
				DestinationType: request.DestinationType,
				DestinationBaseUrl: request.DestinationBaseUrl,
				ExecutorId: sql.NullInt64{Valid: false},
				Status: "Pending",
			}
			if length > cursor + FILES_PER_TASK {
				t.SourceUrls = request.SourceUrls[cursor:cursor+FILES_PER_TASK]
				tasks = append(tasks, &t)
				cursor += FILES_PER_TASK
			} else {
				t.SourceUrls = request.SourceUrls[cursor:length]
				tasks = append(tasks, &t)
				break
			}
		}
		err = insertTasks(tasks)
		if err != nil {
			logger.Println("Error inserting tasks: ", tasks, "with error: ", err)
			continue
		}
	}
}

func init()  {
	flag.Parse() // mesos-go uses golang/glog, which requires parse flags first
}

func main()  {
	// TODO: log to a file
	logger = log.New(os.Stdout, "Optimus Prime: ", log.LstdFlags|log.Lshortfile)

	db = createDbConnection()
	defer db.Close()

	requestBuffer = make(chan TransferRequest, REQUEST_BUFFER)
	go requestHandler()

	go startApiServer()

	frameworkInfo := &mesosproto.FrameworkInfo{
		User: proto.String(""), // let mesos-go fill in
		Name: proto.String("Optimus Prime"),
	}

	config := scheduler.DriverConfig{
		Scheduler: newScheduler(),
		Framework: frameworkInfo,
		Master: MASTER,
	}
	driver, err := scheduler.NewMesosSchedulerDriver(config)
	if err != nil {
		logger.Println("Unable to create SchedulerDriver: ", err.Error())
	}
	status, err := driver.Run()
	if err != nil {
		logger.Println("Framework stopped with status ", status.String(),
			"and error ", err.Error())
	}
	logger.Println("Framework terminated")
}
