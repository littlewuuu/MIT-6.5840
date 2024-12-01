package raft

import (
	"6.5840/labgob"
	"bytes"
	"fmt"
)

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).
func (rf *Raft) persist() {
	// Your code here (3C).
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	rf.logs.persist(e)
	raftstate := w.Bytes()
	rf.persister.Save(raftstate, rf.logs.snapshot) // snapshot 和其他的持久化信息是分开保存的
	LOG(rf.me, rf.currentTerm, DPersist, "Persist: %v", rf.persistString())
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (3C).
	// Example:
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)

	if d.Decode(&rf.currentTerm) != nil || d.Decode(&rf.votedFor) != nil {
		LOG(rf.me, rf.currentTerm, DPersist, "Failed to decode persistent state")
	}

	if err := rf.logs.readPersist(d); err != nil {
		LOG(rf.me, rf.currentTerm, DPersist, "Read log error: %v", err)
		return
	}
	rf.logs.snapshot = rf.persister.ReadSnapshot()
	LOG(rf.me, rf.currentTerm, DPersist, "Read from persist: %v", rf.persistString())

	// as snapshot and other persist states are stored separately
	// it may read updated snapshot while outdated logs
	if rf.logs.snapshotLastIdx > rf.commitIndex {
		rf.commitIndex = rf.logs.snapshotLastIdx
		rf.lastApplied = rf.logs.snapshotLastTerm
	}
}

func (rf *Raft) persistString() string {
	return fmt.Sprintf("T%d, VotedFor: %d, Log: [0: %d)", rf.currentTerm, rf.votedFor, rf.logs.size())
}
