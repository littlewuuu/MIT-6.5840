# Lab 3B 

## bugs

### bug1

见 git lab3B: out-TestFailAgree3B.text

### bug2

见 git lab3B: out-TestBackup3B.text

### bug 3

Leader 在发送 AppendEntries RPC 时，超时（call rpc !ok）就要 return

```cod
		if !reply.Success { // AppendEntries failed, need set nextIndex[peer]
			oldNextIndex := rf.nextIndex[peer]
			if reply.ConflictTerm != -1 {
				idx := args.PrevLogIndex
				for idx > 0 && rf.logs[idx].Term != reply.ConflictTerm {
					idx--
				}
				if idx == 0 { // leader does not contain any log with term == reply.ConflictIndex
					rf.nextIndex[peer] = reply.ConflictIndex
				} else {
					rf.nextIndex[peer] = idx + 1
				}
			} else { 
				//否则流程会继续走到这里，reply.ConflictIndex 为默认初始值 0，那下一次发送 AppendEntriesRPC 的时候 preLogIndex = -1, 溢出
				rf.nextIndex[peer] = reply.ConflictIndex
			}

			LOG(rf.me, rf.currentTerm, DLog, "-> S%d failed, set nextIndex from %d to %d", peer, oldNextIndex, rf.nextIndex[peer])
			return
		}
```


## 最终测试结果

```bash
(raft) ~/GolandProjects/6.5840/src/raft$ dstest 3B -p 30 -n 100                                                                                                                  ✹ ✭master 
 Verbosity level set to 0
┏━━━━━━┳━━━━━━━━┳━━━━━━━┳━━━━━━━━━━━━━━┓
┃ Test ┃ Failed ┃ Total ┃         Time ┃
┡━━━━━━╇━━━━━━━━╇━━━━━━━╇━━━━━━━━━━━━━━┩
│ 3B   │      0 │   100 │ 85.56 ± 2.80 │
└──────┴────────┴───────┴──────────────┘
(raft) ~/GolandProjects/6.5840/src/raft$ go test -run 3B -race                                                                                                                   ✹ ✭master 
Test (3B): basic agreement ...
  ... Passed --   1.6  3   18    5410    3
Test (3B): RPC byte count ...
  ... Passed --   4.8  3   50  115862   11
Test (3B): test progressive failure of followers ...
  ... Passed --   5.5  3   84   17928    3
Test (3B): test failure of leaders ...
  ... Passed --   5.6  3  114   26993    3
Test (3B): agreement after follower reconnects ...
  ... Passed --   5.7  3   78   20164    7
Test (3B): no agreement if too many followers disconnect ...
  ... Passed --   4.1  5  156   32350    3
Test (3B): concurrent Start()s ...
  ... Passed --   1.1  3   16    3954    6
Test (3B): rejoin of partitioned leader ...
  ... Passed --   7.3  3  117   31467    4
Test (3B): leader backs up quickly over incorrect follower logs ...
  ... Passed --  45.6  5 2215 1709427  102
Test (3B): RPC counts aren't too high ...
  ... Passed --   2.3  3   30    8666   12
PASS
ok      6.5840/raft     85.311s
```

# Lab 3C

## bugs

### bug1

data racing

fix：见 gitlog

### bug2

收到过期的 AppendEntries RPC

fix：严格遵守 fig2 AppendEntries RPC receiver implementation

### bug3

applyTicker 没有被唤醒

## 测试结果

```bash
(raft) ~/GolandProjects/6.5840/src/raft$ dstest 3C -p 100 -n 1000                                                                                                         1 ↵  ➜ ✹ ✭master 
 Verbosity level set to 0
┏━━━━━━┳━━━━━━━━┳━━━━━━━┳━━━━━━━━━━━━━━━┓
┃ Test ┃ Failed ┃ Total ┃          Time ┃
┡━━━━━━╇━━━━━━━━╇━━━━━━━╇━━━━━━━━━━━━━━━┩
│ 3C   │      0 │  1000 │ 125.05 ± 6.87 │
└──────┴────────┴───────┴───────────────┘

```

