package graft

import (
	"log"
	"testing"
	"time"
)

func TestNewNode(t *testing.T) {
	node, err := NewNode([]string{"10.88.147.130:1213", "10.88.147.2:1213"}, "10.88.104.33", "test.log", 1213)
	if err != nil {
		log.Println(err.Error())
		return
	}
	log.Println("test")
	if node.State() == LEADER {
		log.Println("i am leader")
	}
	for {

		select {
		case sc := <-node.StateChg:
			log.Println(sc)
		default:
			log.Println(node.State())
			time.Sleep(1 * time.Second)
		}
	}

}
