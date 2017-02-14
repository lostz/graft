package graft

import (
	"log"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	r, err := New([]string{"10.88.147.130:6000"}, "10.88.104.33:6000", 6000)
	if err != nil {
		log.Panic("new :%v", err)
		return
	}
	for {
		log.Println("", r.state)
		time.Sleep(10 * time.Second)
	}

}
