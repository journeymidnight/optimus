package main

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/mesos/mesos-go/mesosproto"
	"strconv"

	"git.letv.cn/optimus/optimus/common"
	"time"
	"encoding/json"
)

func createDbConnection() *sql.DB {
	conn, err := sql.Open("mysql", CONFIG.DatabaseConnectionString)
	if err != nil {
		panic(fmt.Sprintf("Error connecting to database: %v", err))
	}
	logger.Println("Connected to database")
	return conn
}

func insertJob(req *TransferRequest) (err error) {
	_, err = db.Exec("insert job set id = 0, uuid = ?, create_time = NOW(), "+
		"callback_url = ?, callback_token = ?, access_key = ?, status = ?",
		req.uuid, req.callbackUrl, req.callbackToken, req.accessKey, "Pending")
	return err
}

func insertTasks(tasks []*common.TransferTask) error {
	for _, task := range tasks {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		result, err := tx.Exec(
			"insert into task(id, uid, job_uuid, target_type, target_bucket, target_acl, status, access_key, secret_key) "+
				"values(?, ?, ?, ?, ?, ?, ?, ?, ?)",
			0, task.UId, task.JobUuid, task.TargetType, task.TargetBucket, task.TargetAcl, task.Status, task.AccessKey, task.SecretKey)
		if err != nil {
			tx.Rollback()
			return err
		}
		taskId, err := result.LastInsertId()
		if err != nil {
			tx.Rollback()
			return err
		}
		for _, url := range task.OriginUrls {
			_, err := tx.Exec("insert into url(id, task_id, origin_url, status) "+
				"values(?, ?, ?, ?)", 0, taskId, url, task.Status)
			if err != nil {
				tx.Rollback()
				return err
			}
		}
		tx.Commit()
	}
	return nil
}

func upsertSlave(slave *Slave) error {
	_, err := db.Exec("insert into slave(id, uuid, hostname, status) "+
		"values(?, ?, ?, ?) on duplicate key update "+
		"status = values(status),"+
		"hostname = values(hostname)", 0, slave.uuid, slave.hostname, slave.status)
	return err
}

func getIdleExecutorsOnSlave(tx *sql.Tx, slaveUuid string) (executors []*Executor) {
	rows, err := tx.Query("select uuid, task_running from executor where "+
		"slave_uuid = ? and task_running < ? and status != ? for update",
		slaveUuid, CONFIG.ExecutorIdleThreshold, "Lost")
	if err != nil {
		logger.Println("Error querying idle executors: ", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var executor Executor
		if err := rows.Scan(&executor.id, &executor.taskRunning); err != nil {
			logger.Println("Row scan error: ", err)
			continue
		}
		executors = append(executors, &executor)
	}
	if err := rows.Err(); err != nil {
		logger.Println("Row error: ", err)
	}
	return
}

func getNextUserPendingTasks(tx *sql.Tx, limit int) (tasks []*common.TransferTask) {
	for {
		ak, err := getNextUser()
		if err != nil {
			logger.Println("Error get next user: ", err)
		}
		if ak == "" {
			break
		}
		tasks = getPendingTasks(ak, tx, limit)
		if len(tasks) == 0 {
			removeSchedUser(ak)
		} else {
			return
		}
	}
	return
}

func getPendingTasks(uid string, tx *sql.Tx, limit int) (tasks []*common.TransferTask) {
	taskRows, err := tx.Query(
		"select id, job_uuid, target_type, target_bucket, target_acl, access_key, secret_key from task "+
			"where uid = ? and status = ? limit ? for update", uid, "Pending", limit)
	if err != nil {
		logger.Println("Error querying pending tasks: ", err)
		return
	}
	defer taskRows.Close()
	for taskRows.Next() {
		var task common.TransferTask
		var targetType string
		if err := taskRows.Scan(&task.Id, &task.JobUuid, &targetType, &task.TargetBucket,
			&task.TargetAcl, &task.AccessKey, &task.SecretKey); err != nil {
			logger.Println("Row scan error: ", err)
			continue
		}
		task.Status = "Pending"
		if addr, ok := cluster[targetType]; ok {
			task.TargetType = "s3"
			task.TargetCluster = addr
		} else {
			logger.Println("Target type is wrong. target: ", targetType)
			continue
		}
		tasks = append(tasks, &task)
	}
	for _, task := range tasks {
		urlRows, err := tx.Query("select origin_url from url where task_id = ?", task.Id)
		if err != nil {
			logger.Println("Error querying urls: ", err)
			continue
		}
		for urlRows.Next() {
			var url string
			if err := urlRows.Scan(&url); err != nil {
				logger.Println("Row scan error: ", err)
				break
			}
			task.OriginUrls = append(task.OriginUrls, url)
		}
		urlRows.Close()
	}
	return
}

func initializeTaskStatus(tx *sql.Tx, tasks []*mesosproto.TaskInfo, slaveUuid string) {
	for _, task := range tasks {
		taskId := task.TaskId.GetValue()
		executorUuid := task.Executor.ExecutorId.GetValue()
		_, err := tx.Exec("insert into executor(id, slave_uuid, uuid, task_running, status) "+
			"values(?, ?, ?, ?, ?) on duplicate key update "+
			"task_running = task_running + 1", 0, slaveUuid, executorUuid, 1, "Scheduled")
		if err != nil {
			logger.Println("Error upsert executor: ", err)
			continue
		}
		_, err = tx.Exec("update task set executor_uuid = ?, status = ?, schedule_time = NOW() where "+
			"id = ?", executorUuid, "Scheduled", taskId)
		if err != nil {
			logger.Println("Error update task: ", err)
		}
		var transferTask common.TransferTask
		err = json.Unmarshal(task.Data, &transferTask)
		if err != nil {
			fmt.Println("Malformed task info:", err)
			continue
		}
		_, err = tx.Exec("update job set status = ? where "+
		    "uuid = ? and status = ?", "Scheduled", transferTask.JobUuid, "Pending")
		if err != nil {
			logger.Println("Error update job: ", err)
		}
	}
	tx.Commit()
}

func updateUrl(update *common.UrlUpdate) {
	_, err := db.Exec("update url set status = ?, target_url = ?, size = ? where "+
		"task_id = ? and origin_url = ?",
		update.Status, update.TargetUrl, update.Size, update.TaskId, update.OriginUrl)
	if err != nil {
		logger.Println("Error updating url: ", err)
	}
}

func updateTask(taskId string, executorUuid string, status string) {
	taskIdInt, _ := strconv.ParseInt(taskId, 10, 64)
	_, err := db.Exec("update task set status = ? where id = ?", status, taskIdInt)
	if err != nil {
		logger.Println("Error updating status for task ", taskId, "with error ", err)
	}
	if status != "Failed" && status != "Finished" {
		return
	}
	// also update status of pending files for "Failed" and "Finished"
	_, err = db.Exec("update url set status = ? where "+
		"task_id = ? and status = ?", status, taskId, "Pending")
	if err != nil {
		logger.Println("Error updating url status for task ", taskId, "with error ", err)
	}
	// Note here we assume the status is transforming from "Scheduled" to others
	_, err = db.Exec("update executor set task_running = task_running - 1 where "+
		"uuid = ?", executorUuid)
	if err != nil {
		logger.Println("Error updating task running count: ", err)
	}
}

func getJobSummary(jobUuid string) (summary JobResult, err error) {
	summary.JobUuid = jobUuid
	rows, err := db.Query("select u.origin_url, u.status from url u "+
		"join task t on u.task_id = t.id "+
		"join job j on t.job_uuid = j.uuid "+
		"where j.uuid = ?", jobUuid)
	if err != nil {
		logger.Println("Error querying job url status: ", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var url, status string
		if err := rows.Scan(&url, &status); err != nil {
			logger.Println("Row scan error: ", err)
			continue
		}
		switch status {
		case "Finished":
			summary.SuccessUrls = append(summary.SuccessUrls, url)
		case "Failed":
			summary.FailedUrls = append(summary.FailedUrls, url)
		case "Pending":
			summary.PendingUrls = append(summary.PendingUrls, url)
		}
	}
	return summary, nil
}

func tryFinishJob(taskId string) {
	var jobUuid string
	err := db.QueryRow("select job_uuid from task where id = ?", taskId).Scan(&jobUuid)
	if err != nil {
		logger.Println("Error querying job uuid: ", err)
		return
	}
	var failed, finished, total int
	err = db.QueryRow("select count(*) from task where "+
		"job_uuid = ? and status = ?", jobUuid, "Finished").Scan(&finished)
	if err != nil {
		logger.Println("Error querying finished task number: ", err)
		return
	}
	err = db.QueryRow("select count(*) from task where "+
		"job_uuid = ? and status = ?", jobUuid, "Failed").Scan(&failed)
	if err != nil {
		logger.Println("Error querying failed task number: ", err)
		return
	}
	err = db.QueryRow("select count(*) from task where "+
		"job_uuid = ?", jobUuid).Scan(&total)
	if err != nil {
		logger.Println("Error querying total task number: ", err)
		return
	}
	if failed+finished == total {
		var finishedSize int64
		if finished > 0 {
			err = db.QueryRow("select sum(u.size) from url u "+
			    "join task t on u.task_id = t.id "+
			    "join job j on t.job_uuid = j.uuid "+
			    "where j.uuid = ? and u.status = ?", jobUuid, "Finished").Scan(&finishedSize)
			if err != nil {
				logger.Println("Error get total finished size: ", err)
				return
			}
		}

		if failed > 0 {
			_, err = db.Exec("update job set status = ? where "+
				"uuid = ?", "Failed", jobUuid)
		} else {
			_, err = db.Exec("update job set complete_time = NOW(), status = ?, finished_size = ? where "+
				"uuid = ?", "Finished", finishedSize, jobUuid)
		}
		if err != nil {
			logger.Println("Error updating job status: ", err)
		}
		var callbackUrl, callbackToken sql.NullString
		err := db.QueryRow("select callback_token, callback_url from job where "+
			"uuid = ?", jobUuid).Scan(&callbackToken, &callbackUrl)
		if err != nil {
			logger.Println("Error querying callback info: ", err)
			return
		}
		if !callbackUrl.Valid || callbackUrl.String == "" {
			return
		}
		url := callbackUrl.String
		if callbackToken.Valid && callbackToken.String != "" {
			url += "?" + callbackToken.String
		}
		summary, err := getJobSummary(jobUuid)
		if err != nil {
			logger.Println("Error getting job summary for job", jobUuid, "with error", err)
			return
		}
		putJobCallback(url, &summary)
	}
}

func taskLostUpdate(taskId string, executorUuid string) {
	taskIdInt, _ := strconv.ParseInt(taskId, 10, 64)
	_, err := db.Exec("update task set status = ?, executor_uuid = NULL, schedule_time = NULL where "+
		"id = ?", "Pending", taskIdInt)
	if err != nil {
		logger.Println("Error updating task status for id", taskIdInt, "with error ", err)
	}
}

func executorLostUpdate(executorUuid string) {
	_, err := db.Exec("update executor set status = ? where uuid = ?",
		"Lost", executorUuid)
	if err != nil {
		logger.Println("Error removing executor ", executorUuid, "with error ", err)
	}
	_, err = db.Exec("update task set status = ?, executor_uuid = NULL, schedule_time = NULL where "+
		"executor_uuid = ? and status = ?", "Pending", executorUuid, "Running")
	if err != nil {
		logger.Println("Error updating task status for ", executorUuid, "with error ", err)
	}
}

func slaveLostUpdate(slaveUuid string) {
	var executorUuids []string
	rows, err := db.Query("select uuid from executor where "+
		"slave_uuid = ?", slaveUuid)
	if err != nil {
		logger.Println("Error querying executors for slave: ", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var executorUuid string
		if err := rows.Scan(&executorUuid); err != nil {
			logger.Println("Row scan error: ", err)
			continue
		}
		executorUuids = append(executorUuids, executorUuid)
	}
	_, err = db.Exec("update slave set status = ? where "+
		"uuid = ?", "Lost", slaveUuid)
	if err != nil {
		logger.Println("Error updating slave status for ", slaveUuid, " with error ", err)
	}
	for _, uuid := range executorUuids {
		executorLostUpdate(uuid)
	}
}

func getSecretKey(accessKey string) (secretKey string, err error) {
	err = db.QueryRow("select secret_key from user where "+
		"access_key = ?", accessKey).Scan(&secretKey)
	return
}

func getKeysForUser(userAccessKey string, requestType string) (string, string) {
	var accessKeyColumnName, secretKeyColumnName string
	switch requestType {
	case "s3":
		accessKeyColumnName = "s3_ak"
		secretKeyColumnName = "s3_sk"
	case "Vaas":
		accessKeyColumnName = "vaas_ak"
		secretKeyColumnName = "vaas_sk"
	}
	var ak, sk sql.NullString
	err := db.QueryRow("select "+accessKeyColumnName+","+secretKeyColumnName+
		" from user where access_key = ?", userAccessKey).Scan(&ak, &sk)
	if err != nil {
		logger.Println("Error querying AK/SK for user", userAccessKey)
		return "", ""
	}
	return ak.String, sk.String
}

func userOwnsJob(accessKey string, jobUuid string) bool {
	var count int
	err := db.QueryRow("select count(*) from job where "+
		"uuid = ? and access_key = ?", jobUuid, accessKey).Scan(&count)
	if err != nil {
		logger.Println("Error querying job: ", err)
		return false
	}
	return count != 0
}

func getScheduledTasks() (tasks []*scheduledTask) {
	rows, err := db.Query("select id, schedule_time from task where "+
		"status = ?", "Scheduled")
	if err != nil {
		logger.Println("Error querying scheduled tasks:", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var task scheduledTask
		var rawTime []byte
		if err := rows.Scan(&task.id, &rawTime); err != nil {
			logger.Println("Row scan error:", err)
			continue
		}
		local, err := time.LoadLocation("Local")
		if err != nil {
			logger.Println("Error loading current location:", err)
			continue
		}
		date, err := time.ParseInLocation("2006-01-02 15:04:05", string(rawTime), local)
		if err != nil {
			logger.Println("Error parsing date string from DB: ", string(rawTime))
			continue
		}
		task.scheduleTime = date
		tasks = append(tasks, &task)
	}
	return
}

func rescheduleTasks(tasks []*scheduledTask) {
	for _, task := range tasks {
		var executorUuid string
		err := db.QueryRow("select id from task where "+
			"id = ?", task.id).Scan(&executorUuid)
		if err != nil {
			logger.Println("Error querying executor UUID for task", task.id,
				"with error", err)
			continue
		}
		_, err = db.Exec("update executor set task_running = task_running - 1 where "+
			"uuid = ?", executorUuid)
		if err != nil {
			logger.Println("Error updating executor ", executorUuid,
				"with error", err)
		}
		_, err = db.Exec("update task set status = ?, executor_uuid = ? where "+
			"id = ?", "Pending", nil, task.id)
		if err != nil {
			logger.Println("Error rescheduling task", task.id, "with error", err)
		}
	}
}

func suspendJob(jobUuid string) error {
	_, err := db.Exec("update task t join job j on t.job_uuid = j.uuid "+
		"set t.status = ?, j.status = ? "+
		"where j.uuid = ? and t.status = ?", "Suspended", "Suspended", jobUuid, "Pending")
	if err != nil {
		logger.Println("Error suspending for job! uuid ", jobUuid, "with error ", err)
		return err
	}
	return nil
}

func resumeJob(jobUuid string) error {
	_, err := db.Exec("update task t join job j on t.job_uuid = j.uuid "+
		"set t.status = ?, j.status = ? "+
		"where j.uuid = ? and t.status = ?", "Pending", "Pending", jobUuid, "Suspended")
	if err != nil {
		logger.Println("Error resuming for job! uuid ", jobUuid, "with error ", err)
		return err
	}
	return nil
}

func getUserTimeSpans(ak string, spans *[]Span) error {
	rows, err := db.Query("select start, end from schedule where access_key = ?", ak)
	if err != nil {
		logger.Println("Error querying table schedule:", err)
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var span Span
		if err := rows.Scan(&span.Start, &span.End); err != nil {
			logger.Println("Row scan error:", err)
			continue
		}
		*spans = append(*spans, span)
	}
	return nil
}

func delAndInsertScheTable(accessKey string, spans []Span) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec("delete from schedule where access_key = ?", accessKey)
	if err != nil {
		tx.Rollback()
		return err
	}
	for i := 0; i< len(spans); i++ {
		_, err := tx.Exec("insert into schedule(access_key, start, end) " +
		    "values(?, ?, ?)", accessKey, spans[i].Start, spans[i].End)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	tx.Commit()
	return nil
}

func queryJobList(accessKey string, stime string, etime string, status int, jobid string, result *[]JobList) error {
	sql := "select uuid, create_time, complete_time, status from job where access_key = \"" + accessKey + "\""
	if len(stime) != 0 {
		sql = sql + " AND create_time > FROM_UNIXTIME(" + stime + ")"
	}
	if len(etime) != 0 {
		sql = sql + " AND create_time < FROM_UNIXTIME(" + etime + ")"
	}
	if jobid != "" {
		sql = sql + " AND uuid = \"" + jobid + "\""
	}
	if status & 1 != 0 {
		sql = sql + " AND status = \"Finished\""
	}
	if status & 2 != 0 {
		sql = sql + " AND status = \"Pending\""
	}
	if status & 4 != 0 {
		sql = sql + " AND status = \"Failed\""
	}
	if status & 8 != 0 {
		sql = sql + " AND status = \"Scheduled\""
	}
	sql += " order by create_time desc limit 1000"
	logger.Println("EqueryJobList:", sql)
	rows, err := db.Query(sql)
	if err != nil {
		logger.Println("Error querying scheduled tasks:", err)
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var job JobList
		var rawCreateTime []byte
		var rawCompleteTime []byte
		if err := rows.Scan(&job.JobUuid, &rawCreateTime, &rawCompleteTime, &job.Status); err != nil {
			logger.Println("Row scan error:", err)
			continue
		}
		local, err := time.LoadLocation("Local")
		if err != nil {
			logger.Println("Error loading current location:", err)
			continue
		}
		if len(rawCreateTime) != 0 {
			date, err := time.ParseInLocation("2006-01-02 15:04:05", string(rawCreateTime), local)
			if err != nil {
				logger.Println("Error parsing date string from DB: ", string(rawCreateTime))
				continue
			}
			job.CreateTime = date.Unix()
		} else {
			job.CreateTime = 0
		}
		if len(rawCompleteTime) != 0 {
			date, err := time.ParseInLocation("2006-01-02 15:04:05", string(rawCompleteTime), local)
			if err != nil {
				logger.Println("Error parsing date string from DB: ", string(rawCompleteTime))
				continue
			}
			job.CompleteTime = date.Unix()
		} else  {
			job.CompleteTime = 0
		}
		*result = append(*result, job)
	}
	return nil
}

func queryFinishedSize(accessKey string) (int64, error) {
	var totalFinishedSize int64
	err := db.QueryRow("select sum(finished_size) from job where access_key = ?", accessKey).Scan(&totalFinishedSize)
	if err != nil {
		logger.Println("Error get total total finished size: ", err)
		return 0, nil
	}

	return totalFinishedSize, nil
}

func queryScheduledJobUuids(accessKey string, jobUuids *[]string) error {
	rows, err := db.Query("select uuid from job where "+
	"access_key = ? and status = ?", accessKey, "Scheduled")
	if err != nil {
		logger.Println("Error querying scheduled tasks:", err)
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			logger.Println("Row scan error:", err)
			continue
		}
		*jobUuids = append(*jobUuids, uuid)
	}
	return nil
}

func queryRunningUrls(jobUuid string, urls *[]string) error {
	rows, err := db.Query("select u.origin_url from url u "+
	    " join task t on u.task_id = t.id join job j on t.job_uuid = j.uuid "+
		"  where j.uuid = ? and t.status = ?", jobUuid, "Running")
	if err != nil {
		logger.Println("Error querying running tasks:", err)
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			logger.Println("Row scan error:", err)
			continue
		}
		*urls = append(*urls, url)
	}
	return nil
}

func getPendingUsers(aks *[]string) error {
	rows, err := db.Query("select distinct(access_key) from job "+
	"  where status = ?", "Pending")
	if err != nil {
		logger.Println("Error querying distinct access key:", err)
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ak string
		if err := rows.Scan(&ak); err != nil {
			logger.Println("Row scan error:", err)
			continue
		}
		*aks = append(*aks, ak)
	}
	return nil
}

func getUserPriority(ak string) (int, error) {
	priority := 0
	err := db.QueryRow("select priority from user where "+
	"access_key = ?", ak).Scan(&priority)
	if err != nil {
		logger.Println("Error querying finished task number: ", err)
		return 0, err
	}
	return priority, nil
}

func clearExecutors() {
	_, err := db.Exec("update executor set status = ?", "Lost")
	if err != nil {
		logger.Println("Error clearing executors: ", err)
	}
}

func clearRunningTask() {
	_, err := db.Exec("update task set status = ?, executor_uuid = NULL, schedule_time = NULL where "+
		"status = ?", "Pending", "Running")
	if err != nil {
		logger.Println("Error clearing running task with error ", err)
	}
}

func initS3ClusterAddr(cluster map[string]string) error {
	rows, err := db.Query("select target, addr from cluster")
	if err != nil {
		logger.Println("Error querying table cluster:", err)
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var target string
		var addr string
		if err := rows.Scan(&target, &addr); err != nil {
			logger.Println("Row scan error:", err)
			return err
		}
		cluster[target] = addr
	}
	return nil
}
