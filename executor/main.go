package main

import (
	"flag"
	"fmt"

	exec "github.com/mesos/mesos-go/executor"
	mesos "github.com/mesos/mesos-go/mesosproto"

	"git.letv.cn/zhangcan/optimus/common"
	"encoding/json"
	"net/url"
	"strings"
	"os"
	"net/http"
	"io"
)


var (
	MAX_RETRY_TIMES = 3

	results = make(chan *FileTask)
)

type FileTask struct  {
	name string
	sourceUrl string
	destinationType string
	destinationUrl string
	status string // in Finished/Failed
	retriedTimes int
}

func transfer(task *FileTask)  {
	file, err := os.Create(task.name)
	if err != nil {
		fmt.Println("Error creating file: ", task.name)
		task.status = "Failed"
		results <- task
		return
	}
	defer file.Close()

	response, err := http.Get(task.sourceUrl)
	if err != nil {
		fmt.Println("Error downloading file: ", task.name)
		task.status = "Failed"
		results <- task
		return
	}
	defer response.Body.Close()

	n, err := io.Copy(file, response.Body)
	if err != nil {
		fmt.Println("Error downloading file: ", task.name)
		task.status = "Failed"
		results <- task
		return
	}
	fmt.Println(n, "bytes downloaded")
	task.status = "Finished"
	results <- task
}

type megatronExecutor struct {
	tasksLaunched int
}

func newExampleExecutor() *megatronExecutor {
	return &megatronExecutor{tasksLaunched: 0}
}

func (exec *megatronExecutor) Registered(driver exec.ExecutorDriver,
execInfo *mesos.ExecutorInfo, fwinfo *mesos.FrameworkInfo, slaveInfo *mesos.SlaveInfo) {
	fmt.Println("Registered Executor on slave ", slaveInfo.GetHostname())
}

func (exec *megatronExecutor) Reregistered(driver exec.ExecutorDriver, slaveInfo *mesos.SlaveInfo) {
	fmt.Println("Re-registered Executor on slave ", slaveInfo.GetHostname())
}

func (exec *megatronExecutor) Disconnected(exec.ExecutorDriver) {
	fmt.Println("Executor disconnected.")
}

func updateStatus(driver exec.ExecutorDriver, taskId *mesos.TaskID, status mesos.TaskState)  {
	runStatus := &mesos.TaskStatus{
		TaskId: taskId,
		State:  status.Enum(),
	}
	_, err := driver.SendStatusUpdate(runStatus)
	if err != nil {
		fmt.Println("Error sending status update: ", err)
	}
}

func (exec *megatronExecutor) LaunchTask(driver exec.ExecutorDriver, taskInfo *mesos.TaskInfo) {
	fmt.Println("Launching task", taskInfo.GetName(), "with command", taskInfo.Command.GetValue())
	updateStatus(driver, taskInfo.GetTaskId(), mesos.TaskState_TASK_RUNNING)

	exec.tasksLaunched++
	fmt.Println("Total tasks launched ", exec.tasksLaunched)

	var task common.TransferTask
	err := json.Unmarshal(taskInfo.GetData(), &task)
	if err != nil {
		fmt.Println("Malformed task info")
		updateStatus(driver, taskInfo.GetTaskId(), mesos.TaskState_TASK_ERROR)
		return
	}
	fmt.Println("Task info data: ", task)

	for _, sourceUrl := range task.SourceUrls {
		urlParsed, err := url.Parse(sourceUrl)
		if err != nil {
			fmt.Println("Bad URL: ", sourceUrl)
			updateStatus(driver, taskInfo.GetTaskId(), mesos.TaskState_TASK_ERROR)
			return
		}
		segments := strings.Split(urlParsed.Path, "/")
		fileName := segments[len(segments) - 1]
		t := &FileTask{
			name: fileName,
			sourceUrl: sourceUrl,
			destinationType: task.DestinationType,
			destinationUrl: task.DestinationBaseUrl,
			retriedTimes: 0,
		}
		go transfer(t)
	}
	finished := 0
	FOR:
	for {
		result := <- results
		switch result.status {
		case "Finished":
			finished++
			if finished == len(task.SourceUrls) {
				break FOR
			}
		case "Failed":
			if result.retriedTimes < MAX_RETRY_TIMES {
				result.retriedTimes++
				go transfer(result)
			} else {
				fmt.Println("URL failed for ", result.sourceUrl, "after retries")
				updateStatus(driver, taskInfo.GetTaskId(), mesos.TaskState_TASK_FAILED)
				return
			}
		default:
			fmt.Println("Should NEVER hit here")
		}
	}

	updateStatus(driver, taskInfo.GetTaskId(), mesos.TaskState_TASK_FINISHED)
	fmt.Println("Task finished", taskInfo.GetName())
}

func (exec *megatronExecutor) KillTask(driver exec.ExecutorDriver, taskId *mesos.TaskID) {
	fmt.Println("Kill task")
	driver.Stop()
}

func (exec *megatronExecutor) FrameworkMessage(driver exec.ExecutorDriver, msg string) {
	fmt.Println("Got framework message: ", msg)
}

func (exec *megatronExecutor) Shutdown(driver exec.ExecutorDriver) {
	fmt.Println("Shutting down the executor")
	status, err := driver.Stop()
	fmt.Println("Stop status ", status, "err ", err)
}

func (exec *megatronExecutor) Error(driver exec.ExecutorDriver, err string) {
	fmt.Println("Got error message:", err)
}


func init() {
	flag.Parse() // mesos-go uses golang/glog, which requires to parse flags first
}

func main() {
	fmt.Println("Starting Megatron...")

	config := exec.DriverConfig{
		Executor: newExampleExecutor(),
	}
	driver, err := exec.NewMesosExecutorDriver(config)

	if err != nil {
		fmt.Println("Unable to create a ExecutorDriver ", err.Error())
	}

	_, err = driver.Start()
	if err != nil {
		fmt.Println("Failed to start:", err)
		return
	}
	fmt.Println("Megatron has started and running")

	_, err = driver.Join()
	if err != nil {
		fmt.Println("Driver failed:", err)
	}
	fmt.Println("Executor terminated")
}
