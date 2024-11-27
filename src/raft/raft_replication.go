package raft

import "time"

type AppendEntriesArgs struct {
	Term     int
	LeaderId int
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

	if args.Term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DLog2, "<-%d, Reject append log, higher term T%d < T%d", args.LeaderId, args.Term, rf.currentTerm)
		return
	}

	if args.Term > rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DLog, "AppendEntries Handler: Become follower")
		rf.becomeFollowerLocked(args.Term)
	}
	reply.Success = true
	rf.resetElectionTimerLocked()
}

func (rf *Raft) startReplication(term int) bool {
	replicateToPeer := func(peer int, args *AppendEntriesArgs) {
		reply := &AppendEntriesReply{}
		ok := rf.sendAppendEntriesRPC(peer, args, reply)

		rf.mu.Lock()
		defer rf.mu.Unlock()
		if !ok {
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Lost or crashed", peer)
		}

		// 对齐 term
		if reply.Term > rf.currentTerm {
			rf.becomeFollowerLocked(reply.Term)
			return
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
			args := &AppendEntriesArgs{
				rf.currentTerm,
				rf.me,
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
