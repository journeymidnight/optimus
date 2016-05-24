# 验证

用单独的验证系统，我们知道用户的AK/SK

transfer系统专用

AK,SK

# 验证方法

build signature string

signatureStr=
	Method + '\n'
	Date + '\n'
	Host + '\n'
	Path

注意: 没有后面的query

Date format

	lua:
	print(os.date("!%a, %d %b %Y %H:%M:%S %Z"))
	'Tue, 24 May 2016 03:25:18 GMT'

	python:
	print time.strftime('%a, %d %b %Y %H:%M:%S %Z')
	'Tue, 24 May 2016 11:45:57 CST'

Add a Header

Authorization: AK:base64(hmac(signatureStr, SK))

#提交任务

Method:PUT

URL:  /submittask?token=$token&callback=http://yourtask

URL:  /submittask

Body:


token由用户指定

Json Format:

	S3
	{
	    "origin-files": [
		"http://abc",
		"http://def",
		"http://bad"
	    ],
	    "target-type": "s3s",
	    "target-bucket": "bucketone"
	}

	Vaas
	{
	    "origin-files": [
		"abc",
		"def",
	    ],
	    "target-type": "Vaas",
	}


Response:

code:200

Json Format: 

	{"taskid":"1234567"}

#Callback 请求

Method:PUT


URL:用户指定$token

Body:

Json Format:

	{
	"taskid":"1234567",
	"success-files":[
		"http://abc",
		"http://def",
	]
	"failed-files":[
		"http://bad"
	]
	}


#查询请求

Method:GET
URL:/status?taskid=343434

Response:

body:

Json Format:

	{
	"taskid":"1234567",
	"success-files":[
		"http://abc",
		"http://def",
	]
	"failed-files":[
		"http://bad"
	]

	"queue-files"[
		"http://queue.file"
	]
	}
