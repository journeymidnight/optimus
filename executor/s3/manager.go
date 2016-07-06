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
  "crypto/md5"
  "encoding/hex"
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
	chunkSize    int64
	totalSize    int64
	uploaded     int64
	parts        []s3.Part
	multi        *s3.Multi

	onFinish     func(error)
}

func (d *Driver) NewMultiPartWriter(xkey string, chunkSize int64, acl string) (*MultiPartWriter, error) {

	xmulti, err := d.Bucket.Multi(xkey, d.getContentType(), s3.ACL(acl))
	if err != nil {
		return nil, err
	}

	w := MultiPartWriter{driver: d, key: xkey, chunkSize: chunkSize, totalSize: 0, multi: xmulti}

	return &w, nil
}

func (w *MultiPartWriter) Start(r s3.ReaderAtSeeker) error {
	go func() {
		var err error
		for {
			parts, err := w.putAll(r)
			if err != nil {
				break
			}
			_, err = w.complete(parts)
			if err != nil {
				break
			}
			break
		}
		w.triggerFinish(err)
	}()

	return nil
}

func (w *MultiPartWriter) OnFinish(fn func(error)) {
	w.onFinish = fn
}

func (w *MultiPartWriter) triggerFinish(err error) {
	if w.onFinish != nil {
		go w.onFinish(err)
	}
}

func hasCode(err error, code string) bool {
	s3err, ok := err.(*s3.Error)
	return ok && s3err.Code == code
}

func seekerInfo(r io.ReadSeeker) (md5hex string, err error) {
	_, err = r.Seek(0, 0)
	if err != nil {
		return "", err
	}
	digest := md5.New()
	_, err = io.Copy(digest, r)
	if err != nil {
		return "", err
	}
	sum := digest.Sum(nil)
	md5hex = hex.EncodeToString(sum)
	return md5hex, nil
}

func (w *MultiPartWriter) putAll(r s3.ReaderAtSeeker) ([]s3.Part, error) {
	old, err := w.multi.ListParts()
	if err != nil && !hasCode(err, "NoSuchUpload") {
		return nil, err
	}

	reuse := 0   // Index of next old part to consider reusing.
	current := 1 // Part number of latest good part handled.
	totalSize, err := r.Seek(0, 2)
	if err != nil {
		return nil, err
	}
	totalSize = int64(totalSize)
	first := true // Must send at least one empty part if the file is empty.
	var result []s3.Part
NextSection:
	for offset := int64(0); offset < totalSize || first; offset += w.chunkSize {
		first = false
		var partSize int64
		if offset + w.chunkSize > totalSize {
			partSize = totalSize - offset
		} else {
			partSize = w.chunkSize
		}
		section := io.NewSectionReader(r, offset, partSize)
		if reuse < len(old) && old[reuse].N == current {
			// Looks like this part was already sent.
			md5hex, err := seekerInfo(section)
			if err != nil {
				return nil, err
			}

			part := &old[reuse]
			etag := md5hex
			if part.N == current && part.Size == partSize && part.ETag == etag {
				fmt.Println("part:", part.N, " is reused!")
				// Checksum matches. Reuse the old part.
				result = append(result, *part)
				w.uploaded += partSize
				current++
				reuse++
				continue NextSection
			}
			reuse++
		}
		fmt.Println("Now put part ", current)

		// Part wasn't found or doesn't match. Send it.
		part, err := w.multi.PutPart(current, section)
		if err != nil {
			return nil, err
		}
		result = append(result, part)
		w.uploaded += partSize
		current++
	}
	return result, nil
}

func (w *MultiPartWriter) complete(parts []s3.Part) (int64, error) {
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

func (w *MultiPartWriter) GetUploadedSize() int64 {
	return w.uploaded
}

func (w *MultiPartWriter) Size() int64 {
	return w.totalSize
}
