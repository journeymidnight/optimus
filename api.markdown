# Optimus API文档

## 鉴权

Optimus目前使用单独的鉴权系统，会为每个用户分配Access Key(AK)和Secret Key(SK)

用户需要在API请求头部添加如下Header：

- x-date
- Authorization

鉴权失败的请求Optimus会返回401错误

### Header生成方法

#### x-date

请求发出时的时间，格式为

```
lua:
print(os.date("!%a, %d %b %Y %H:%M:%S %Z"))
'Tue, 24 May 2016 03:25:18 GMT'

python:
print time.strftime('%a, %d %b %Y %H:%M:%S %Z')
'Tue, 24 May 2016 11:45:57 CST'

javascript:
console.log(new Date().toUTCString())
'Tue, 24 May 2016 06:48:20 GMT'
```

#### Authorization

格式为`AK:base64(hmac(signatureStr, SK))`，hmac目前采用的hash算法是`SHA-1`

其中，`signatureStr`是需要签名的内容，格式为：
	HTTP_Method + '\n'
	Date + '\n'
	URL_Host + '\n'
	URL_Path

例如一个提交任务请求(PUT /transferjob，假设服务器地址为optimus.lecloud.com)，需要签名的内容为：

```
PUT\n
Tue, 24 May 2016 06:48:20 GMT\n
optimus.lecloud.com\n
/transferjob
```

注意: URL标准格式为```scheme://[userinfo@]host/path[?query][#fragment]```

##提交任务

- PUT /transferjob
- PUT /transferjob?token=Token&callback=http://callback_url

使用第二种格式时，任务结束时Optimus会向http://callback_url发送callback(详见下文`Callback请求`)；Token字段可以不填写

Request body(使用JSON格式):

- S3

  ```json
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
  ```


- Vaas

  ```json
  {
      "origin-files": [
  	"http://abc",
  	"http://def",
      ],
      "target-type": "Vaas",
  }
  ```

Response code: 202

Response body(JSON格式): 

```json
{"jobid":Job_ID}
```
###Callback请求

- PUT http://callback_url?Token

Request Body(JSON格式):

```json
{
    "jobid": Job_ID,
    "success-files":[
        "http://abc",
        "http://def",
    ],
    "failed-files":[
        "http://bad"
    ]
}
```


##查询任务状态

- GET /status?jobid=Job_ID

Response body(JSON格式):

```json
{
    "jobid": JOB_ID,
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
```

