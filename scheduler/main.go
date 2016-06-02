package main

import (
	"database/sql"
	"flag"
	"github.com/gogo/protobuf/proto"
	"github.com/mesos/mesos-go/mesosproto"
	"github.com/mesos/mesos-go/scheduler"
	"log"
	"os"

	"encoding/json"
	"git.letv.cn/zhangcan/optimus/common"
	"time"
)

var (
	CONFIG        Config
	logger        *log.Logger
	db            *sql.DB
	requestBuffer chan TransferRequest
)

type Config struct {
	LogDirectory             string
	MesosMaster              string
	ExecutorUrl              string
	ExecuteCommand           string
	ApiBindAddress           string
	DatabaseConnectionString string
	WebRoot			 string
	ApiAuthGraceTime 	 time.Duration // allowed time-shift for x-date header
	RequestBufferSize        int
	FilesPerTask             int
	ExecutorIdleThreshold    int           // if an executor has taskRunning < THRESHOLD, treat it as idle
	TaskScheduleTimeout      time.Duration // if a task has been scheduled for certain time and not
	// become "Running", consider it as lost and reschedule it
	CpuPerExecutor float64
	MemoryPerTask  float64
	DiskPerTask    float64
}

func requestHandler() {
	for {
		request := <-requestBuffer
		err := insertJob(&request)
		if err != nil {
			logger.Println("Error inserting request: ", request, "with error: ", err)
			continue
		}
		accessKey, secretKey := getKeysForUser(request.accessKey, request.TargetType)
		tasks := []*common.TransferTask{}
		cursor := 0
		length := len(request.OriginUrls)
		for {
			t := common.TransferTask{
				JobUuid:      request.uuid,
				TargetType:   request.TargetType,
				TargetBucket: request.TargetBucket,
				TargetAcl:    request.TargetAcl,
				Status:       "Pending",
				AccessKey:    accessKey,
				SecretKey:    secretKey,
			}
			if length > cursor+CONFIG.FilesPerTask {
				t.OriginUrls = request.OriginUrls[cursor : cursor+CONFIG.FilesPerTask]
				tasks = append(tasks, &t)
				cursor += CONFIG.FilesPerTask
			} else {
				t.OriginUrls = request.OriginUrls[cursor:length]
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

type scheduledTask struct {
	id           int64
	scheduleTime time.Time
}

func rescheduler() {
	for {
		scheduledTasks := getScheduledTasks()
		now := time.Now()
		tasksToReschedule := []*scheduledTask{}
		for _, task := range scheduledTasks {
			if diff := now.Sub(task.scheduleTime); diff > CONFIG.TaskScheduleTimeout {
				tasksToReschedule = append(tasksToReschedule, task)
			}
		}
		rescheduleTasks(tasksToReschedule)

		time.Sleep(CONFIG.TaskScheduleTimeout)
	}
}

func init() {
	flag.Parse() // mesos-go uses golang/glog, which requires to parse flags first
}

func main() {
	configFile, err := os.Open("/etc/optimus.json")
	if err != nil {
		panic(err.Error())
	}
	defer configFile.Close()
	jsonDecoder := json.NewDecoder(configFile)
	err = jsonDecoder.Decode(&CONFIG)
	if err != nil {
		panic("Error parsing config file /etc/optimus.json with error: " + err.Error())
	}

	logFile, err := os.OpenFile(CONFIG.LogDirectory+"/optimus.log",
		os.O_APPEND|os.O_RDWR|os.O_CREATE, 0664)
	if err != nil {
		panic(err.Error())
	}
	logger = log.New(logFile, "Optimus: ", log.LstdFlags|log.Lshortfile)

	logger.Println("CONFIG: ", CONFIG)

	db = createDbConnection()
	defer db.Close()
	clearExecutors()

	requestBuffer = make(chan TransferRequest, CONFIG.RequestBufferSize)
	go requestHandler()

	go startApiServer()

	go rescheduler()

	frameworkInfo := &mesosproto.FrameworkInfo{
		User: proto.String(""), // let mesos-go fill in
		Name: proto.String("Optimus Prime"),
	}

	config := scheduler.DriverConfig{
		Scheduler: newScheduler(),
		Framework: frameworkInfo,
		Master:    CONFIG.MesosMaster,
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
