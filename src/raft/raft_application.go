package raft

// 等待日志应用条件变量的信号
// 1. if Leader: 通过 append entries reply 计算到的 commitIndex 的变化触发
// 2. if Follower: 收到 leader 的 AppendEntries RPC 时触发
func (rf *Raft) applyTicker() {
	for !rf.killed() {
		rf.mu.Lock()
		rf.applyCond.Wait() // 会释放锁，等待信号量
		// 1. 收集所有需要 apply 的日志
		entries := make([]LogEntry, 0)
		for i := rf.lastApplied + 1; i <= rf.commitIndex; i++ {
			entries = append(entries, rf.logs.at(i))
		}
		rf.mu.Unlock()

		for i, entry := range entries {
			rf.applyCh <- ApplyMsg{
				CommandValid: entry.CommandValid,
				Command:      entry.Command,
				CommandIndex: rf.lastApplied + i + 1,
			}
		}
		rf.mu.Lock()
		LOG(rf.me, rf.currentTerm, DApply, "Apply log［%d, %d］", rf.lastApplied+1, rf.lastApplied+len(entries))
		rf.lastApplied += len(entries)
		rf.mu.Unlock()
	}
}
