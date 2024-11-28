package raft

import (
	"sort"
	"time"
)

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

func (rf *Raft) sendAppendEntriesRPC(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	//LOG(rf.me, rf.currentTerm, DLog, "Send Append Entry to S%d", server)
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	//LOG(rf.me, rf.currentTerm, DLog, "Recv Append Entry Reply from S%d", server)

	return ok
}

// AppendEntries RPC handler
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	LOG(rf.me, rf.currentTerm, DVote, "Received AppendEntries RPC, from S%d,", args.LeaderId)

	reply.Success = false
	reply.Term = rf.currentTerm

	// rules all servers 2
	if args.Term > rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DLog, "AppendEntries Handler: Become follower")
		rf.becomeFollowerLocked(args.Term)
	}

	// step 1
	if args.Term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DLog2, "<-%d, Reject append log, higher term T%d < T%d", args.LeaderId, args.Term, rf.currentTerm)
		return
	}
	rf.resetElectionTimerLocked()

	// step 2
	if len(rf.logs) <= args.PrevLogIndex {
		LOG(rf.me, rf.currentTerm, DLog2, "<-%d, Reject append log, no entry at preLogIndex:%d", args.LeaderId, args.PrevLogIndex)
		return
	}

	// step 3
	log := rf.logs[args.PrevLogIndex]
	if log.Term != args.PrevLogTerm {
		LOG(rf.me, rf.currentTerm, DLog2, "<-%d, log at preLogIndex:%d has different T%d than T%d", args.LeaderId, args.PrevLogIndex, log.Term, args.PrevLogTerm)
		return
	}

	// step 4
	reply.Success = true
	rf.logs = append(rf.logs[:args.PrevLogIndex+1], args.Entries...)
	LOG(rf.me, rf.currentTerm, DLog2, "<- S%d, Append log success, (%d,%d]", args.LeaderId, args.PrevLogIndex, args.PrevLogIndex+len(args.Entries))

	// step 5
	if args.LeaderCommit > rf.commitIndex {
		if args.LeaderCommit > len(rf.logs)-1 {
			LOG(rf.me, rf.currentTerm, DApply, "Follower update commitIndex from %d to %d", rf.commitIndex, len(rf.logs)-1)
			rf.commitIndex = len(rf.logs) - 1
		} else {
			LOG(rf.me, rf.currentTerm, DApply, "Follower update commitIndex from %d to %d", rf.commitIndex, args.LeaderCommit)
			rf.commitIndex = args.LeaderCommit
		}
		rf.applyCond.Signal()
	}

}

func (rf *Raft) startReplication(term int) bool {
	replicateToPeer := func(peer int, args *AppendEntriesArgs) {
		reply := &AppendEntriesReply{}
		ok := rf.sendAppendEntriesRPC(peer, args, reply)

		rf.mu.Lock()
		defer rf.mu.Unlock()
		if rf.contextLostLocked(Leader, term) {
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Context Lost, T%d:Leader->T%d:%s", peer, term, rf.currentTerm, rf.role)
			return
		}

		if !ok {
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Lost or crashed", peer)
		}

		// 对齐 term
		if reply.Term > rf.currentTerm {
			rf.becomeFollowerLocked(reply.Term)
			return
		}

		// handle reply
		if !reply.Success { // AppendEntries failed
			idx := rf.nextIndex[peer] - 1
			if idx > 0 {
				rf.nextIndex[peer] = idx
			}
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d failed, set nextIndex from %d to %d", peer, idx+1, rf.nextIndex[peer])
			return
		} else { //success
			rf.matchIndex[peer] = args.PrevLogIndex + len(args.Entries)
			rf.nextIndex[peer] = rf.matchIndex[peer] + 1

			// update leader commitIndex
			// 找出 matchIndex 中的中位数，作为全局的 commitIndex
			majorityMatched := rf.getMajorityIndexLocked()
			if majorityMatched > rf.commitIndex {
				LOG(rf.me, rf.currentTerm, DApply, "Leader update commitIndex from %d to %d", rf.commitIndex, majorityMatched)
				rf.commitIndex = majorityMatched
				rf.applyCond.Signal()
			}
		}
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.contextLostLocked(Leader, term) {
		LOG(rf.me, rf.currentTerm, DLog, "Context Lost leader[%d] -> %s[T%d]", term, rf.role, rf.currentTerm)
		return false
	}

	for peer := range rf.peers {
		if peer != rf.me {
			prevLogIndex := rf.nextIndex[peer] - 1
			prevLogTerm := rf.logs[prevLogIndex].Term
			args := &AppendEntriesArgs{
				Term:         rf.currentTerm,
				LeaderId:     rf.me,
				PrevLogIndex: prevLogIndex,
				PrevLogTerm:  prevLogTerm,
				Entries:      rf.logs[prevLogIndex+1:],
				LeaderCommit: rf.commitIndex,
			}
			go replicateToPeer(peer, args)
		}
	}
	return true
}

// 只有在当前 term 才能发送心跳
func (rf *Raft) replicationTicker(term int) {
	for !rf.killed() {
		ok := rf.startReplication(term)
		if !ok { // 上下文发生变化，不再是 leader
			break
		}
		time.Sleep(replicateInterval)
	}
}

// find log index to commit
func (rf *Raft) getMajorityIndexLocked() int {
	tmpIndexes := make([]int, len(rf.peers))
	copy(tmpIndexes, rf.matchIndex)
	sort.Ints(sort.IntSlice(tmpIndexes))
	majorityIdx := (len(rf.peers) - 1) / 2
	LOG(rf.me, rf.currentTerm, DDebug, "Match index after sort: %v, majority[%d]=%d", tmpIndexes, majorityIdx, tmpIndexes[majorityIdx])
	return tmpIndexes[majorityIdx]
}