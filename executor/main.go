package main

import (
	"flag"
	"fmt"

	exec "github.com/mesos/mesos-go/executor"
	mesos "github.com/mesos/mesos-go/mesosproto"

	"encoding/json"
	"git.letv.cn/optimus/optimus/common"
	"git.letv.cn/optimus/optimus/executor/s3"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type ReaderAtSeeker interface {
	io.ReaderAt
	io.ReadSeeker
}

var (
	MAX_RETRY_TIMES = 3
	S3_ENDPOINT     = "http://s3s.lecloud.com"
	CHUNK_SIZE      = 5 << 20 // 5 MB

	results = make(chan *FileTask)
)

type FileTask struct {
	name         string
	originUrl    string
	targetUrl    string
	targetType   string
	targetBucket string
	targetAcl    string
	status       string // in Finished/Failed
	retriedTimes int
	accessKey    string
	secretKey    string
}

func s3SimpleUpload(file io.Reader, task *FileTask, contentType string) (targetUrl string, err error) {
	d := s3.NewDriver(task.accessKey, task.secretKey, S3_ENDPOINT, task.targetBucket, contentType)
	uploader, err := d.NewSimpleMultiPartWriter(task.name, CHUNK_SIZE, task.targetAcl)
	if err != nil {
		return
	}
	defer uploader.Close()

	n, err := io.Copy(uploader, file)
	if err != nil {
		return
	}
	fmt.Println("File", task.name, "uploaded with", n, "bytes")
	targetUrl = S3_ENDPOINT + "/" + task.targetBucket + task.name // task.name has a prefix "/"
	return
}

func s3Upload(file ReaderAtSeeker, task *FileTask, contentType string) (targetUrl string, err error) {
	d := s3.NewDriver(task.accessKey, task.secretKey, S3_ENDPOINT, task.targetBucket, contentType)
	uploader, err := d.NewMultiPartWriter(task.name, CHUNK_SIZE, task.targetAcl)
	if err != nil {
		fmt.Println("NewMultiPartWriter failed!")
		return
	}

	parts, err := uploader.PutAll(file, int64(CHUNK_SIZE))
	if err != nil {
		return
	}

	n, err := uploader.Complete(parts)
	if err != nil {
		return
	}

	fmt.Println("File", task.name, "uploaded with", n, "bytes")
	targetUrl = S3_ENDPOINT + "/" + task.targetBucket + task.name // task.name has a prefix "/"
	return
}

func fileDownload(fileDl *FileDl) (size int64, err error) {
	var finish = make(chan bool)
	fileDl.OnFinish(func() {
		finish <- true
	})

	var dlErr error
	fileDl.OnError(func(errCode int, err error) {
		dlErr = err
		fmt.Println("Error downloading: errCode:", errCode, "err:", err)
	})

	var exit bool
	var dlSize int64
	fileDl.Start()
	for !exit {
		dlSize = fileDl.GetDownloadedSize()

		select {
		case exit = <-finish:
			fmt.Println("downloaded size", dlSize)
		default:
			time.Sleep(time.Second * 1)
		}
	}

	return dlSize, dlErr
}

func transfer(task *FileTask) {
	var err error
	filename := strings.Replace(strings.Replace(task.originUrl, "/", "", -1),
		":", "", -1) // escape "/" and ":" in url so it could be used as filename
	file, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error creating file: ", task.name)
		task.status = "Failed"
		results <- task
		return
	}
	defer os.Remove(filename)
	defer file.Close()

	fileDl, err := NewFileDl(task.originUrl, file)
	if err != nil {
		fmt.Println("Cannot new file downloader!", "with error", err)
		task.status = "Failed"
		results <- task
		return
	}
	contentType := fileDl.GetContentType()
	n, err := fileDownload(fileDl)
	if err != nil {
		fmt.Println("Error downloading file: ", task.name, "with error", err)
		task.status = "Failed"
		results <- task
		return
	}
	fmt.Println("File", task.name, "downloaded with", n, "bytes")
	file.Seek(0, 0)
	var targetUrl string
	switch task.targetType {
	case "s3s":
		targetUrl, err = s3Upload(file, task, contentType)
	case "Vaas":
		fmt.Println("Vaas upload has not been implemented")
		task.status = "Failed"
		results <- task
		return
	default:
		fmt.Println("Unknown target type")
		task.status = "Failed"
		results <- task
		return
	}
	if err != nil {
		fmt.Println("Uploading error: ", err)
		task.status = "Failed"
		results <- task
		return
	}

	task.status = "Finished"
	task.targetUrl = targetUrl
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

func updateTaskStatus(driver exec.ExecutorDriver, taskId *mesos.TaskID, status mesos.TaskState) {
	runStatus := &mesos.TaskStatus{
		TaskId: taskId,
		State:  status.Enum(),
	}
	_, err := driver.SendStatusUpdate(runStatus)
	if err != nil {
		fmt.Println("Error sending status update: ", err)
	}
}

func updateFileStatus(driver exec.ExecutorDriver, taskId string, fileTask *FileTask) {
	id, err := strconv.ParseInt(taskId, 10, 64)
	if err != nil {
		fmt.Println("Error converting taskId to int64: ", err)
		return
	}
	update := common.UrlUpdate{
		OriginUrl: fileTask.originUrl,
		TargetUrl: fileTask.targetUrl,
		TaskId:    id,
		Status:    fileTask.status,
	}
	jsonUpdate, err := json.Marshal(update)
	if err != nil {
		fmt.Println("Error marshal json: ", err)
		return
	}
	driver.SendFrameworkMessage(string(jsonUpdate))
}

func (exec *megatronExecutor) LaunchTask(driver exec.ExecutorDriver, taskInfo *mesos.TaskInfo) {
	fmt.Println("Launching task", taskInfo.GetName(), "with command", taskInfo.Command.GetValue())
	updateTaskStatus(driver, taskInfo.GetTaskId(), mesos.TaskState_TASK_RUNNING)

	exec.tasksLaunched++
	fmt.Println("Total tasks launched ", exec.tasksLaunched)

	var task common.TransferTask
	err := json.Unmarshal(taskInfo.GetData(), &task)
	if err != nil {
		fmt.Println("Malformed task info:", err)
		updateTaskStatus(driver, taskInfo.GetTaskId(), mesos.TaskState_TASK_ERROR)
		return
	}
	fmt.Println("Task info data: ", task)

	for _, sourceUrl := range task.OriginUrls {
		urlParsed, err := url.Parse(sourceUrl)
		if err != nil {
			fmt.Println("Bad URL: ", sourceUrl)
			updateTaskStatus(driver, taskInfo.GetTaskId(), mesos.TaskState_TASK_ERROR)
			return
		}
		t := &FileTask{
			name:         urlParsed.Path,
			originUrl:    sourceUrl,
			targetType:   task.TargetType,
			targetBucket: task.TargetBucket,
			targetAcl:    task.TargetAcl,
			retriedTimes: 0,
			accessKey:    task.AccessKey,
			secretKey:    task.SecretKey,
		}
		go transfer(t)
	}
	finished := 0
	failed := 0
FOR:
	for {
		result := <-results
		switch result.status {
		case "Finished":
			finished++
			updateFileStatus(driver, taskInfo.TaskId.GetValue(), result)
			if finished+failed == len(task.OriginUrls) {
				break FOR
			}
		case "Failed":
			if result.retriedTimes < MAX_RETRY_TIMES {
				result.retriedTimes++
				go transfer(result)
			} else {
				failed++
				fmt.Println("URL failed for ", result.originUrl, "after retries")
				updateFileStatus(driver, taskInfo.TaskId.GetValue(), result)
			}
			if finished+failed == len(task.OriginUrls) {
				break FOR
			}
		default:
			fmt.Println("Should NEVER hit here")
		}
	}
	if failed == 0 {
		updateTaskStatus(driver, taskInfo.GetTaskId(), mesos.TaskState_TASK_FINISHED)
		fmt.Println("Task finished", taskInfo.GetName())
	} else {
		updateTaskStatus(driver, taskInfo.GetTaskId(), mesos.TaskState_TASK_FAILED)
		fmt.Println("Task (maybe partially) failed", taskInfo.GetName())
	}
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
