package main

import (
	"github.com/mesos/mesos-go/mesosproto"
	"github.com/gogo/protobuf/proto"
	"github.com/mesos/mesos-go/scheduler"
	"os"
	"log"
	"flag"
	"strconv"
	"github.com/mesos/mesos-go/mesosutil"
)

var (
	// TODO: replace global variables with config parameters
	MASTER = "127.0.0.1:5050"
	EXECUTOR_URL = "http://127.0.0.1:8000/main_6"
	EXECUTE_CMD = "./main_6"
	CPU_PER_TASK = .5
	MEM_PER_TASK = 500.
	DISK_PER_TASK = 1024.
	logger *log.Logger
)

type Scheduler struct{ // implements scheduler.Scheduler interface
	tasksLaunched int
	// TODO: more instance variables
}

func newScheduler() *Scheduler{
	return &Scheduler{}
}

func (scheduler *Scheduler) Registered(driver scheduler.SchedulerDriver,
frameworkID *mesosproto.FrameworkID, masterInfo *mesosproto.MasterInfo)  {
	logger.Println("Framework registered.")
	logger.Println("Framework ID: ", frameworkID.GetValue())
	logger.Println("Master: ", masterInfo)
}

func (scheduler *Scheduler) Reregistered(driver scheduler.SchedulerDriver,
masterInfo *mesosproto.MasterInfo)  {
	logger.Println("Framework re-registered.")
	logger.Println("Master: ", masterInfo)
}

func (scheduler *Scheduler) Disconnected(driver scheduler.SchedulerDriver)  {
	logger.Println("Disconnected from master!")
	driver.Stop(true)
	// TODO: fail-over
}

func buildUris() []*mesosproto.CommandInfo_URI {
	var uris []*mesosproto.CommandInfo_URI
	uris = append(uris, &mesosproto.CommandInfo_URI{
		Value: &EXECUTOR_URL,
		Executable: proto.Bool(true),
		Extract: proto.Bool(false),
		Cache: proto.Bool(true),
	})
	uris = append(uris, &mesosproto.CommandInfo_URI{
		Value: proto.String("http://www.le.com/index.html"),  // TODO: file to download
		Executable: proto.Bool(false),
		Extract: proto.Bool(false),
		Cache: proto.Bool(false),
	})
	return uris
}

func (scheduler *Scheduler) ResourceOffers(driver scheduler.SchedulerDriver,
offers []*mesosproto.Offer)  {
	for _, offer := range offers {
		var totalCpu, totalMemory, totalDisk float64
		for _, resource := range offer.Resources {
			switch resource.GetName() {
			case "cpus":
				totalCpu += *resource.GetScalar().Value
			case "mem":
				totalMemory += *resource.GetScalar().Value
			case "disk":
				totalDisk += *resource.GetScalar().Value
			}
		}
		logger.Println("Offered cpu: ", totalCpu, "memory: ", totalMemory,
			"disk: ", totalDisk)
		tasks := []*mesosproto.TaskInfo{}
		for totalCpu >= CPU_PER_TASK && totalMemory >= MEM_PER_TASK &&
			totalDisk >= DISK_PER_TASK {
			taskID := &mesosproto.TaskID{
				Value: proto.String(strconv.Itoa(scheduler.tasksLaunched)),
			}
			executor := &mesosproto.ExecutorInfo{
				ExecutorId: &mesosproto.ExecutorID{
					Value: proto.String("transmission-" + taskID.GetValue()),
				},
				Command: &mesosproto.CommandInfo{
					Value: proto.String(EXECUTE_CMD),
					Uris: buildUris(),
				},
				//Resources: []*mesosproto.Resource{
				//	mesosutil.NewScalarResource("cpus", CPU_PER_TASK),
				//	mesosutil.NewScalarResource("mem", MEM_PER_TASK),
				//	mesosutil.NewScalarResource("disk", DISK_PER_TASK),
				//},
			}
			task := &mesosproto.TaskInfo{
				Name: proto.String("Transmission-" + taskID.GetValue()),
				TaskId: taskID,
				SlaveId: offer.SlaveId,
				Executor: executor,
				Resources: []*mesosproto.Resource{
					mesosutil.NewScalarResource("cpus", CPU_PER_TASK),
					mesosutil.NewScalarResource("mem", MEM_PER_TASK),
					mesosutil.NewScalarResource("disk", DISK_PER_TASK),
				},
				Data: []byte("where to upload to"), // TODO
			}

			tasks = append(tasks, task)
			scheduler.tasksLaunched++
			totalCpu -= CPU_PER_TASK
			totalMemory -= MEM_PER_TASK
			totalDisk -= DISK_PER_TASK
		}
		driver.LaunchTasks([]*mesosproto.OfferID{offer.Id},
			tasks, &mesosproto.Filters{})
	}
}

func (scheduler *Scheduler) OfferRescinded(driver scheduler.SchedulerDriver,
offer *mesosproto.OfferID)  {
	logger.Println("Offer rescinded: ", offer)
}

func (scheduler *Scheduler) StatusUpdate(driver scheduler.SchedulerDriver,
taskStatus *mesosproto.TaskStatus)  {
	logger.Println("Status update: task", taskStatus.TaskId.GetValue(),
		" is in state ", taskStatus.State.Enum().String())
}

func (scheduler *Scheduler) FrameworkMessage(driver scheduler.SchedulerDriver,
executorID *mesosproto.ExecutorID, slaveID *mesosproto.SlaveID, message string)  {
	logger.Printf("Framework message from executor %q slave %q: %q\n",
		executorID, slaveID, message)
}

func (scheduler *Scheduler) SlaveLost(driver scheduler.SchedulerDriver,
slaveID *mesosproto.SlaveID)  {
	logger.Printf("Slave lost: %v", slaveID)
}

func (scheduler *Scheduler) ExecutorLost(driver scheduler.SchedulerDriver,
executorID *mesosproto.ExecutorID, slaveID *mesosproto.SlaveID, code int)  {
	logger.Printf("Executor %q lost on slave %q code %d",
		executorID, slaveID, code)
}

func (scheduler *Scheduler) Error(driver scheduler.SchedulerDriver, error string)  {
	logger.Println("Unrecoverable error: ", error)
	driver.Stop(false)
}


func init()  {
	flag.Parse() // mesos-go uses golang/glog, which requires parse flags first
}

func main()  {
	// TODO: log to a file
	logger = log.New(os.Stdout, "Optimus Prime: ", log.LstdFlags|log.Lshortfile)

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