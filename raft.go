package graft

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/lostz/graft/protocol"
)

const (
	LEADER = iota
	CANDIDATE
	FLLOWER
	HBINTERVAL = 50 * time.Millisecond // 50ms
)

// Raft raft peer
type Raft struct {
	mu            sync.Mutex
	me            string
	peers         []string
	state         int
	voteCount     int
	chanHeartbeat chan bool
	chanGrantVote chan bool
	chanLeader    chan bool
	currentTerm   uint64
	votedFor      string
}

// GetState return currentTerm and whether this peer is leader
func (rf *Raft) GetState() (uint64, bool) {
	return rf.currentTerm, rf.state == LEADER
}

// IsLeader is leader
func (rf *Raft) IsLeader() bool {
	return rf.state == LEADER
}

// SendVoteRequest rpc handler
func (rf *Raft) SendVoteRequest(context context.Context, vreqt *protocol.VoteRequest) (vrepe *protocol.VoteResponse, err error) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	vrepe.Granted = false
	if vreqt.Term < rf.currentTerm {
		vrepe.Term = rf.currentTerm
		return
	}
	if vreqt.Term > rf.currentTerm {
		rf.currentTerm = vreqt.Term
		rf.state = FLLOWER
		rf.votedFor = ""
	}
	if rf.votedFor == "" || rf.votedFor == vreqt.Candidate {
		rf.chanGrantVote <- true
		rf.state = FLLOWER
		vrepe.Granted = true
		rf.votedFor = vreqt.Candidate
	}
	return

}

// Heartbeat rpc handler
func (rf *Raft) Heartbeat(context context.Context, hreqt *protocol.HeartbeatRequest) (repe *protocol.Response, err error) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if hreqt.Term < rf.currentTerm {
		repe.Term = rf.currentTerm
		return
	}
	rf.chanHeartbeat <- true
	if hreqt.Term > rf.currentTerm {
		rf.currentTerm = hreqt.Term
		rf.state = FLLOWER
		rf.votedFor = ""
		repe.Term = hreqt.Term
	}
	return
}

func (rf *Raft) sendRequestVote(peer string, vreqt *protocol.VoteRequest) bool {
	conn, err := grpc.Dial(peer, []grpc.DialOption{grpc.WithTimeout(300 * time.Millisecond), grpc.WithInsecure()}...)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
		return false
	}
	defer conn.Close()
	c := protocol.NewRaftClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	vrepe, err := c.SendVoteRequest(ctx, vreqt)
	if err != nil {
		log.Fatalf("did not VoteRequest: %v", err)
		return false
	}
	rf.mu.Lock()
	defer rf.mu.Unlock()
	term := rf.currentTerm
	if rf.state != CANDIDATE {
		return true
	}
	if vreqt.Term != term {
		return true
	}
	if vrepe.Term > term {
		rf.currentTerm = term
		rf.state = FLLOWER
		rf.votedFor = ""
	}
	if vrepe.Granted {
		rf.voteCount++
		if rf.state == CANDIDATE && rf.voteCount > len(rf.peers)/2 {
			rf.state = FLLOWER
			rf.chanLeader <- true
		}
	}
	return true
}

func (rf *Raft) sendHeartbeat(peer string, hreqt *protocol.HeartbeatRequest) bool {
	conn, err := grpc.Dial(peer, []grpc.DialOption{grpc.WithTimeout(300 * time.Millisecond), grpc.WithInsecure()}...)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
		return false
	}
	defer conn.Close()
	c := protocol.NewRaftClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	repe, err := c.SendHeartbeat(ctx, hreqt)
	if err != nil {
		log.Fatalf("did not HeartbeatRequest: %v", err)
		return false
	}
	rf.mu.Lock()
	defer rf.mu.Unlock()
	term := rf.currentTerm
	if rf.state != LEADER {
		return true
	}
	if hreqt.Term != rf.currentTerm {
		return true
	}
	if repe.Term > rf.currentTerm {
		rf.currentTerm = repe.Term
		rf.state = FLLOWER
		rf.votedFor = ""
		return true
	}
	return true
}

func (rf *Raft) broadcastRequestVote() {
	rf.mu.Lock()
	vreqt := &protocol.VoteRequest{Term: rf.currentTerm, Candidate: rf.me}
	rf.mu.Unlock()
	for _, peer := range rf.peers {
		if peer != rf.me && rf.state == CANDIDATE {
			go func(peer string) {
				rf.sendRequestVote(peer, vreqt)
			}(peer)
		}
	}
}

func (rf *Raft) broadcastHeartbeat() {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	hreqt := &protocol.HeartbeatRequest{Term: rf.currentTerm, Leader: rf.me}
	for _, peer := range rf.peers {
		if peer != rf.me && rf.state == LEADER {
			go func(peer string) {
				rf.sendHeartbeat(peer, hreqt)
			}(peer)
		}

	}
}

func (rf *Raft) loop() {
	for {
		switch rf.state {
		case FLLOWER:
			select {
			case <-rf.chanHeartbeat:
			case <-rf.chanGrantVote:
			case <-time.After(time.Duration(rand.Int63()%333+550) * time.Millisecond):
				rf.state = CANDIDATE
			}
		case LEADER:
			rf.broadcastHeartbeat()
			time.Sleep(HBINTERVAL)
		case CANDIDATE:
			rf.mu.Lock()
			rf.currentTerm++
			rf.votedFor = rf.me
			rf.voteCount = 1
			rf.mu.Unlock()
			go rf.broadcastRequestVote()
			select {
			case <-time.After(time.Duration(rand.Int63()%300+510) * time.Millisecond):
			case <-rf.chanHeartbeat:
				rf.state = FLLOWER
			case <-rf.chanLeader:
				rf.mu.Lock()
				rf.state = LEADER
				rf.mu.Unlock()
			}

		}

	}

}

func New(peers []string, me string, port int) (*Raft, error) {
	rf := &Raft{}
	rf.peers = peers
	rf.me = me
	rf.state = FLLOWER
	rf.votedFor = ""
	rf.currentTerm = 0
	rf.chanHeartbeat = make(chan bool, 100)
	rf.chanGrantVote = make(chan bool, 100)
	rf.chanLeader = make(chan bool, 100)
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	g := grpc.NewServer()
	protocol.RegisterRaftServer(g, rf)
	rf.server = g
	go g.Serve(lis)
	go rf.loop()
	return rf, nil
}
