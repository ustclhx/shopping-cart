package shopping

import(
	"distributed-system/util"
	//"net/rpc"
	"rush-shopping/kv"
)

type clientspool struct{
	pool *util.ResourcePool
}

func NewClientpools(network,addr string, size int) *clientspool{
	pool := util.NewResourcePool(func() util.Resource {
		return util.DialServer(network, addr)
	}, size)
	return &clientspool{pool: pool}
}

func (cp *clientspool) Put(key,value string) (ok bool, reply kv.Reply){
	args:=&kv.PutArgs{Key: key, Value: value}
	ok=util.RPCPoolCall(cp.pool,"ShoppingKVStoreService.RPCPut",args,&reply)
	return
}

func (cp *clientspool) Get(key string) (ok bool, reply kv.Reply){
	args:= &kv.GetArgs{Key: key}
	ok=util.RPCPoolCall(cp.pool,"ShoppingKVStoreService.RPCGet",args,&reply)
	return
}

func (cp *clientspool) Incr(key string,delta int) (ok bool, reply kv.Reply){
	args:= &kv.IncrArgs{Key: key, Delta: delta}
	ok=util.RPCPoolCall(cp.pool,"ShoppingKVStoreService.RPCIncr",args,&reply)
	return
}

func (cp *clientspool) SubmitOrder(CartIDStr,UserToken,CartValue string)(ok bool, reply int){
	args:= &SubmitOrderArgs{CartIDStr:CartIDStr,UserToken:UserToken,CartValue:CartValue}
	ok=util.RPCPoolCall(cp.pool,"ShoppingKVStoreService.SubmitOrder",args,&reply)
	return
}

func (cp *clientspool) PayOrder(OrderIDStr,UserToken string,Delta int)(ok bool, reply int){
	args:=&PayOrderArgs{OrderIDStr:OrderIDStr,UserToken:UserToken,Delta:Delta}
	ok=util.RPCPoolCall(cp.pool,"ShoppingKVStoreService.PayOrder",args,&reply)
	return
}
