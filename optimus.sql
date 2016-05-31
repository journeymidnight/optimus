/*
SQLs to create tables for Optimus
 */

SET FOREIGN_KEY_CHECKS = 0;

DROP TABLE IF EXISTS user;
CREATE TABLE user (
  id BIGINT NOT NULL AUTO_INCREMENT,
  access_key VARCHAR(50) NOT NULL UNIQUE,
  secret_key VARCHAR(50) NOT NULL,
  s3_ak VARCHAR(50) DEFAULT NULL,
  s3_sk VARCHAR(50) DEFAULT NULL,
  vass_ak VARCHAR(50) DEFAULT NULL,
  vass_sk VARCHAR(50) DEFAULT NULL,
  description VARCHAR(50) DEFAULT NULL,
  PRIMARY KEY (id),
  INDEX (access_key)
);

DROP TABLE IF EXISTS job;
CREATE TABLE job (
  id BIGINT NOT NULL AUTO_INCREMENT,
  uuid CHAR(60) NOT NULL UNIQUE,
  access_key VARCHAR(50) NOT NULL,
  create_time DATETIME,
  complete_time DATETIME,
  callback_token VARCHAR(100),
  callback_url TEXT,
  status VARCHAR(20) NOT NULL,
  PRIMARY KEY (id),
  INDEX (uuid),
  FOREIGN KEY (access_key) REFERENCES user(access_key)
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
  slave_uuid CHAR(60) NOT NULL,
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
  job_uuid CHAR(60) NOT NULL,
  executor_uuid CHAR(60),
  target_type VARCHAR(20) NOT NULL,
  target_bucket VARCHAR(100),
  target_acl VARCHAR(20),
  status VARCHAR(20) NOT NULL,
  access_key VARCHAR(50),
  secret_key VARCHAR(50),
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
  task_id BIGINT NOT NULL,
  origin_url TEXT NOT NULL,
  target_url TEXT,
  status VARCHAR(20) NOT NULL,
  PRIMARY KEY (id),
  INDEX (task_id),
  FOREIGN KEY (task_id) REFERENCES task(id)
);

SET FOREIGN_KEY_CHECKS = 1;

-- For tests
INSERT INTO user (access_key, secret_key, s3_ak, s3_sk)
    VALUES ("hehe", "haha", "9EEIWGS705M4ZJ3N7FEM", "8humW3nOraybmbIjY6s15IVned87gz/nUrgxYlEX");