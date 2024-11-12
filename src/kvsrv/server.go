package kvsrv

import (
	"log"
	"strconv"
	"sync"
)

const Debug = false

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.Printf(format, a...)
	}
	return
}

type preReq struct {
	reqSeq uint64
	value  string
}

type KVServer struct {
	mu sync.Mutex
	// Your definitions here.
	storage map[string]string
	// 因为这里假设每个客户端在上一个请求的响应之前不会发出新的请求，所以只需要记录每个客户端的最新请求即可
	histories map[int64]preReq
}

func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	// Your code here.
	kv.mu.Lock()
	defer kv.mu.Unlock()
	reply.Value = kv.storage[args.Key]
}

func (kv *KVServer) Put(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	clientId := args.ClintId
	seq := args.ReqSeq

	kv.mu.Lock()
	defer kv.mu.Unlock()

	res, ok := kv.histories[clientId]
	if ok { // 重复请求
		if seq <= res.reqSeq {
			return
		}
	}
	kv.storage[args.Key] = args.Value
	kv.histories[clientId] = preReq{reqSeq: seq, value: args.Value}
}

func (kv *KVServer) Append(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	clientId := args.ClintId
	seq := args.ReqSeq

	kv.mu.Lock()
	defer kv.mu.Unlock()

	res, ok := kv.histories[clientId]
	if ok {
		// 重复请求
		// client 的当前请求未收到响应，但 server 已经处理过该请求，此时 client 重发请求，返回的 value 才有意义，
		// 也就是说 seq == res.reqSeq 时，返回的 value 才有意义
		if seq == res.reqSeq {
			reply.Value = res.value
			return
		}
		// 如果 seq < res.reqSeq，说明 client 的 seq 请求已经处理完成了，也就是说 server 已经处理过 client seq 之后的请求了，
		// 这时返回 value 是没有意义的，因为无论如何 client 都会丢弃这个响应
		if seq < res.reqSeq {
			return
		}
	}
	oldVal := kv.storage[args.Key]
	kv.storage[args.Key] = oldVal + args.Value
	reply.Value = oldVal
	kv.histories[clientId] = preReq{reqSeq: seq, value: oldVal}
}

func (kv *KVServer) Finish(args *PutAppendArgs, reply *PutAppendReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if res, ok := kv.histories[args.ClintId]; ok {
		if args.ReqSeq == res.reqSeq {
			delete(kv.histories, args.ClintId)
		}
	}
}

func StartKVServer() *KVServer {
	kv := new(KVServer)

	// You may need initialization code here.
	kv.storage = make(map[string]string)
	kv.histories = make(map[int64]preReq)
	return kv
}

func encode(clientId int64, seq uint64) string {
	return strconv.FormatInt(clientId, 10) + strconv.FormatUint(seq, 10)
}
