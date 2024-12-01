package raft

import "fmt"

// Snapshot the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (3D).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if index > rf.commitIndex {
		LOG(rf.me, rf.currentTerm, DSnap, "Couldn't snapshot before CommitIdx: %d>%d", index, rf.commitIndex)
		return
	}

	if index <= rf.logs.snapshotLastIdx {
		LOG(rf.me, rf.currentTerm, DSnap, "Already snapshot in %d<=%d", index, rf.logs.snapshotLastIdx)
		return
	}

	rf.logs.doSnapshot(index, snapshot)
	rf.persist()
}

type InstallSnapshotArgs struct {
	Term              int
	LeaderId          int
	LastIncludedIndex int
	LastIncludedTerm  int
	Data              []byte
}

func (args *InstallSnapshotArgs) toString() string {
	return fmt.Sprintf("Leader-%d, T%d, Last: [%d]T%d", args.LeaderId, args.Term, args.LastIncludedIndex, args.LastIncludedTerm)
}

type InstallSnapshotReply struct {
	Term int
}

func (reply *InstallSnapshotReply) toString() string {
	return fmt.Sprintf("T%d", reply.Term)
}

// leader
func (rf *Raft) sendInstallSnapshotRPC(server int, args *InstallSnapshotArgs, reply *InstallSnapshotReply) bool {
	ok := rf.peers[server].Call("Raft.InstallSnapshot", args, reply)
	return ok
}

// InstallSnapshot RPC handler, follower
func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	LOG(rf.me, rf.currentTerm, DSnap, "<- S%d, RecvSnapshot, Args=%v", args.LeaderId, args.toString())

	reply.Term = rf.currentTerm

	// step 1
	if args.Term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DSnap, "<- S%d, Reject Snap, Higher Term: T%d>T%d", args.LeaderId, rf.currentTerm, args.Term)
		return
	}

	if args.Term >= rf.currentTerm { // = handle the case when the peer is candidate
		rf.becomeFollowerLocked(args.Term)
	}

	// local snapshot already contains the one in RPC
	if rf.logs.snapshotLastIdx >= args.LastIncludedIndex {
		LOG(rf.me, rf.currentTerm, DSnap, "<- S%d, Reject Snap, Already installed: %d>%d", args.LeaderId, rf.logs.snapshotLastIdx, args.LastIncludedIndex)
		return
	}

	// install snapshot
	rf.logs.installSnapshot(args.LastIncludedIndex, args.LastIncludedTerm, args.Data)
	rf.persist()
	rf.snapPending = true
	rf.applyCond.Signal()
}

func (rf *Raft) installToPeer(peer int, term int, args *InstallSnapshotArgs) {
	reply := &InstallSnapshotReply{}
	ok := rf.sendInstallSnapshotRPC(peer, args, reply)

	rf.mu.Lock()
	defer rf.mu.Unlock()
	if !ok {
		LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Lost or crashed", peer)
		return
	}
	LOG(rf.me, rf.currentTerm, DDebug, "-> S%d, SendSnapshot, Reply=%v", peer, reply.toString())

	if reply.Term > rf.currentTerm {
		rf.becomeFollowerLocked(reply.Term)
		return
	}

	if rf.contextLostLocked(Leader, term) {
		LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Context Lost, T%d:Leader->T%d:%s", peer, term, rf.currentTerm, rf.role)
		return
	}

	if args.LastIncludedIndex > rf.matchIndex[peer] {
		rf.matchIndex[peer] = args.LastIncludedIndex
		rf.nextIndex[peer] = rf.matchIndex[peer] + 1
	}

	// note: no need update the commitIndex here
	// because the snapshot already include the commitIndex
}
