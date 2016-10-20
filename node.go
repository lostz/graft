package graft

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/lostz/graft/protocol"
	"github.com/uber-go/zap"

	"google.golang.org/grpc"
)

var logger = zap.New(zap.NewTextEncoder())

//Node ...
type Node struct {
	electTimer   *time.Timer
	handler      *ChanHandler
	id           string
	ip           string
	leader       string
	logPath      string
	mu           sync.Mutex
	peers        []string
	quit         chan chan struct{}
	server       *grpc.Server
	state        State
	stateChg     []*StateChange
	term         uint64
	vote         string
	VoteRequests chan *protocol.VoteRequest
	VoteResponse chan *protocol.VoteResponse
	HeartBeats   chan *protocol.HeartbeatRequest
}

func (n *Node) broadcastVote() {
	for _, peer := range n.peers {
		go n.votePeer(peer)
	}
}

func (n *Node) votePeer(peer string) {
	conn, err := grpc.Dial(peer, []grpc.DialOption{grpc.WithTimeout(500 * time.Millisecond), grpc.WithInsecure()}...)
	if err != nil {
		logger.Error(
			"grpc dial",
			zap.String("addr", peer),
			zap.String("err", err.Error()),
		)
		return
	}
	defer conn.Close()
	c := protocol.NewRaftClient(conn)
	c.ReceiveVoteRequest(context.Background(), &protocol.VoteRequest{Term: n.term, Candidate: n.id})
	return
}

func (n *Node) broadcastHearbeat() {

	for _, peer := range n.peers {
		go n.heartbeatPeer(peer)

	}
}

func (n *Node) heartbeatPeer(peer string) {
	conn, err := grpc.Dial(peer, []grpc.DialOption{grpc.WithTimeout(1000 * time.Millisecond), grpc.WithInsecure()}...)
	if err != nil {
		logger.Error(
			"grpc dial",
			zap.String("addr", peer),
			zap.String("err", err.Error()),
		)
		return
	}
	defer conn.Close()
	c := protocol.NewRaftClient(conn)
	c.Heartbeat(context.Background(), &protocol.HeartbeatRequest{Term: n.term, Leader: n.id})

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
func (n *Node) Heartbeat(context context.Context, requset *protocol.HeartbeatRequest) (*protocol.Response, error) {
	if requset.Term < n.term {
		return &protocol.Response{}, nil
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
			logger.Error(
				"write state",
				zap.Uint64("term", n.term),
				zap.String("error", err.Error()),
			)
		}
	}
	n.switchToFollower(requset.Leader)
	return &protocol.Response{}, nil
}

func (n *Node) handleHeartBeat(hb *protocol.HeartbeatRequest) bool {
	if hb.Term < n.term {
		return false
	}
	saveState := false
	stepDown := false
	if hb.Term > n.term {
		n.term = hb.Term
		n.vote = ""
		stepDown = true
		saveState = true
	}
	if n.State() == CANDIDATE && hb.Term >= n.term {
		n.term = hb.Term
		n.vote = ""
		stepDown = true
		saveState = true
	}
	n.resetElectionTimeout()
	if saveState {
		if err := n.writeState(); err != nil {
			stepDown = true
		}
	}
	return stepDown

}

func (n *Node) handleVoteRequest(vreq *protocol.VoteRequest) bool {
	deny := &protocol.VoteResponse{Term: n.term, Granted: false}
	if vreq.Term < n.term {
		n.sendVoteResponse(vreq.Candidate, deny)
		return false
	}
	saveState := false
	stepDown := false
	if vreq.Term > n.term {
		n.term = vreq.Term
		n.vote = ""
		n.leader = ""
		stepDown = true
		saveState = true
	}
	if n.State() == LEADER && !stepDown {
		n.sendVoteResponse(vreq.Candidate, deny)
		return stepDown
	}
	if n.vote != "" && n.vote != vreq.Candidate {
		n.sendVoteResponse(vreq.Candidate, deny)
		return stepDown
	}
	n.setVote(vreq.Candidate)
	if saveState {
		if err := n.writeState(); err != nil {
			n.setVote("")
			n.sendVoteResponse(vreq.Candidate, deny)
			n.resetElectionTimeout()
			return true
		}

	}
	accept := &protocol.VoteResponse{Term: n.term, Granted: true}
	n.sendVoteResponse(vreq.Candidate, accept)
	n.resetElectionTimeout()
	return stepDown

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
	votes := 1
	n.setVote(n.id)
	if err := n.writeState(); err != nil {
		n.switchToFollower("")
		return
	}
	if n.wonElection(votes) {
		n.switchToLeader()
		return
	}
	n.broadcastVote()
	for {
		select {
		case q := <-n.quit:
			n.processQuit(q)
			return
		case <-n.electTimer.C:
			n.switchToCandidate()
			return
		case vresp := <-n.VoteResponse:
			fmt.Println(vresp)
			if vresp.Granted && vresp.Term == n.term {
				votes++
				if n.wonElection(votes) {
					n.switchToLeader()
					return
				}

			}
		case vreq := <-n.VoteRequests:
			fmt.Println(vreq)
			if stepDown := n.handleVoteRequest(vreq); stepDown {
				n.switchToFollower("")
				return
			}
		case hb := <-n.HeartBeats:
			fmt.Println(hb)
			if stepDown := n.handleHeartBeat(hb); stepDown {
				n.switchToFollower(hb.Leader)
				return
			}

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
		case vreq := <-n.VoteRequests:
			if shouldReturn := n.handleVoteRequest(vreq); shouldReturn {
				return
			}
		case hb := <-n.HeartBeats:
			if n.leader == "" {
				n.setLeader(hb.Leader)
			}
			if stepDown := n.handleHeartBeat(hb); stepDown {
				n.setLeader(hb.Leader)
			}

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
		case vreq := <-n.VoteRequests:
			if stepDown := n.handleVoteRequest(vreq); stepDown {
				n.switchToFollower("")
				return
			}
		case hb := <-n.HeartBeats:
			if stepDown := n.handleHeartBeat(hb); stepDown {
				n.switchToFollower(hb.Leader)
				return
			}

		}

	}

}

// ReceiveVoteResponse recevie voteresponse
func (n *Node) ReceiveVoteResponse(context context.Context, vresp *protocol.VoteResponse) (*protocol.Response, error) {
	n.VoteResponse <- vresp
	return &protocol.Response{}, nil
}

func (n *Node) sendVoteResponse(peer string, vresp *protocol.VoteResponse) {
	conn, err := grpc.Dial(peer, []grpc.DialOption{grpc.WithTimeout(500 * time.Millisecond), grpc.WithInsecure()}...)
	if err != nil {
		logger.Error(
			"grpc dial",
			zap.String("addr", peer),
			zap.String("err", err.Error()),
		)
		return
	}
	defer conn.Close()
	c := protocol.NewRaftClient(conn)
	c.ReceiveVoteResponse(context.Background(), vresp)
	return

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

func (n *Node) setLeader(leader string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.leader = leader
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

//ReceiveVoteRequest grpc
func (n *Node) ReceiveVoteRequest(context context.Context, vreq *protocol.VoteRequest) (*protocol.Response, error) {
	n.VoteRequests <- vreq
	return &protocol.Response{}, nil
}

func (n *Node) waitOnLoopFinish() {
	q := make(chan struct{})
	n.quit <- q
	<-q
}

func (n *Node) wonElection(votes int) bool {
	return votes >= quorumNeeded(len(n.peers)+1)
}

//NewNode ....
func NewNode(handler *ChanHandler, peers []string, ip, logPath string, port int) (*Node, error) {
	n := &Node{
		ip:      ip,
		state:   FOLLOWER,
		peers:   peers,
		handler: handler,
	}
	n.quit = make(chan chan struct{})
	n.HeartBeats = make(chan *protocol.HeartbeatRequest)
	n.VoteRequests = make(chan *protocol.VoteRequest)
	n.VoteResponse = make(chan *protocol.VoteResponse)
	n.id = fmt.Sprintf("%s:%d", ip, port)
	n.electTimer = time.NewTimer(randElectionTimeout())
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	n.logPath = logPath
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
