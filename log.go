package graft

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

type checksum struct {
	sha, data []byte
}

type persistentState struct {
	CurrentTerm uint64
	VotedFor    string
}

func (n *Node) closeLog() error {
	err := os.Remove(n.logPath)
	n.logPath = ""
	return err
}

func (n *Node) initLog(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		return err
	}
	defer file.Close()
	n.logPath = path
	ps, err := n.readState(path)
	if err != nil && err != ErrLogNoState {
		return err
	}
	if ps != nil {
		n.setTerm(ps.CurrentTerm)
		n.setVote(ps.VotedFor)
	}
	return nil

}

func (n *Node) readState(path string) (*persistentState, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(buf) <= 0 {
		return nil, ErrLogNoState
	}

	ps := &persistentState{}
	if err := json.Unmarshal(buf, ps); err != nil {
		return nil, err
	}
	return ps, nil
}

func (n *Node) writeState() error {
	n.mu.Lock()
	ps := persistentState{
		CurrentTerm: n.term,
		VotedFor:    n.vote,
	}
	logPath := n.logPath
	n.mu.Unlock()
	buf, err := json.Marshal(ps)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(logPath, buf, 0660)

}
