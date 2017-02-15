package graft

import (
	"log"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	chanState := make(chan bool, 100)
	r, err := New([]string{"10.88.147.130:6000", "10.88.147.2:6003"}, "10.88.104.33:6000", 6000, chanState)
	if err != nil {
		log.Panic("new :%v", err)
		return
	}
	for {
		log.Println("", r.state)
		time.Sleep(10 * time.Second)
		r.Stop()
		time.Sleep(10 * time.Second)
		return
	}

}
