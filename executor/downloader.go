package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
	"errors"
)

const (
	MaxThread = 1
	BufferSize = 4096   // How many bytes to read every time
)

type DlBuf  struct {
	buf    []byte
	off    int64
	len    int64
	idx    int
	reset  bool
	exit   bool
	replyc chan bool
}

type progressCB func(speed int, dlSize int64, rkv *RedisKeyValue)

type FileDl struct {
	Url  string
	Size int64
	MaxSpeed int64
	File *os.File
	rkv *RedisKeyValue

	progress progressCB
	stime time.Time
	spend time.Time

	BlockList []Block
	err       [MaxThread]error

	bytesDone  int64

	ContentType string
}

type Block struct {
	Begin int64 `json:"begin"`
	End   int64 `json:"end"`
}

func NewFileDl(url string, file *os.File, maxSpeed int64) (*FileDl, error) {
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
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			fmt.Println("Error HEAD file: ", url, "with status", resp.StatusCode)
			size = -1
			contentType = ""
		} else {
			size = resp.ContentLength
			contentType = resp.Header.Get("Content-Type")
		}
	}

	f := &FileDl{
		Url:  url,
		Size: size,
		File: file,
		MaxSpeed: maxSpeed,
		ContentType: contentType,
	}
	fmt.Println("maxSpeed:", maxSpeed)

	for i := 0; i < MaxThread; i++ {
		f.err[i] = nil
	}

	return f, nil
}

func (f *FileDl) GetContentType() string {
	return f.ContentType
}

func (f *FileDl) SetCB(rkv *RedisKeyValue, progress progressCB) {
	f.progress = progress
	f.rkv = rkv
}

func (f *FileDl) Download() (bytesDone int64, dlErr error) {
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
	return f.download()
}

func dlTimer(timeout chan bool) {
	time.Sleep(time.Microsecond * 100000)
	timeout <- true
}

func (f *FileDl) download() (bytesDone int64, dlErr error) {
	totalSlices := len(f.BlockList)
	bufChan := make(chan *DlBuf, totalSlices)
	for i := range f.BlockList {
		go func(id int) {
			defer func() {
				bufChan <- &DlBuf{exit: true}
			}()

			try := 3
			var err error
			for ; try != 0; try-- {
				err = f.downloadBlock(id, bufChan)
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

	f.stime = time.Now()
	var delayTime int
	if f.MaxSpeed > 0 {
		delayTime = int(float64(1000000 * totalSlices * BufferSize) / float64(f.MaxSpeed))
	}

	timeout := make (chan bool, 1)
	go dlTimer(timeout)
	var finished[MaxThread] int64
	var bytesPerSecond int
	var count int
	for {
		var flag bool
		var dlBuf *DlBuf

		select {
		case dlBuf = <- bufChan:
		case flag  = <- timeout:
			if count < totalSlices {
				go dlTimer(timeout)
			}
		    if f.progress != nil {
				f.progress(bytesPerSecond, bytesDone, f.rkv)
		    }
			//fmt.Println("downloaded size:", bytesDone, "downloaded speed:", bytesPerSecond)
		}

		if flag && count == totalSlices {
			break
		}

		if dlBuf != nil {
			if dlBuf.exit {
				count++
				continue
			} else if dlBuf.reset {
				bytesDone -= finished[dlBuf.idx]
				finished[dlBuf.idx] = 0
				continue
			}
			_, e := f.File.WriteAt(dlBuf.buf[:dlBuf.len], dlBuf.off)
			if e != nil {
				dlBuf.replyc <- false
				fmt.Println("Error writing file: idx:", dlBuf.idx, "len:", dlBuf.len, "error", e)
				continue
			}
			finished[dlBuf.idx] += dlBuf.len
			bytesDone += dlBuf.len
			dlBuf.replyc <- true
		} else {
			continue
		}

		now := time.Now()
		bytesDone += dlBuf.len
		bytesPerSecond = int(float64(bytesDone) / now.Sub(f.stime).Seconds())
		if f.MaxSpeed > 0 {
			var ratio = float64(bytesPerSecond) / float64(f.MaxSpeed)
			if ratio > 1.05 {
				delayTime += 10000;
			} else if ratio < 0.95 && delayTime >= 10000 {
				delayTime -= 10000;
			} else if ratio <  0.95 {
				delayTime = 0
			}
			time.Sleep(time.Duration(delayTime) * time.Microsecond)
		}
	}

	for i := 0; i < totalSlices; i++ {
		if f.err[i] != nil {
			dlErr = f.err[i]
			break
		}
	}
	f.bytesDone = bytesDone

	return
}

// download a file block
func (f *FileDl) downloadBlock(id int, bufChan chan *DlBuf) error {
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
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		fmt.Println("Error GET file: ", f.Url, "with status", resp.StatusCode)
		return errors.New("Error GET Request")
	}
	if end == -1 {
		f.ContentType = resp.Header.Get("Content-Type")
	}
	defer resp.Body.Close()

	var exit bool
	replyc := make(chan bool, 1)
	var buf = make([]byte, BufferSize)
	bufChan <- &DlBuf{buf: nil, off: 0, len: 0, replyc: replyc, reset: true, idx: id}
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

		bufChan <- &DlBuf{buf: buf, off: f.BlockList[id].Begin, len: readLen, replyc: replyc, reset: false, idx: id}
		var reply bool
		select {
		case reply = <-replyc:
		}
		if !reply {
			return nil
		}
		f.BlockList[id].Begin += readLen
	}

	return nil
}
