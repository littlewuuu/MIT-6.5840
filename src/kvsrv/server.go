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

type KVServer struct {
	mu sync.Mutex
	// Your definitions here.
	storage map[string]string

	histories map[string]string
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
	enc := encode(clientId, seq)

	kv.mu.Lock()
	defer kv.mu.Unlock()

	res := kv.histories[enc]
	if res != "" { //
		reply.Value = res
		return
	}
	kv.storage[args.Key] = args.Value
	kv.histories[enc] = args.Value
}

func (kv *KVServer) Append(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	clientId := args.ClintId
	seq := args.ReqSeq
	enc := encode(clientId, seq)

	kv.mu.Lock()
	defer kv.mu.Unlock()

	res := kv.histories[enc]
	if res != "" { //  fixme 不能使用空字符串判断请求是否重复，因为第一次请求后，放入 histories 的也是空字符串
		println("dub res", res)
		reply.Value = res
		return
	}
	oldVal := kv.storage[args.Key]
	kv.storage[args.Key] = oldVal + args.Value
	reply.Value = oldVal
	println("oldVal", oldVal, "new Value", args.Value)
	kv.histories[enc] = oldVal
}

func (kv *KVServer) Finish(args *PutAppendArgs, reply *PutAppendReply) {
	enc := encode(args.ClintId, args.ReqSeq)
	kv.mu.Lock()
	delete(kv.histories, enc)
	kv.mu.Unlock()
}

func StartKVServer() *KVServer {
	kv := new(KVServer)

	// You may need initialization code here.
	kv.storage = make(map[string]string)
	kv.histories = make(map[string]string)
	return kv
}

func encode(clientId int64, seq uint64) string {
	return strconv.FormatInt(clientId, 10) + strconv.FormatUint(seq, 10)
}
