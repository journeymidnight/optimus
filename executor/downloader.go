package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	MaxThread = 1
	BufferSize = 4096   // How many bytes to read every time
)

type FileDl struct {
	Url  string
	Size int64
	File *os.File

	BlockList []Block
	err       [MaxThread]error

	onStart  func()
	onFinish func()
	onError  func(int, error)

	Downloaded [MaxThread]int64
	Lock       [MaxThread]sync.Mutex

	ContentType string
}

type Block struct {
	Begin int64 `json:"begin"`
	End   int64 `json:"end"`
}

func NewFileDl(url string, file *os.File) (*FileDl, error) {
	var size int64
	var contentType string
	var client = &http.Client{
		Timeout: time.Second * 20,
	}
	resp, err := client.Head(url)
	if err != nil {
		fmt.Println("Head error")
		size = -1
		contentType = ""
	} else {
		size = resp.ContentLength
		contentType = resp.Header.Get("Content-Type")
	}

	f := &FileDl{
		Url:  url,
		Size: size,
		File: file,
		ContentType: contentType,
	}

	for i := 0; i < MaxThread; i++ {
		f.err[i] = nil
	}

	return f, nil
}

func (f *FileDl) GetContentType() string {
	return f.ContentType
}

func (f *FileDl) Start() {
	go func() {
		if f.Size <= 0 {
			f.BlockList = append(f.BlockList, Block{0, -1})
		} else {
			blockSize := f.Size / int64(MaxThread)
			var begin int64
			for i := 0; i < MaxThread; i++ {
				var end = (int64(i) + 1) * blockSize
				f.BlockList = append(f.BlockList, Block{begin, end})
				begin = end + 1
			}

			f.BlockList[MaxThread-1].End = f.Size - 1

		}

		f.trigger(f.onStart)
		f.download()
	}()
}

func (f *FileDl) download() {
	totalSlices := len(f.BlockList)
	ok := make(chan bool, totalSlices)
	for i := range f.BlockList {
		go func(id int) {
			defer func() {
				ok <- true
			}()

			try := 3
			var err error
			for ; try != 0; try-- {
				err = f.downloadBlock(id)
				if err != nil {
					fmt.Println("Error downloading file block: id", id, "with error", err)
					// re-download the file block
					continue
				}
				break
			}
			if try == 0 {
				f.err[id] = err
			}

		}(i)
	}

	for i := 0; i < totalSlices; i++ {
		<-ok
	}

	for i := 0; i < totalSlices; i++ {
		if f.err[i] != nil {
			f.triggerOnError(0, f.err[i])
			break
		}
	}

	f.trigger(f.onFinish)
}

// download a file block
func (f *FileDl) downloadBlock(id int) error {
	request, err := http.NewRequest("GET", f.Url, nil)
	if err != nil {
		return err
	}
	begin := f.BlockList[id].Begin
	end := f.BlockList[id].End
	if end != -1 {
		request.Header.Set(
			"Range",
			"bytes="+strconv.FormatInt(begin, 10)+"-"+strconv.FormatInt(end, 10),
		)
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	if end == -1 {
		f.ContentType = resp.Header.Get("Content-Type")
	}
	defer resp.Body.Close()

	var exit bool
	var buf = make([]byte, BufferSize)
	for ; !exit; {
		n, e := resp.Body.Read(buf)
		if (e != nil) && (e != io.EOF){
			return e
		} else if e == io.EOF {
			exit = true
		}

		readLen := int64(n)
		if end != -1 {
			needSize := f.BlockList[id].End + 1 - f.BlockList[id].Begin
			if readLen > needSize {
				readLen = needSize
				exit = true
			}
		}
		_, e = f.File.WriteAt(buf[:readLen], f.BlockList[id].Begin)
		if e != nil {
			return e
		}

		f.Lock[id].Lock()
		f.Downloaded[id] += readLen
		f.Lock[id].Unlock()

		f.BlockList[id].Begin += readLen
	}

	return nil
}

// get statistics info
func (f FileDl) GetDownloadedSize() int64 {
	var size int64
	for i := 0; i < MaxThread; i++ {
		size += f.Downloaded[i]
	}
	return size
}

// events triggered when start a task
func (f *FileDl) OnStart(fn func()) {
	f.onStart = fn
}

// events triggered when finish a task
func (f *FileDl) OnFinish(fn func()) {
	f.onFinish = fn
}

// events triggered when error occured
func (f *FileDl) OnError(fn func(int, error)) {
	f.onError = fn
}

// trigger event
func (f FileDl) trigger(fn func()) {
	if fn != nil {
		go fn()
	}
}

// trigger error event
func (f FileDl) triggerOnError(errCode int, err error) {
	if f.onError != nil {
		go f.onError(errCode, err)
	}
}
