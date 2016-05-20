package main

import (
	"github.com/satori/go.uuid"
)

func Min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func newUuid() string {
	return uuid.NewV4().String()
}
