package s3

import (
  "github.com/goamz/goamz/s3"
  "github.com/goamz/goamz/aws"
  "io"
  "errors"
  "net/http"
  "strconv"
  "strings"
  "bytes"
  "fmt"
)


var (
	BAD_PATH = errors.New("bad path")
)

type Driver struct {
	S3            *s3.S3
	Encrypt       bool
	RootDirectory string
	Bucket        *s3.Bucket
	ContentType   string
}

func NewDriver(ak string, sk string, endpoint string, xbucket string, contentType string) *Driver {
	var region = aws.Region{
		"local_s3",
		"",
		endpoint,
		"",
		true,
		true,
		"https://sdb.us-west-1.amazonaws.com",
		"",
		"https://sns.us-west-1.amazonaws.com",
		"https://sqs.us-west-1.amazonaws.com",
		"https://iam.amazonaws.com",
		"https://elasticloadbalancing.us-west-1.amazonaws.com",
		"https://dynamodb.us-west-1.amazonaws.com",
		aws.ServiceInfo{"https://monitoring.us-west-1.amazonaws.com", aws.V2Signature},
		"https://autoscaling.us-west-1.amazonaws.com",
		aws.ServiceInfo{"https://rds.us-west-1.amazonaws.com", aws.V2Signature},
		"https://sts.amazonaws.com",
		"https://cloudformation.us-west-1.amazonaws.com",
		"https://ecs.us-west-1.amazonaws.com",
		"https://streams.dynamodb.us-west-1.amazonaws.com",
	}

	auth := aws.Auth{AccessKey:ak,SecretKey:sk}
	s3obj := s3.New(auth, region)
	bucket := s3obj.Bucket(xbucket)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return &Driver{S3:s3obj,Bucket:bucket,RootDirectory:"",ContentType:contentType}
}


func (d *Driver) PutContent(path string, contents []byte, ACL string) error {
	s := s3.Options{}
	err := d.Bucket.Put(d.s3Path(path), contents, d.getContentType(), s3.ACL(ACL), s)
	return err
}


func (d *Driver) s3Path(path string) string {
	return strings.TrimLeft(strings.TrimRight(d.RootDirectory, "/")+path, "/")
}


func (d *Driver) Reader(xpath string, offset int64) (io.ReadCloser, error) {

	path := d.s3Path(xpath)
	if path == "" {
		return nil, BAD_PATH
	}

	headers := make(http.Header)
	headers.Add("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")

	resp, err := d.Bucket.GetResponse(path)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (d *Driver) getContentType() string {
	return d.ContentType
}



type SimpleMultiPartWriter struct {
	driver *Driver
	key string
	minChunkSize int
        data []byte
        totalSize int64
        parts  []s3.Part
	multi *s3.Multi
}


func (d *Driver) NewSimpleMultiPartWriter(xkey string, xminChunkSize int, acl string) (*SimpleMultiPartWriter,error){

	xmulti, err := d.Bucket.InitMulti(xkey, d.getContentType(), s3.ACL(acl))
	if err != nil {
		return nil, err
	}

	w := SimpleMultiPartWriter{driver:d, key:xkey, minChunkSize:xminChunkSize, totalSize:0, multi:xmulti}

	return &w,nil
}


func (w *SimpleMultiPartWriter) Size() int64 {
	return w.totalSize
}

func (w *SimpleMultiPartWriter) Write(b []byte) (n int, err error) {
       w.data = append(w.data, b...)
       w.totalSize += int64(len(b))

       if len(w.data) > w.minChunkSize {
		part,err := w.multi.PutPart(len(w.parts)+1, bytes.NewReader(w.data))
		fmt.Printf("upload part number %d, %d bytes\n", len(w.parts) + 1, len(w.data))
		if err != nil {
			return 0, err
		}
		w.parts = append(w.parts, part)
		w.data = nil
       }
       return len(b), nil
}


func (w *SimpleMultiPartWriter) Seek(offset int64, whence int) (int64, error) {
	return 0, errors.New("not implemented")
}


func (w *SimpleMultiPartWriter) Close() error {
	if len(w.data) > 0 {
		part, err := w.multi.PutPart(len(w.parts) + 1, bytes.NewReader(w.data))
		fmt.Printf("close upload part number%d, %dbytes\n", len(w.parts) + 1, len(w.data))
		if err != nil {
			return err
		}
		w.parts = append(w.parts, part)
	}

	err := w.multi.Complete(w.parts)

	if err != nil {
		fmt.Println(err)
		w.multi.Abort()
	}
	return nil
}


type MultiPartWriter struct {
	driver       *Driver
	key          string
	minChunkSize int
	data         []byte
	totalSize    int64
	parts        []s3.Part
	multi        *s3.Multi
}

func (d *Driver) NewMultiPartWriter(xkey string, xminChunkSize int, acl string) (*MultiPartWriter, error) {

	xmulti, err := d.Bucket.Multi(xkey, d.getContentType(), s3.ACL(acl))
	if err != nil {
		return nil, err
	}

	w := MultiPartWriter{driver: d, key: xkey, minChunkSize: xminChunkSize, totalSize: 0, multi: xmulti}

	return &w, nil
}

func (w *MultiPartWriter) PutAll(r s3.ReaderAtSeeker, partSize int64) ([]s3.Part, error) {
	parts, err := w.multi.PutAll(r, partSize)
	if err != nil {
		return nil, err
	}
	return parts, err
}

func (w *MultiPartWriter) Complete(parts []s3.Part) (int64, error) {
	err := w.multi.Complete(parts)
	if err != nil {
		return 0, err
	}
	for index := 0; index < len(parts); index++ {
		w.totalSize += parts[index].Size
	}
	w.parts = parts
	return w.totalSize, nil
}

func (w *MultiPartWriter) Size() int64 {
	return w.totalSize
}
