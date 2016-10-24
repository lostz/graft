package graft

import (
	"log"
	"testing"
)

func TestNewNode(t *testing.T) {
	node, err := NewNode([]string{"10.88.147.130:1213", "10.88.147.2:1213"}, "10.88.104.33", "test.log", 1213)
	if err != nil {
		log.Println(err.Error())
		return
	}
	if node.State() == LEADER {
		log.Println("i am leader")
	}
	for {

		select {
		case sc := <-node.StateChg:
			log.Println(sc)
		}
	}

}
