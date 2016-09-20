package graft

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/context"

	log "github.com/Sirupsen/logrus"
	"github.com/lostz/graft/protocol"

	"google.golang.org/grpc"
)

//Node ...
type Node struct {
	electTimer *time.Timer
	handler    ChanHandler
	id         string
	ip         string
	leader     string
	logPath    string
	mu         sync.Mutex
	peers      []string
	quit       chan chan struct{}
	server     *grpc.Server
	state      State
	stateChg   []*StateChange
	term       uint64
	vote       string
}

func (n *Node) broadcastVote() int {
	votes := 1
	for _, peer := range n.peers {
		conn, err := grpc.Dial(peer, grpc.WithTimeout(500*time.Millisecond))
		if err != nil {
			log.WithFields(log.Fields{
				"addr": peer,
			}).Errorf("grpc dial %+v", err)
			continue
		}
		defer conn.Close()
		c := protocol.NewRaftClient(conn)
		vresp, err := c.VoteOn(context.Background(), &protocol.VoteRequest{Term: n.term, Candidate: n.id})
		if err != nil {
			log.WithFields(log.Fields{
				"addr": peer,
			}).Errorf("voteOn %+v", err)
			continue
		}
		if vresp.Granted && vresp.Term == n.term {
			votes++
		}

	}

	return votes
}

func (n *Node) broadcastHearbeat() {

	for _, peer := range n.peers {
		conn, err := grpc.Dial(peer, grpc.WithTimeout(1000*time.Millisecond))
		if err != nil {
			log.WithFields(log.Fields{
				"addr": peer,
			}).Errorf("grpc dial %+v", err)
			continue
		}
		defer conn.Close()
		c := protocol.NewRaftClient(conn)
		_, err = c.Heartbeat(context.Background(), &protocol.HeartbeatRequest{Term: n.term, Leader: n.id})
		if err != nil {
			log.WithFields(log.Fields{
				"addr": peer,
			}).Errorf("heartbeat  %+v", err)
			continue

		}

	}
}

func (n *Node) clearTimers() {
	if n.electTimer != nil {
		n.electTimer.Stop()
		n.electTimer = nil
	}
}

//Close wait until the state is processed
func (n *Node) Close() {
	if n.State() == CLOSED {
		return
	}
	n.server.GracefulStop()
	n.waitOnLoopFinish()
	n.clearTimers()
	n.closeLog()

}

//Heartbeat from leader
func (n *Node) Heartbeat(context context.Context, requset *protocol.HeartbeatRequest) (*protocol.HeartbeatResponse, error) {
	if requset.Term < n.term {
		return &protocol.HeartbeatResponse{}, nil
	}
	saveState := false
	if n.State() == LEADER {
		n.switchToFollower("")
	}
	n.mu.Lock()
	n.term = requset.Term
	n.vote = ""
	n.mu.Unlock()
	saveState = true
	n.resetElectionTimeout()
	if saveState {
		if err := n.writeState(); err != nil {
			log.WithFields(log.Fields{
				"term": n.term,
			}).Errorf("save state %+v", err)
		}
	}
	n.switchToFollower(requset.Leader)
	return &protocol.HeartbeatResponse{}, nil
}

func (n *Node) isRunning() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.state != CLOSED
}

func (n *Node) loop() {
	for n.isRunning() {
		switch n.State() {
		case FOLLOWER:
			n.runAsFollower()
		case CANDIDATE:
			n.runAsCandidate()
		case LEADER:
			n.runAsLeader()
		}

	}

}

func (n *Node) postStateChange(sc *StateChange) {
	go func() {
		n.handler.StateChange(sc.From, sc.To)
		n.mu.Lock()
		n.stateChg = n.stateChg[1:]
		if len(n.stateChg) > 0 {
			sc := n.stateChg[0]
			n.postStateChange(sc)
		}
		n.mu.Unlock()
	}()
}

func (n *Node) processQuit(q chan struct{}) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.state = CLOSED
	close(q)
}

func (n *Node) resetElectionTimeout() {
	n.electTimer.Reset(randElectionTimeout())
}

func (n *Node) runAsCandidate() {
	n.setVote(n.id)
	if err := n.writeState(); err != nil {
		n.switchToFollower("")
		return
	}
	votes := n.broadcastVote()
	if n.wonElection(votes) {
		n.switchToLeader()
		return
	}
	for {
		select {
		case q := <-n.quit:
			n.processQuit(q)
			return
		case <-n.electTimer.C:
			n.switchToCandidate()
			return

		}
	}

}

func (n *Node) runAsFollower() {
	for {
		select {
		case q := <-n.quit:
			n.processQuit(q)
			return
		case <-n.electTimer.C:
			n.switchToCandidate()
			return
		}

	}

}

func (n *Node) runAsLeader() {
	hb := time.NewTicker(100 * time.Millisecond)
	defer hb.Stop()
	for {
		select {
		case q := <-n.quit:
			n.processQuit(q)
			return
		case <-hb.C:
			n.broadcastHearbeat()

		}

	}

}

func (n *Node) setTerm(term uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.term = term
}
func (n *Node) setVote(candidate string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.vote = candidate
}

//State node current state
func (n *Node) State() State {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.state

}

func (n *Node) switchState(state State) {
	if state == n.state {
		return
	}
	old := n.state
	n.state = state
	sc := &StateChange{From: old, To: state}
	n.stateChg = append(n.stateChg, sc)
	if len(n.stateChg) == 1 {
		n.postStateChange(sc)
	}
}

func (n *Node) switchToCandidate() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.term++
	n.leader = ""
	n.state = CANDIDATE
	n.resetElectionTimeout()
	n.switchState(CANDIDATE)

}
func (n *Node) switchToFollower(leader string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.leader = leader
	n.switchState(FOLLOWER)
}

func (n *Node) switchToLeader() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.leader = n.id
	n.switchState(LEADER)
}

//VoteOn grpc
func (n *Node) VoteOn(context context.Context, requset *protocol.VoteRequest) (*protocol.VoteResponse, error) {
	resp := &protocol.VoteResponse{Term: n.term, Granted: false}
	if requset.Term < n.term {
		return resp, nil
	}
	saveState := false
	if requset.Term > n.term {
		n.mu.Lock()
		n.term = requset.Term
		n.vote = ""
		n.leader = ""
		n.mu.Unlock()
		saveState = true
	}
	if n.State() == LEADER && !saveState {
		return resp, nil
	}

	if n.vote != "" && n.vote != requset.Candidate {
		return resp, nil
	}
	n.setVote(requset.Candidate)
	if saveState {
		if err := n.writeState(); err != nil {
			n.setVote("")
			n.resetElectionTimeout()
			n.switchToFollower("")
			return resp, nil
		}
	}
	n.resetElectionTimeout()
	resp.Granted = true
	return resp, nil
}

func (n *Node) waitOnLoopFinish() {
	q := make(chan struct{})
	n.quit <- q
	<-q
}

func (n *Node) wonElection(votes int) bool {
	return votes >= quorumNeeded(len(n.peers))
}

func genID(ip string) string {
	IP := net.ParseIP(ip)
	machineID := uint16(IP[2])<<8 + uint16(IP[3])
	return strconv.FormatUint(uint64((time.Now().UTC().UnixNano()/sonyflakeTimeUnit)<<(BitLenSequence+BitLenMachineID))|uint64(0)<<BitLenMachineID|uint64(machineID), 10)

}

//NewNode ....
func NewNode(peers []string, ip, logPath string, port int) (*Node, error) {
	n := &Node{
		ip:    ip,
		state: FOLLOWER,
		peers: peers,
	}
	n.id = genID(ip)
	n.electTimer = time.NewTimer(randElectionTimeout())
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	g := grpc.NewServer()
	protocol.RegisterRaftServer(g, n)
	n.server = g
	go g.Serve(lis)
	go n.loop()
	return n, nil
}

func quorumNeeded(clusterSize int) int {
	switch clusterSize {
	// Handle 0, but 0 is really an invalid cluster size.
	case 0:
		return 0
	default:
		return clusterSize/2 + 1
	}
}

func randElectionTimeout() time.Duration {
	delta := rand.Int63n(int64(500 * time.Millisecond))
	return (500*time.Millisecond + time.Duration(delta))
}
