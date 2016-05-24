/*
SQLs to create tables for Optimus
 */

SET FOREIGN_KEY_CHECKS = 0;

DROP TABLE IF EXISTS user;
CREATE TABLE user (
  id BIGINT NOT NULL AUTO_INCREMENT,
  access_key VARCHAR(50) NOT NULL UNIQUE,
  secret_key VARCHAR(50) NOT NULL,
  description VARCHAR(50) DEFAULT NULL,
  PRIMARY KEY (id),
  INDEX (access_key)
);

DROP TABLE IF EXISTS job;
CREATE TABLE job (
  id BIGINT NOT NULL AUTO_INCREMENT,
  uuid CHAR(60) NOT NULL UNIQUE,
  create_time DATETIME,
  PRIMARY KEY (id),
  INDEX (uuid)
);

DROP TABLE IF EXISTS slave;
CREATE TABLE slave (
  id BIGINT NOT NULL AUTO_INCREMENT,
  uuid CHAR(60) NOT NULL UNIQUE,
  hostname VARCHAR(50),
  status VARCHAR(20) NOT NULL,
  PRIMARY KEY (id),
  INDEX (uuid)
);

DROP TABLE IF EXISTS executor;
CREATE TABLE executor (
  id BIGINT NOT NULL AUTO_INCREMENT,
  uuid CHAR(60) NOT NULL UNIQUE,
  slave_uuid CHAR(60) NOT NULL UNIQUE,
  task_running INT DEFAULT 0,
  status VARCHAR(20) NOT NULL,
  PRIMARY KEY (id),
  INDEX (uuid),
  INDEX (slave_uuid),
  FOREIGN KEY (slave_uuid) REFERENCES slave(uuid)
);

DROP TABLE IF EXISTS task;
CREATE TABLE task (
  id BIGINT NOT NULL AUTO_INCREMENT,
  job_uuid CHAR(60) NOT NULL UNIQUE,
  executor_uuid CHAR(60) NOT NULL UNIQUE,
  target_type VARCHAR(20) NOT NULL,
  target_bucket VARCHAR(100),
  status VARCHAR(20) NOT NULL,
  PRIMARY KEY (id),
  INDEX (job_uuid),
  INDEX (executor_uuid),
  INDEX (status),
  FOREIGN KEY (job_uuid) REFERENCES job(uuid),
  FOREIGN KEY (executor_uuid) REFERENCES executor(uuid)
);

DROP TABLE IF EXISTS url;
CREATE TABLE url (
  id BIGINT NOT NULL AUTO_INCREMENT,
  task_id BIGINT NOT NULL UNIQUE,
  origin_url TEXT NOT NULL,
  target_url TEXT,
  status VARCHAR(20) NOT NULL,
  PRIMARY KEY (id),
  INDEX (task_id),
  FOREIGN KEY (task_id) REFERENCES task(id)
);

SET FOREIGN_KEY_CHECKS = 1;

