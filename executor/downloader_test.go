package main

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func Test_DownloadFile(t *testing.T) {
	filename := "/tmp/downloadfile"
	file, err := os.Create(filename)
	if err != nil {
		t.Error("Error creating file!")
		return
	}
	defer os.Remove(filename)
	defer file.Close()

	fileDl, err := NewFileDl("http://vss2.waqu.com/2gpq0lb12wtmnbcu/normal.mp4", file)
	if err != nil {
		t.Error("Error new file downloader!")
		return
	}

	var finish = make(chan bool)
	fileDl.OnFinish(func() {
		finish <- true
	})

	var dlErr error
	fileDl.OnError(func(errCode int, err error) {
		dlErr = err
		fmt.Println("Error downloading: errCode:", errCode, "err:", err)
	})

	var exit bool
	var dlSize int64
	fileDl.Start()
	for !exit {
		dlSize = fileDl.GetDownloadedSize()

		select {
		case exit = <-finish:
			fmt.Println("downloaded size", dlSize)
		default:
			time.Sleep(time.Second * 1)
			fmt.Println("downloaded size", dlSize)
		}
	}

	if dlErr != nil {
		t.Error("Error downloading file!")
	}
}

