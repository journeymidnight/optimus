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
	//defer os.Remove(filename)
	defer file.Close()

	fileDl, err := NewFileDl("http://vss2.waqu.com/2gpq0lb12wtmnbcu/normal.mp4", file, 0)
	if err != nil {
		t.Error("Error new file downloader!")
		return
	}
	start := time.Now()
	bytesDone, dlErr := fileDl.Download()
	if dlErr != nil {
		t.Error("Error downloading file!")
	}
	fmt.Println("Downloaded Bytes:", bytesDone, "spend time:", time.Now().Sub(start).Seconds())
}

