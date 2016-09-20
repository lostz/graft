package graft

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"io/ioutil"
	"os"
)

type checksum struct {
	sha, data []byte
}

type persistentState struct {
	currentTerm uint64
	votedFor    string
}

func (n *Node) closeLog() error {
	err := os.Remove(n.logPath)
	n.logPath = ""
	return err
}

func (n *Node) initLog(path string) error {
	if _, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0660); err != nil {
		return err
	}
	n.logPath = path
	ps, err := n.readState(path)
	if err != nil || err != ErrLogCorrupt {
		return err
	}
	if ps != nil {
		n.setTerm(ps.currentTerm)
		n.setVote(ps.votedFor)
	}
	return nil

}

func (n *Node) readState(path string) (*persistentState, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(buf) <= 0 {
		return nil, ErrNoStateLog
	}

	chs := &checksum{}
	if err := json.Unmarshal(buf, chs); err != nil {
		return nil, err
	}

	sha := sha1.New().Sum(chs.data)
	if !bytes.Equal(sha, chs.sha) {
		return nil, ErrLogCorrupt
	}

	ps := &persistentState{}
	if err := json.Unmarshal(chs.data, ps); err != nil {
		return nil, err
	}
	return ps, nil
}

func (n *Node) writeState() error {
	n.mu.Lock()
	ps := persistentState{
		currentTerm: n.term,
		votedFor:    n.vote,
	}
	logPath := n.logPath
	n.mu.Unlock()
	buf, err := json.Marshal(ps)
	if err != nil {
		return err
	}

	// Set a SHA1 to test for corruption on read
	chs := checksum{
		sha:  sha1.New().Sum(buf),
		data: buf,
	}

	data, err := json.Marshal(chs)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(logPath, data, 0660)

}
