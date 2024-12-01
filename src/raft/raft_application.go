package raft

// 等待日志应用条件变量的信号
// 1. if Leader: 通过 append entries reply 计算到的 commitIndex 的变化触发
// 2. if Follower: 收到 leader 的 AppendEntries RPC 更新 commitIndex 时触发
func (rf *Raft) applyTicker() {
	for !rf.killed() {
		rf.mu.Lock()
		rf.applyCond.Wait() // 会释放锁，等待信号量
		// 1. 收集所有需要 apply 的日志
		entries := make([]LogEntry, 0)
		snapPendingApply := rf.snapPending
		if !snapPendingApply {
			if rf.lastApplied < rf.logs.snapshotLastIdx {
				rf.lastApplied = rf.logs.snapshotLastIdx
			}

			// make sure that all entries are in the rf.logs.tailLogs
			start := rf.lastApplied + 1
			end := rf.commitIndex
			if end >= rf.logs.size() {
				end = rf.logs.size() - 1
			}
			for i := start; i <= end; i++ {
				entries = append(entries, rf.logs.at(i))
			}
		}
		rf.mu.Unlock()

		if !snapPendingApply {
			for i, entry := range entries {
				rf.applyCh <- ApplyMsg{
					CommandValid: entry.CommandValid,
					Command:      entry.Command,
					CommandIndex: rf.lastApplied + i + 1,
				}
			}
		} else {
			rf.applyCh <- ApplyMsg{
				SnapshotValid: true,
				Snapshot:      rf.logs.snapshot,
				SnapshotIndex: rf.logs.snapshotLastIdx,
				SnapshotTerm:  rf.logs.snapshotLastTerm,
			}
		}

		rf.mu.Lock()
		if !snapPendingApply {
			LOG(rf.me, rf.currentTerm, DApply, "Apply log［%d, %d］", rf.lastApplied+1, rf.lastApplied+len(entries))
			rf.lastApplied += len(entries)
		} else {
			LOG(rf.me, rf.currentTerm, DApply, "Apply snapshot for [0, %d]", rf.logs.snapshotLastIdx)
			// Leader 发送的 snapshot 一定是 committed 的，follower 可以直接应用
			rf.lastApplied = rf.logs.snapshotLastIdx
			if rf.commitIndex < rf.lastApplied {
				rf.commitIndex = rf.lastApplied
			}
			rf.snapPending = false
		}
		rf.mu.Unlock()
	}
}
