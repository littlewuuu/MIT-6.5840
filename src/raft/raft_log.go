package raft

import (
	"6.5840/labgob"
	"fmt"
)

type Log struct {
	snapshotLastIdx  int
	snapshotLastTerm int

	// contains log [1, snapshotLastIdx]
	snapshot []byte
	// tailLog[snapshotLastIdx] as dummy head
	// tailLog(snapshotLastIdx, snapshotLastIdx+len(tailLog)-1] for real logs
	tailLog []LogEntry
}

func NewLog(snapshotLastIdx, snapshotLastTerm int, snapshot []byte, entries []LogEntry) *Log {
	log := &Log{
		snapshotLastIdx:  snapshotLastIdx,
		snapshotLastTerm: snapshotLastTerm,
		snapshot:         snapshot,
	}
	log.tailLog = append(log.tailLog, LogEntry{
		Term: snapshotLastTerm,
	})
	log.tailLog = append(log.tailLog, entries...)
	return log
}

// all the functions below should be called under the protection of rf.mutex
// all the functions below should be called under the protection of rf.mutex
func (rl *Log) readPersist(d *labgob.LabDecoder) error {
	var lastIdx int
	if err := d.Decode(&lastIdx); err != nil {
		return fmt.Errorf("decode last include index failed")
	}
	rl.snapshotLastIdx = lastIdx

	var lastTerm int
	if err := d.Decode(&lastTerm); err != nil {
		return fmt.Errorf("decode last include term failed")
	}
	rl.snapshotLastTerm = lastTerm

	var log []LogEntry
	if err := d.Decode(&log); err != nil {
		return fmt.Errorf("decode tail log failed")
	}
	rl.tailLog = log

	return nil
}

func (rl *Log) persist(e *labgob.LabEncoder) {
	e.Encode(rl.snapshotLastIdx)
	e.Encode(rl.snapshotLastTerm)
	e.Encode(rl.tailLog)
}

func (log *Log) size() int {
	return log.snapshotLastIdx + len(log.tailLog)
}

func (log *Log) idx(logicIdx int) int {
	// if the logicIdx fall beyond [snapLastIdx, size()-1]
	if logicIdx < log.snapshotLastIdx || logicIdx >= log.size() {
		panic(fmt.Sprintf("%d is out of [%d, %d]", logicIdx, log.snapshotLastIdx, log.size()-1))
	}
	return logicIdx - log.snapshotLastIdx
}

func (log *Log) at(logicIdx int) LogEntry {
	return log.tailLog[log.idx(logicIdx)]
}

func (log *Log) last() (index, term int) {
	i := len(log.tailLog) - 1
	return log.snapshotLastIdx + i, log.tailLog[i].Term
}

func (log *Log) firstFor(term int) int {
	for idx, entry := range log.tailLog {
		if entry.Term == term {
			return idx + log.snapshotLastIdx
		} else if entry.Term > term {
			break
		}
	}
	return InvalidIndex
}

func (log *Log) tailLogs(startIdx int) []LogEntry {
	if startIdx >= log.size() {
		return nil
	}
	return log.tailLog[log.idx(startIdx):]
}

// mutate methods
func (rl *Log) append(e LogEntry) {
	rl.tailLog = append(rl.tailLog, e)
}

// 保留 logicPrevIndex 位置的 log
func (rl *Log) appendFrom(logicPrevIndex int, entries []LogEntry) {
	rl.tailLog = append(rl.tailLog[:rl.idx(logicPrevIndex)+1], entries...)
}

// string methods for debugging
func (rl *Log) String() string {
	var terms string
	prevTerm := rl.snapshotLastTerm
	prevStart := rl.snapshotLastIdx
	for i := 0; i < len(rl.tailLog); i++ {
		if rl.tailLog[i].Term != prevTerm {
			terms += fmt.Sprintf(" [%d, %d]T%d", prevStart, rl.snapshotLastIdx+i-1, prevTerm)
			prevTerm = rl.tailLog[i].Term
			prevStart = i
		}
	}
	terms += fmt.Sprintf("[%d, %d]T%d", prevStart, rl.snapshotLastIdx+len(rl.tailLog)-1, prevTerm)
	return terms
}

// snapshot in the index
// do checkpoint from the app layer
func (rl *Log) doSnapshot(index int, snapshot []byte) {
	if index <= rl.snapshotLastIdx {
		return
	}

	idx := rl.idx(index)

	rl.snapshotLastIdx = index
	rl.snapshotLastTerm = rl.tailLog[idx].Term
	rl.snapshot = snapshot

	// make a new log array
	newLog := make([]LogEntry, 0, rl.size()-rl.snapshotLastIdx)
	newLog = append(newLog, LogEntry{
		Term: rl.snapshotLastTerm,
	})
	newLog = append(newLog, rl.tailLog[idx+1:]...)
	rl.tailLog = newLog
}

// install snapshot from the raft layer
func (rl *Log) installSnapshot(index, term int, snapshot []byte) {
	rl.snapshotLastIdx = index
	rl.snapshotLastTerm = term
	rl.snapshot = snapshot

	// make a new log array
	newLog := make([]LogEntry, 0, 1)
	newLog = append(newLog, LogEntry{
		Term: rl.snapshotLastTerm,
	})
	rl.tailLog = newLog
}
