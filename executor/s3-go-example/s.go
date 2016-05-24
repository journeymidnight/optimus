package main

import (
	"os"
	"io"
	"fmt"
)

func main() {
	d := NewDriver("9EEIWGS705M4ZJ3N7FEM", "8humW3nOraybmbIjY6s15IVned87gz/nUrgxYlEX", "http://s3.lecloud.com", "test")

	//min mulitpart size is 5MB, But I use 50MB
	s3writer, err := d.NewSimpleMultiPartWriter("ver_00_22-1033839045-avc-799948-aac-63999-2802920-305780444-57a3b26e437e4f381ee3a78a948cf526-1458857629069.mp4", 4 << 20, "private")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer s3writer.Close()

	reader, err := os.Open("ver_00_22-1033839045-avc-799948-aac-63999-2802920-305780444-57a3b26e437e4f381ee3a78a948cf526-1458857629069.mp4")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer reader.Close()

	//var buf []byte := make([]byte, 1 << 20)
	n, err := io.Copy(s3writer, reader)
	fmt.Printf("read %d bytes\n", n)

}
