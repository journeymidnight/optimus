package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"fmt"
	"github.com/mesos/mesos-go/mesosproto"
)

func createDbConnection() *sql.DB {
	conn, err := sql.Open("mysql", DB_CONNECTION_STRING)
	if err != nil {
		panic(fmt.Sprintf("Error connecting to database: %v", err))
	}
	logger.Println("Connected to database")
	return conn
}

func insertRequest(req *TransferRequest) (id int64, err error) {
	result, err := db.Exec("insert request set id = 0, create_time = NOW()")
	if err != nil {
		return -1, err
	}
	return result.LastInsertId()
}

func insertTasks(tasks []*TransferTask) error {
	for _, task := range tasks {
		tx, err := db.Begin()
		if err != nil {return err}
		result, err := tx.Exec("insert into task(id, request_id, destination_type, destination_base_url, status) " +
		"values(?, ?, ?, ?, ?)", 0, task.RequestId, task.DestinationType, task.DestinationBaseUrl, task.Status)
		if err != nil {
			tx.Rollback()
			return err
		}
		taskId, err := result.LastInsertId()
		if err != nil {
			tx.Rollback()
			return err
		}
		for _, url := range task.SourceUrls {
			_, err := tx.Exec("insert into url(id, task_id, url) " +
			"values(?, ?, ?)", 0, taskId, url)
			if err != nil {
				tx.Rollback()
				return err
			}
		}
		tx.Commit()
	}
	return nil
}

func upsertSlave(slave *Slave) (int64, error) {
	result, err := db.Exec("insert into slave(id, uuid, hostname, status) " +
	"values(?, ?, ?, ?) on duplicate key update " +
	"status = values(status)," +
	"hostname = values(hostname)", 0, slave.uuid, slave.hostname, slave.status)
	if err != nil {
		return -1, err
	}
	id, err := result.LastInsertId()
	if id != 0 { // it's a insert operation
		return id, err
	}
	// it's an upsert operation, find id for the slave
	err = db.QueryRow("select id from slave where uuid = ?", slave.uuid).Scan(&id)
	return id, err
}

func getIdleExecutorsOnSlave(tx *sql.Tx, slaveId int64) (executors []*Executor) {
	rows, err := tx.Query("select uuid, task_running from executor where " +
		"slave_id = ? and task_running < ? for update", slaveId, EXECUTOR_IDLE_THRESHOLD)
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

func getPendingTasks(tx *sql.Tx, limit int) (tasks []*TransferTask) {
	taskRows, err := tx.Query("select id, request_id, destination_type, destination_base_url from task " +
		"where status = ? limit ? for update", "Pending", limit)
	if err != nil {
		logger.Println("Error querying pending tasks: ", err)
		return
	}
	defer taskRows.Close()
	for taskRows.Next() {
		var task TransferTask
		if err := taskRows.Scan(&task.Id, &task.RequestId, &task.DestinationType,
			&task.DestinationBaseUrl); err != nil {
			logger.Println("Row scan error: ", err)
			continue
		}
		task.Status = "Pending"
		tasks = append(tasks, &task)
	}
	for _, task := range tasks {
		urlRows, err := tx.Query("select url from url where task_id = ?", task.Id)
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
			task.SourceUrls = append(task.SourceUrls, url)
		}
		urlRows.Close()
	}
	return
}

func initializeTaskStatus(tx *sql.Tx, tasks []*mesosproto.TaskInfo, slaveId int64) {
	for _, task := range tasks {
		taskId := task.TaskId.GetValue()
		uuid := task.Executor.ExecutorId.GetValue()
		result, err := tx.Exec("insert into executor(id, slave_id, uuid, task_running) " +
			"values(?, ?, ?, ?) on duplicate key update " +
			"task_running = task_running + 1", 0, slaveId, uuid, 1)
		if err != nil {
			logger.Println("Error upsert executor: ", err)
			continue
		}
		executorId, err := result.LastInsertId()
		if err != nil {
			logger.Println("Error getting executor ID: ", err)
			continue
		}
		if executorId == 0 { // it's an upsert operation, need to find id
			err = tx.QueryRow("select id from executor where uuid = ?",
				uuid).Scan(&executorId)
			if err != nil {
				logger.Println("Error getting executor ID: ", err)
				continue
			}
		}
		_, err = tx.Exec("update task set executor_id = ?, status = ? where " +
			"id = ?", executorId, "Scheduled", taskId)
		if err != nil {
			logger.Println("Error update task: ", err)
		}
	}
	tx.Commit()
}

func updateTask(taskId string, executorId string, status string)  {
	logger.Println("Updating task. task id:", taskId, "executor id:", executorId, "status:", status)
}

func slaveLostUpdate(slaveUuid string)  {
	logger.Println("slave uuid: ", slaveUuid)
}

func executorLostUpdate(executorUuid string)  {
	logger.Println("executor uuid: ", executorUuid)
}