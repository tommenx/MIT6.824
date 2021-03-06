package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"fmt"
	"labrpc"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// import "bytes"
// import "encoding/gob"

const (
	FOLLOWER = iota
	CANDIDATE
	LEADER

	HEARTBEAT_INTERVAL    = 100
	MIN_ELECTION_INTERVAL = 400
	MAX_ELECTION_INTERVAL = 500
)

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
//
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	votedFor     int   //vote for the num of server
	voteAcquired int   //the num of vote
	state        int32 //indicate current state
	currentTerm  int32 //indicate current term

	voteCh   chan struct{} //success to vote
	appendCh chan struct{} //success to update log

	electionTimer *time.Timer
}

func (rf *Raft) getTerm() int32 {
	return atomic.LoadInt32(&rf.currentTerm)
}

func (rf *Raft) incrementTerm() int32 {
	return atomic.AddInt32(&rf.currentTerm, 1)
}

func (rf *Raft) isState(state int32) bool {
	return atomic.LoadInt32(&rf.state) == state
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (2A).
	term = int(rf.getTerm())
	isleader = rf.isState(LEADER)
	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := gob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := gob.NewDecoder(r)
	// d.Decode(&rf.xxx)
	// d.Decode(&rf.yyy)
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term        int32
	CandidateId int
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int32
	VoteGranted bool
}

//
// example RequestVote RPC handler.
//
//handle from receiver
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if args.Term < rf.currentTerm {
		reply.VoteGranted = false
		reply.Term = rf.currentTerm
	} else if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.updateStateTo(FOLLOWER)
		rf.votedFor = args.CandidateId
		reply.VoteGranted = true
	} else {
		if rf.votedFor == -1 {
			rf.votedFor = args.CandidateId
			reply.VoteGranted = true
		} else {
			reply.VoteGranted = false
		}
	}
	if reply.VoteGranted == true {
		go func() { rf.voteCh <- struct{}{} }()
	}
}

type AppendEntriesArgs struct {
	Term     int32
	LeaderID int
}

type AppendEntriesReply struct {
	Term    int32
	Success bool
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	//deal with the msg and reply
	if args.Term < rf.currentTerm {
		reply.Success = false
		reply.Term = rf.currentTerm
	} else if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.updateStateTo(FOLLOWER)
		reply.Success = true
	} else {
		reply.Success = true
	}
	go func() {
		rf.appendCh <- struct{}{}
	}()
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).

	return index, term, isLeader
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (2A, 2B, 2C).
	rf.state = FOLLOWER
	rf.votedFor = -1
	rf.voteCh = make(chan struct{})
	rf.appendCh = make(chan struct{})

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	go rf.startLoop()

	return rf
}

func (rf *Raft) updateStateTo(state int32) {
	if rf.isState(state) {
		return
	}
	stateDesc := []string{"FOLLOWER", "CANDIDATE", "LEADER"}
	preState := rf.state
	switch state {
	case FOLLOWER:
		rf.state = FOLLOWER
		rf.votedFor = -1
	case CANDIDATE:
		rf.state = CANDIDATE
		rf.startElection()
	case LEADER:
		rf.state = LEADER
	default:
		fmt.Printf("Warning: invalid state %d, do nothing.\n", state)
	}
	fmt.Printf("In term %d: Server %d transfer from %s to %s\n",
		rf.currentTerm, rf.me, stateDesc[preState], stateDesc[rf.state])

}

func randElectionDuration() time.Duration {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return time.Millisecond * time.Duration(r.Int63n(MAX_ELECTION_INTERVAL-MIN_ELECTION_INTERVAL)+MIN_ELECTION_INTERVAL)
}

func (rf *Raft) startElection() {
	rf.incrementTerm()  //first increase current term
	rf.votedFor = rf.me //vote for self
	rf.voteAcquired = 1 //acquire self's vote
	rf.electionTimer.Reset(randElectionDuration())
	rf.broadcastVoteReq()
}

//send vote request to each server and deal with the reply
//vote for the Id which contained in the vote args
//reply the voteGranted only ture or false
func (rf *Raft) broadcastVoteReq() {
	args := RequestVoteArgs{Term: atomic.LoadInt32(&rf.currentTerm), CandidateId: rf.me}
	for i, _ := range rf.peers {
		if i == rf.me {
			continue
		}
		// is it is candidate then send vote req
		go func(server int) {
			var reply RequestVoteReply
			if rf.isState(CANDIDATE) && rf.sendRequestVote(server, &args, &reply) {
				rf.mu.Lock()
				defer rf.mu.Unlock()
				//deal with the reply
				if reply.VoteGranted {
					rf.voteAcquired += 1
				} else {
					if reply.Term > rf.currentTerm {
						rf.currentTerm = reply.Term
						rf.updateStateTo(FOLLOWER)
					}
				}
			} else {
				fmt.Printf("Server %d send vote req failed.\n", rf.me)
			}
		}(i)
	}
}

func (rf *Raft) broadcastAppendEnrties() {
	args := AppendEntriesArgs{Term: atomic.LoadInt32(&rf.currentTerm), LeaderID: rf.me}
	for i, _ := range rf.peers {
		if i == rf.me {
			continue
		}
		go func(server int) {
			var reply AppendEntriesReply
			if rf.isState(LEADER) && rf.sendAppendEntries(server, &args, &reply) {
				rf.mu.Lock()
				defer rf.mu.Unlock()
				if reply.Success {

				} else {
					if reply.Term > rf.currentTerm {
						rf.currentTerm = reply.Term
						rf.updateStateTo(FOLLOWER)
					}
				}
			}
		}(i)
	}
}

func (rf *Raft) startLoop() {
	rf.electionTimer = time.NewTimer(randElectionDuration())
	for {
		switch atomic.LoadInt32(&rf.state) {
		case FOLLOWER:
			select {
			case <-rf.voteCh:
				rf.electionTimer.Reset(randElectionDuration())
			case <-rf.appendCh:
				rf.electionTimer.Reset(randElectionDuration())
			case <-rf.electionTimer.C:
				//time out
				rf.mu.Lock()
				rf.updateStateTo(CANDIDATE)
				rf.mu.Unlock()
			}
		case CANDIDATE:
			rf.mu.Lock()
			select {
			case <-rf.appendCh:
				rf.updateStateTo(FOLLOWER)
			case <-rf.electionTimer.C:
				rf.electionTimer.Reset(randElectionDuration())
				rf.startElection()
			default:
				if rf.voteAcquired > len(rf.peers)/2 {
					rf.updateStateTo(LEADER)
				}
			}
			rf.mu.Unlock()
		case LEADER:
			rf.broadcastAppendEnrties()
			time.Sleep(HEARTBEAT_INTERVAL * time.Millisecond)
		}

	}
}
