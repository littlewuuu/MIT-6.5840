package kvsrv

import (
	"6.5840/labrpc"
	"sync/atomic"
)
import "crypto/rand"
import "math/big"

type Clerk struct {
	server *labrpc.ClientEnd
	// You will have to modify this struct.
	clintId int64
	reqSeq  atomic.Uint64
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func MakeClerk(server *labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.server = server
	// You'll have to add code here.
	ck.clintId = nrand()
	ck.reqSeq = atomic.Uint64{}
	ck.reqSeq.Add(1)
	return ck
}

// Get fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.server.Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) Get(key string) string {
	// You will have to modify this function.
	args := GetArgs{Key: key, ClintId: ck.clintId, ReqSeq: ck.reqSeq.Load()}
	finishArgs := PutAppendArgs{Key: key, Value: "", ClintId: ck.clintId, ReqSeq: ck.reqSeq.Load()}
	finishReply := PutAppendReply{}
	ck.reqSeq.Add(1)
	reply := GetReply{}

	for !ck.server.Call("KVServer.Get", &args, &reply) {
	}

	for !ck.server.Call("KVServer.Finish", &finishArgs, &finishReply) {
	}
	return reply.Value

}

// PutAppend shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.server.Call("KVServer."+op, &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) PutAppend(key string, value string, op string) string {
	// You will have to modify this function.
	args := PutAppendArgs{Key: key, Value: value, ClintId: ck.clintId, ReqSeq: ck.reqSeq.Load()}
	ck.reqSeq.Add(1)
	reply := PutAppendReply{}
	for !ck.server.Call("KVServer."+op, &args, &reply) {
	}
	finishReply := PutAppendReply{}
	// 如果不发送 Finish 请求，server 总会保留最后一次请求的结果，过不了 test
	for !ck.server.Call("KVServer.Finish", &args, &finishReply) {
	}
	return reply.Value
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}

// Append value to key's value and return that value
func (ck *Clerk) Append(key string, value string) string {
	return ck.PutAppend(key, value, "Append")
}
