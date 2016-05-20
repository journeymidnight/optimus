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

}

func insertTasks(tasks *[]TransferTask) error {

}

func upsertSlave(slave *Slave) error {

}

func getIdleExecutorsOnSlave(tx *sql.Tx, slaveId string) (executors []*Executor) {
// select for update
// use EXECUTOR_IDLE_THRESHOLD
}

func getPendingTasks(tx *sql.Tx, limit int) (tasks []*TransferTask) {
// select for update
}

func updateTaskStatus(tx *sql.Tx, tasks []*mesosproto.TaskInfo)  {
// call tx.commit() here
}