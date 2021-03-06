package main

import (
	"database/sql"
	"flag"
	"github.com/gogo/protobuf/proto"
	"github.com/mesos/mesos-go/mesosproto"
	"github.com/mesos/mesos-go/scheduler"
	"github.com/garyburd/redigo/redis"
	"github.com/FZambia/go-sentinel"
	"log"
	"os"

	"encoding/json"
	"legitlab.letv.cn/optimus/optimus/common"
	"time"
	"os/signal"
	"syscall"
	"errors"
)

var (
	CONFIG        Config
	logger        *log.Logger
	db            *sql.DB
	pool          *redis.Pool
	requestBuffer chan TransferRequest
	cluster       map[string]string
	userMaxSpeed  map[string]int64
)

type Config struct {
	LogDirectory             string
	MesosMaster              string
	ExecutorUrl              string
	ExecuteCommand           string
	ApiBindAddress           string
	DatabaseConnectionString string
	WebRoot                  string
	RedisMasterName          string
	RedisAddress             []string
	ApiAuthGraceTime         time.Duration // allowed time-shift for x-date header
	RequestBufferSize        int
	FilesPerTask             int
	ExecutorIdleThreshold    int           // if an executor has taskRunning < THRESHOLD, treat it as idle
	TaskScheduleTimeout      time.Duration // if a task has been scheduled for certain time and not
	// become "Running", consider it as lost and reschedule it
	CpuPerExecutor float64
	MemoryPerTask  float64
	DiskPerTask    float64
}

/*https://godoc.org/github.com/garyburd/redigo/redis#Pool*/
func newRedisPool(server, password string) *redis.Pool {
	return &redis.Pool{
		MaxIdle: 3,
		IdleTimeout: 60 * time.Second,
		Dial: func () (redis.Conn, error) {
			c, err := redis.Dial("tcp", server)
			if err != nil {
				return nil, err
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

func newSentinelPool(addrs []string, master string) *redis.Pool {
	sntnl := &sentinel.Sentinel{
		Addrs:      addrs,
		MasterName: master,
		Dial: func(addr string) (redis.Conn, error) {
			timeout := 500 * time.Millisecond
			c, err := redis.DialTimeout("tcp", addr, timeout, timeout, timeout)
			if err != nil {
				return nil, err
			}
			return c, nil
		},
	}
	return &redis.Pool{
		MaxIdle:     3,
		MaxActive:   64,
		Wait:        true,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			masterAddr, err := sntnl.MasterAddr()
			if err != nil {
				return nil, err
			}
			c, err := redis.Dial("tcp", masterAddr)
			if err != nil {
				return nil, err
			}
			return c, nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if !sentinel.TestRole(c, "master") {
				return errors.New("Role check failed")
			} else {
				return nil
			}
		},
	}
}

func signalListen() {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGHUP)
	for {
		s := <-c
		logger.Println("Get signal:", s)

		configFile, err := os.Open("/etc/optimus.json")
		if err != nil {
			logger.Println("Error open config file! err", err)
			continue
		}
		jsonDecoder := json.NewDecoder(configFile)
		var cfg Config
		err = jsonDecoder.Decode(&cfg)
		if err != nil {
			logger.Println("Error parsing config file! err", err)
		}
		configFile.Close()
		CONFIG = cfg
	}
}

func requestHandler() {
	for {
		request := <-requestBuffer
		err := insertJob(&request)
		if err != nil {
			logger.Println("Error inserting request: ", request, "with error: ", err)
			continue
		}
		var targetType string
		if _, ok := cluster[request.TargetType]; ok {
			targetType = "s3"
		} else {
			targetType = "Vaas"
		}
		accessKey, secretKey := getKeysForUser(request.accessKey, targetType)
		tasks := []*common.TransferTask{}
		cursor := 0
		length := len(request.OriginUrls)
		for {
			t := common.TransferTask{
				UId:          request.accessKey,
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
		err = chkAndAddSchedUser(request.accessKey)
		if err != nil {
			logger.Println("Error checking and adding user to sched list: ", err)
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
	jsonDecoder := json.NewDecoder(configFile)
	err = jsonDecoder.Decode(&CONFIG)
	if err != nil {
		panic("Error parsing config file /etc/optimus.json with error: " + err.Error())
	}
	configFile.Close()

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
	clearRunningTask()
	initScheduledUsers()
	cluster = make(map[string]string)
	userMaxSpeed = make(map[string]int64)
	err = initS3ClusterAddr(cluster)
	if err != nil {
		panic("Error init s3 cluster address: err" + err.Error())
	}

	requestBuffer = make(chan TransferRequest, CONFIG.RequestBufferSize)
	go requestHandler()

	go startApiServer()

	go rescheduler()

	go signalListen()

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
