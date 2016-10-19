package graft

import (
	"log"
	"testing"
)

func TestNewNode(t *testing.T) {
	errChan := make(chan error)
	stateChangeChan := make(chan StateChange)
	handler := NewChanHandler(stateChangeChan, errChan)
	node, err := NewNode(handler, []string{"10.88.147.130:1213"}, "10.88.104.33", "test.log", 1213)
	if err != nil {
		log.Println(err.Error())
		return
	}
	if node.State() == LEADER {
		log.Println("i am leader")
	}
	for {

		select {
		case sc := <-stateChangeChan:
			log.Println(sc)

		case err := <-errChan:
			log.Println("%+v", err)
		}
	}

}
