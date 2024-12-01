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
	Term          int
	Success       bool
	ConflictIndex int
	ConflictTerm  int
}

func (rf *Raft) sendAppendEntriesRPC(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

// AppendEntries RPC handler
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	LOG(rf.me, rf.currentTerm, DVote, "Received AppendEntries RPC, from S%d, preLogTerm:%d, preLogIndex:%d, len(entries):%d", args.LeaderId, args.PrevLogTerm, args.PrevLogIndex, len(args.Entries))

	reply.Success = false
	reply.Term = rf.currentTerm

	// rules all servers 2
	if args.Term > rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DLog, "AppendEntries Handler: Higher term, become follower")
		rf.becomeFollowerLocked(args.Term)
	}

	// step 1
	if args.Term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DLog2, "<-%d, Reject append log, higher term T%d < T%d", args.LeaderId, args.Term, rf.currentTerm)
		return
	}

	// election timer reset condition 1
	rf.resetElectionTimerLocked()

	// step 2.1
	if rf.logs.size() <= args.PrevLogIndex {
		LOG(rf.me, rf.currentTerm, DLog2, "<-%d, Reject append log, no entry at preLogIndex:%d", args.LeaderId, args.PrevLogIndex)
		reply.ConflictIndex = rf.logs.size()
		reply.ConflictTerm = -1
		return
	}
	if rf.logs.snapshotLastIdx > args.PrevLogIndex { // RPC's PrevLogIndex is already taken into current node's snapshot, this rpc is outdated
		reply.ConflictTerm = rf.logs.snapshotLastTerm
		reply.ConflictIndex = rf.logs.snapshotLastIdx
		LOG(rf.me, rf.currentTerm, DLog2, "<- S%d, Reject append log, already in snapshot, follower log truncated in %d", args.LeaderId, rf.logs.snapshotLastIdx)
		return
	}

	// step 2.2
	log := rf.logs.at(args.PrevLogIndex)
	if log.Term != args.PrevLogTerm {
		LOG(rf.me, rf.currentTerm, DLog2, "<-%d, log at preLogIndex:%d has different T%d than T%d", args.LeaderId, args.PrevLogIndex, log.Term, args.PrevLogTerm)
		reply.ConflictTerm = rf.logs.at(args.PrevLogIndex).Term
		// find first log's index whose term == rf.logs[args.PrevLogIndex].Term,
		reply.ConflictIndex = rf.logs.firstFor(reply.ConflictTerm)
		return

		// wrong way for finding reply.ConflictIndex: first log's index may in snapshot
		//idx := args.PrevLogIndex
		//for idx > 0 && rf.logs.at(idx).Term == reply.ConflictTerm {
		//	idx--
		//}
		//reply.ConflictIndex = idx + 1

	}

	// step 3,4
	reply.Success = true
	idx := args.PrevLogIndex + 1
	for _, entry := range args.Entries {
		if rf.logs.size() <= idx {
			rf.logs.append(entry)
		} else if rf.logs.at(idx).Term != entry.Term { // step 3
			rf.logs.appendFrom(idx-1, []LogEntry{entry})
		}
		idx++
	}
	rf.persist()
	LOG(rf.me, rf.currentTerm, DLog2, "<- S%d, Append log success, (%d,%d], leader CommitIndex:%d", args.LeaderId, args.PrevLogIndex, args.PrevLogIndex+len(args.Entries), args.LeaderCommit)

	// step 5
	if args.LeaderCommit > rf.commitIndex {
		if args.LeaderCommit > rf.logs.size()-1 {
			LOG(rf.me, rf.currentTerm, DApply, "Follower update commitIndex from %d to %d", rf.commitIndex, rf.logs.size()-1)
			rf.commitIndex = rf.logs.size() - 1
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

		// 防止 sendAppendEntriesRPC 出现 data racing
		rf.mu.Lock()
		contextLost := rf.contextLostLocked(Leader, term)
		rf.mu.Unlock()
		if contextLost {
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Context Lost, T%d:Leader->T%d:%s", peer, term, rf.currentTerm, rf.role)
			return
		}

		ok := rf.sendAppendEntriesRPC(peer, args, reply)

		rf.mu.Lock()
		defer rf.mu.Unlock()
		if rf.contextLostLocked(Leader, term) {
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Context Lost, T%d:Leader->T%d:%s", peer, term, rf.currentTerm, rf.role)
			return
		}

		if !ok {
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Lost or crashed", peer)
			// note: must return
			return
		}

		// 对齐 term
		if reply.Term > rf.currentTerm {
			rf.becomeFollowerLocked(reply.Term)
			return
		}

		// handle reply
		if !reply.Success { // AppendEntries failed, need reset nextIndex[peer]
			oldNextIndex := rf.nextIndex[peer]
			if reply.ConflictTerm != -1 {
				firstIdx := rf.logs.firstFor(reply.ConflictTerm)
				if firstIdx != InvalidIndex {
					rf.nextIndex[peer] = firstIdx
				} else {
					rf.nextIndex[peer] = reply.ConflictIndex
				}
			} else {
				rf.nextIndex[peer] = reply.ConflictIndex
			}
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d failed, set nextIndex from %d to %d", peer, oldNextIndex, rf.nextIndex[peer])
			return
		} else { // success
			rf.matchIndex[peer] = args.PrevLogIndex + len(args.Entries)
			rf.nextIndex[peer] = rf.matchIndex[peer] + 1

			// update leader commitIndex
			// 找出 matchIndex 中的中位数，作为全局的 commitIndex
			majorityMatched := rf.getMajorityIndexLocked()
			if majorityMatched > rf.commitIndex && rf.logs.at(majorityMatched).Term == rf.currentTerm { // Figure 8
				LOG(rf.me, rf.currentTerm, DApply, "Leader update commitIndex from %d to %d", rf.commitIndex, majorityMatched)
				rf.commitIndex = majorityMatched
				//rf.applyCond.Signal()
				rf.applyCond.Broadcast()
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

			if prevLogIndex < rf.logs.snapshotLastIdx { // 需要先发送一个 snapshot RPC
				args := &InstallSnapshotArgs{
					Term:              term,
					LeaderId:          rf.me,
					LastIncludedIndex: rf.logs.snapshotLastIdx,
					LastIncludedTerm:  rf.logs.snapshotLastTerm,
					Data:              rf.logs.snapshot,
				}
				LOG(rf.me, rf.currentTerm, DDebug, "-> S%d, SendSnap, Args=%v", peer, args.toString())
				go rf.installToPeer(peer, term, args)
				continue
			}

			prevLogTerm := rf.logs.at(prevLogIndex).Term
			args := &AppendEntriesArgs{
				Term:         rf.currentTerm,
				LeaderId:     rf.me,
				PrevLogIndex: prevLogIndex,
				PrevLogTerm:  prevLogTerm,
				Entries:      rf.logs.tailLogs(prevLogIndex + 1),
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
