package main

import (
	"encoding/json"
	"github.com/gogo/protobuf/proto"
	"github.com/mesos/mesos-go/mesosproto"
	"github.com/mesos/mesos-go/mesosutil"
	"github.com/mesos/mesos-go/scheduler"
	"math"
	"strconv"

	"git.letv.cn/optimus/optimus/common"
)

type Scheduler struct { // implements scheduler.Scheduler interface
	// TODO: more instance variables
}

func newScheduler() *Scheduler {
	return &Scheduler{}
}

func (scheduler *Scheduler) Registered(driver scheduler.SchedulerDriver,
	frameworkID *mesosproto.FrameworkID, masterInfo *mesosproto.MasterInfo) {
	logger.Println("Framework registered.")
	logger.Println("Framework ID: ", frameworkID.GetValue())
	logger.Println("Master: ", masterInfo)
}

func (scheduler *Scheduler) Reregistered(driver scheduler.SchedulerDriver,
	masterInfo *mesosproto.MasterInfo) {
	logger.Println("Framework re-registered.")
	logger.Println("Master: ", masterInfo)
}

func (scheduler *Scheduler) Disconnected(driver scheduler.SchedulerDriver) {
	logger.Println("Disconnected from master!")
	driver.Stop(true)
	// TODO: fail-over
}

func buildUris() []*mesosproto.CommandInfo_URI {
	var uris []*mesosproto.CommandInfo_URI
	uris = append(uris, &mesosproto.CommandInfo_URI{
		Value:      &CONFIG.ExecutorUrl,
		Executable: proto.Bool(true),
		Extract:    proto.Bool(false),
		Cache:      proto.Bool(true),
	})
	return uris
}

type Slave struct {
	uuid     string
	hostname string
	status   string // in Active/Lost
}

type Executor struct {
	id          string // uuid
	taskRunning int
	status      string // in Scheduled/Lost
}

// calculate how many executors and tasks could be launched for certain resources
func calculateCapacity(cpu float64, memory float64, disk float64) (executor int, task int) {
	executor = int(cpu / CONFIG.CpuPerExecutor)
	task = int(math.Min(memory/CONFIG.MemoryPerTask, disk/CONFIG.DiskPerTask))
	return
}

func newTask(task *common.TransferTask, executorId string,
	slaveId *mesosproto.SlaveID) (mesosTask *mesosproto.TaskInfo) {
	taskID := &mesosproto.TaskID{
		Value: proto.String(strconv.FormatInt(task.Id, 10)),
	}
	addrs, err := json.Marshal(CONFIG.RedisAddress)
	if err != nil {
		logger.Println("Error marshal json: ", err)
	}
	executor := &mesosproto.ExecutorInfo{
		ExecutorId: &mesosproto.ExecutorID{
			Value: proto.String(executorId),
		},
		Command: &mesosproto.CommandInfo{
			Shell: proto.Bool(false),
			Value: proto.String(CONFIG.ExecuteCommand),
			Uris:  buildUris(),
			Arguments:  []string{"--redis-master-name", CONFIG.RedisMasterName,"--redis-addr", string(addrs)},
		},
		Resources: []*mesosproto.Resource{
			mesosutil.NewScalarResource("cpus", CONFIG.CpuPerExecutor),
		},
	}
	jsonData, err := json.Marshal(task)
	if err != nil {
		logger.Println("Error marshal json: ", err)
	}
	mesosTask = &mesosproto.TaskInfo{
		Name:     proto.String("Transfer-" + strconv.FormatInt(task.Id, 10)),
		TaskId:   taskID,
		SlaveId:  slaveId,
		Executor: executor,
		Resources: []*mesosproto.Resource{
			mesosutil.NewScalarResource("mem", CONFIG.MemoryPerTask),
			mesosutil.NewScalarResource("disk", CONFIG.DiskPerTask),
		},
		Data: jsonData,
	}
	return
}

func newTaskForExecutor(task *common.TransferTask, executor *Executor,
	slaveId *mesosproto.SlaveID) *mesosproto.TaskInfo {
	return newTask(task, executor.id, slaveId)
}

func newTaskAndExecutor(task *common.TransferTask,
	slaveId *mesosproto.SlaveID) *mesosproto.TaskInfo {
	return newTask(task, newUuid(), slaveId)
}

func (scheduler *Scheduler) ResourceOffers(driver scheduler.SchedulerDriver,
	offers []*mesosproto.Offer) {
	for _, offer := range offers {
		err := upsertSlave(&Slave{
			uuid:     offer.SlaveId.GetValue(),
			hostname: *offer.Hostname,
			status:   "Active",
		})
		if err != nil {
			logger.Println("Error upsert slave: ", err)
			driver.DeclineOffer(offer.Id, &mesosproto.Filters{})
			continue
		}
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
		executorCapacity, taskCapacity := calculateCapacity(totalCpu, totalMemory, totalDisk)

		tx, err := db.Begin()
		if err != nil {
			logger.Println("Failed to begin transaction: ", err)
			driver.DeclineOffer(offer.Id, &mesosproto.Filters{})
			continue
		}
		idleExecutors := getIdleExecutorsOnSlave(tx, offer.SlaveId.GetValue())
		slaveCapacity := Min(executorCapacity+len(idleExecutors), taskCapacity)
		pendingTasks := getNextUserPendingTasks(tx, slaveCapacity)

		executorCursor := 0
		tasks := []*mesosproto.TaskInfo{}
		for _, pendingTask := range pendingTasks {
			if executorCursor < len(idleExecutors) {
				task := newTaskForExecutor(pendingTask, idleExecutors[executorCursor],
					offer.SlaveId)
				tasks = append(tasks, task)
				executorCursor++
				continue
			}
			task := newTaskAndExecutor(pendingTask, offer.SlaveId)
			tasks = append(tasks, task)
		}
		driver.LaunchTasks([]*mesosproto.OfferID{offer.Id},
			tasks, &mesosproto.Filters{})

		if len(tasks) == 0 {
			tx.Rollback()
			continue
		}
		initializeTaskStatus(tx, tasks, offer.SlaveId.GetValue())
		// TODO: reschedule Failed tasks
	}
}

func (scheduler *Scheduler) OfferRescinded(driver scheduler.SchedulerDriver,
	offer *mesosproto.OfferID) {
	logger.Println("Offer rescinded: ", offer)
	// TODO: track tasks
}

func (scheduler *Scheduler) StatusUpdate(driver scheduler.SchedulerDriver,
	taskStatus *mesosproto.TaskStatus) {
	switch *taskStatus.State {
	case mesosproto.TaskState_TASK_RUNNING:
		updateTask(taskStatus.TaskId.GetValue(), taskStatus.ExecutorId.GetValue(), "Running")
	case mesosproto.TaskState_TASK_ERROR, mesosproto.TaskState_TASK_FAILED:
		updateTask(taskStatus.TaskId.GetValue(), taskStatus.ExecutorId.GetValue(), "Failed")
		tryFinishJob(taskStatus.TaskId.GetValue())
	case mesosproto.TaskState_TASK_LOST:
		taskLostUpdate(taskStatus.TaskId.GetValue(), taskStatus.ExecutorId.GetValue())
	case mesosproto.TaskState_TASK_FINISHED:
		updateTask(taskStatus.TaskId.GetValue(), taskStatus.ExecutorId.GetValue(), "Finished")
		tryFinishJob(taskStatus.TaskId.GetValue())
	default:
		logger.Println("Status update: task", taskStatus.TaskId.GetValue(),
			" is in state ", taskStatus.State.Enum().String())
	}
}

func (scheduler *Scheduler) FrameworkMessage(driver scheduler.SchedulerDriver,
	executorID *mesosproto.ExecutorID, slaveID *mesosproto.SlaveID, message string) {
	var urlUpdate common.UrlUpdate
	err := json.Unmarshal([]byte(message), &urlUpdate)
	if err != nil {
		logger.Println("Malformed framework message: ", message, "with error: ", err)
		return
	}
	updateUrl(&urlUpdate)
}

func (scheduler *Scheduler) SlaveLost(driver scheduler.SchedulerDriver,
	slaveID *mesosproto.SlaveID) {
	logger.Printf("Slave lost: %v", slaveID)
	slaveLostUpdate(slaveID.GetValue())
}

func (scheduler *Scheduler) ExecutorLost(driver scheduler.SchedulerDriver,
	executorID *mesosproto.ExecutorID, slaveID *mesosproto.SlaveID, code int) {
	logger.Printf("Executor %q lost on slave %q code %d",
		executorID, slaveID, code)
	executorLostUpdate(executorID.GetValue())
}

func (scheduler *Scheduler) Error(driver scheduler.SchedulerDriver, error string) {
	logger.Println("Unrecoverable error: ", error)
	driver.Stop(false)
}
