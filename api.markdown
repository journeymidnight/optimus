# 验证

用单独的验证系统，我们知道用户的AK/SK

transfer系统专用AK/SK

# 验证方法

build signature string

signatureStr=
	HTTP_Method + '\n'
	Date + '\n'
	URL_Host + '\n'
	URL_Path

注意: URL标准格式为：```scheme://[userinfo@]host/path[?query][#fragment]```

Date format

	lua:
	print(os.date("!%a, %d %b %Y %H:%M:%S %Z"))
	'Tue, 24 May 2016 03:25:18 GMT'
	
	python:
	print time.strftime('%a, %d %b %Y %H:%M:%S %Z')
	'Tue, 24 May 2016 11:45:57 CST'

增加如下Header:

Authorization: AK:base64(hmac(signatureStr, SK))

x-date: Date Format

#提交任务

Method:PUT

URL:  /transferjob?token=$token&callback=http://callback_url

URL:  /transferjob

token由用户指定，用于callback里用户方的验证

Body:

Json Format:

	S3
	{
	    "origin-files": [
		"http://abc",
		"http://def",
		"http://bad"
	    ],
	    "target-type": "s3s",
	    "target-bucket": "bucketone",
	    "target-acl":"public-read"
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

code:202

Json Format: 

	{"jobid":"1234567"}

#Callback 请求

Method:PUT


URL: http://callback_url?$token

Body:

Json Format:

	{
	"jobid":"1234567",
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
URL:/status?jobid=343434

Response:

body:

Json Format:

	{
	"jobid":"1234567",
	"success-files":[
		"http://abc",
		"http://def",
	],
	"failed-files":[
		"http://bad"
	],
	"queued-files"[
		"http://queue.file"
	]
	}
