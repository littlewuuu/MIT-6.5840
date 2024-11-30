package raft

import (
	"math/rand"
	"time"
)

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (3A, 3B).
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (3A).
	Term        int
	VoteGranted bool
}

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
func (rf *Raft) sendRequestVoteRPC(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	//LOG(rf.me, rf.currentTerm, DLog, "Send RequestVote to S%d", server)
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	//LOG(rf.me, rf.currentTerm, DLog, "Recv RequestVote Reply from S%d", server)

	return ok
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (3A, 3B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	LOG(rf.me, rf.currentTerm, DVote, "Received RequestVote RPC, from S%d,", args.CandidateId)

	reply.Term = rf.currentTerm
	reply.VoteGranted = false

	// step 1
	if args.Term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DVote, "From S%d, Reject vote, higher term, T%d>T%d", args.CandidateId, rf.currentTerm, args.Term)
		return
	}

	// fig.2 all servers rules 2
	if args.Term > rf.currentTerm {
		rf.becomeFollowerLocked(args.Term)
	}

	if rf.votedFor != -1 {
		LOG(rf.me, rf.currentTerm, DVote, "-> S%d, Reject, Already voted to S%d", args.CandidateId, rf.votedFor)
		return
	}

	// step 2
	if !rf.isCandidateUptoDate(args.LastLogTerm, args.LastLogIndex) {
		LOG(rf.me, rf.currentTerm, DVote, "Reject vote, S%d's log not up to date", args.CandidateId)
		return
	}
	reply.VoteGranted = true
	rf.votedFor = args.CandidateId
	rf.persist()
	// election timer reset condition 3
	rf.resetElectionTimerLocked()
	LOG(rf.me, rf.currentTerm, DVote, "-> S%d, vote granted", args.CandidateId)
}

func (rf *Raft) electionTicker() {
	for !rf.killed() {
		// Your code here (3A)
		// Check if a leader election should be started.
		rf.mu.Lock()
		if rf.role != Leader && rf.isElectionTimeOut() {
			rf.becomeCandidateLocked()
			go rf.startElection(rf.currentTerm)
		}
		rf.mu.Unlock()

		// pause for a random amount of time between 50 and 350
		// milliseconds.
		ms := 50 + (rand.Int63() % 300)
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
}

// check if candidate's log is at least as up-to-date as rf.me
func (rf *Raft) isCandidateUptoDate(candidateLastLogTerm, candidateLastLogIndex int) bool {
	lastLogIndex, lastLogTerm := rf.logs.last()
	LOG(rf.me, rf.currentTerm, DVote, "Compare last log, Me: [%d]T%d, Candidate: [%d]T%d", lastLogIndex, lastLogTerm, candidateLastLogIndex, candidateLastLogTerm)
	return (candidateLastLogTerm > lastLogTerm) || (candidateLastLogTerm == lastLogTerm && candidateLastLogIndex >= lastLogIndex)
}

func (rf *Raft) resetElectionTimerLocked() {
	LOG(rf.me, rf.currentTerm, DLog, "Reset Election Timer")
	rf.electionStartTime = time.Now()
	randomRange := int64(electionTimeoutMax - electionTimeoutMin)
	rf.electionTimeOut = electionTimeoutMin + time.Duration(rand.Int63()%randomRange) // 当前 term 的超时间隔
}

func (rf *Raft) isElectionTimeOut() bool {
	return time.Since(rf.electionStartTime) > rf.electionTimeOut

}

func (rf *Raft) startElection(term int) {

	votes := 0
	askVoteFromPeer := func(peer int, args *RequestVoteArgs) {
		reply := &RequestVoteReply{}
		ok := rf.sendRequestVoteRPC(peer, args, reply) // do not call RPC with lock

		rf.mu.Lock()
		defer rf.mu.Unlock()

		if !ok {
			LOG(rf.me, rf.currentTerm, DDebug, "Ask vote from S%d, lost or error", peer)
			return
		}

		// 对齐 term
		if reply.Term > rf.currentTerm { // reply.Term 是发送方当前的 term
			rf.becomeFollowerLocked(reply.Term)
		}

		// check context
		if rf.contextLostLocked(Candidate, term) {
			LOG(rf.me, rf.currentTerm, DVote, "Lost Context, abort RequestVoteReply from S%d", peer)
			return
		}

		// 仍然是 Candidate 和 发送 rpc 时的 term
		if reply.VoteGranted {
			votes++
			LOG(rf.me, rf.currentTerm, DVote, "Vote from S%d, total votes %d", peer, votes)
			if votes > len(rf.peers)/2 {
				rf.becomeLeaderLocked()
				go rf.replicationTicker(term)
			}
		}
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()
	LOG(rf.me, rf.currentTerm, DDebug, "start election")

	if rf.contextLostLocked(Candidate, term) {
		LOG(rf.me, rf.currentTerm, DVote, "Lost Candidate to %s, abort RequestVote", rf.role)
		return
	}

	lastLogIdx, lastLogTerm := rf.logs.last()
	for peer := range rf.peers {
		if peer == rf.me {
			votes++
			continue
		}
		args := &RequestVoteArgs{
			Term:         rf.currentTerm,
			CandidateId:  rf.me,
			LastLogIndex: lastLogIdx,
			LastLogTerm:  lastLogTerm,
		}
		go askVoteFromPeer(peer, args)
	}
}
