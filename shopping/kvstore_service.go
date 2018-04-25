package shopping 

import(
	"rush-shopping/kv"
	"sync"
	"strconv"
	"net/rpc"
	"log"
	"net"
	"sync/atomic"
	//"fmt"
)

type ShoppingKVStore struct{
	*kv.KVStore
	RwLock sync.RWMutex
}

func NewShoppingKVStore() *ShoppingKVStore {
	sks := &ShoppingKVStore{KVStore: kv.NewKVStore()}
	return sks
}

type ShoppingKVStoreService struct{
	*ShoppingKVStore
	network string
	addr string
	l net.Listener
}

func NewShoppingKVStoreService(network,addr string) *ShoppingKVStoreService{
	log.Printf("Start kvstore service on %s\n",addr)
	service :=&ShoppingKVStoreService{ShoppingKVStore:NewShoppingKVStore(),network:network,addr:addr}
	return service
}

func (service *ShoppingKVStoreService) Serve(){
	rpcs:=rpc.NewServer()
	rpcs.Register(service)
	l,e:=net.Listen(service.network,service.addr)
	if e!=nil{
		log.Fatal("listen error:",e)
	}
	service.l=l
	go func() {
		for !service.IsDead() {
			if conn, err := l.Accept(); err == nil {
				if !service.IsDead() {
					// concurrent processing
					go rpcs.ServeConn(conn)
				} else if err == nil {
					conn.Close()
				}
			} else {
				if !service.IsDead() {
					log.Fatalln(err.Error())
				}
			}
		}
	}()
}
func (ks *ShoppingKVStoreService) IsDead() bool {
	return atomic.LoadInt32(&ks.Dead) != 0
}

// Clear the data and close the kvstore service.
func (ks *ShoppingKVStoreService) Kill() {
	log.Println("Kill the kvstore")
	atomic.StoreInt32(&ks.Dead, 1)
	ks.Data = nil
	if err := ks.l.Close(); err != nil {
		log.Fatal("Kvsotre rPC server close error:", err)
	}
}
 
func (sks *ShoppingKVStore) SubmitOrder(args *SubmitOrderArgs, reply *int) error{
	//cartKey := getCartKey(args.CartIDStr, args.UserToken)
	orderKey := OrderKeyPrefix + args.UserToken
	num, cartDetail := parseCartValue(args.CartValue)
	*reply= OK
	sks.RwLock.Lock()
	defer sks.RwLock.Unlock()
	for itemID, itemCnt := range cartDetail {
		itemsStockKey := ItemsStockKeyPrefix + strconv.Itoa(itemID)
		if Value, existed := sks.Data[itemsStockKey]; existed {
			iValue, _ := strconv.Atoi(Value)
			if iValue < itemCnt{
				*reply=OutOfStock 
				return nil
			}
		}
	}
	if _,existed:=sks.Data[orderKey];existed{
		*reply=OrderOutOfLimit
		return nil
	}
	price:=0
	for itemID, itemCnt := range cartDetail {
		itemsStockKey := ItemsStockKeyPrefix + strconv.Itoa(itemID)
		itemsPriceKey:=ItemsPriceKeyPrefix+strconv.Itoa(itemID)
		if Value, existed := sks.Data[itemsStockKey]; existed {
			iValue, _ := strconv.Atoi(Value)
			newValue:=strconv.Itoa(iValue-itemCnt)
			sks.Data[itemsStockKey]=newValue
		}
		itemprice,_:=sks.Data[itemsPriceKey]
		iprice,_:=strconv.Atoi(itemprice)
		price+=itemCnt*iprice
	}
	sks.Data[orderKey]=composeOrderValue(false, price, num, cartDetail)
	return nil
}

func (sks *ShoppingKVStore) PayOrder(args *PayOrderArgs, reply *int) error{
	balanceKey := BalanceKeyPrefix + args.UserToken
	rootBalanceKey := BalanceKeyPrefix + RootUserToken
	orderKey := OrderKeyPrefix + args.OrderIDStr
	*reply=OK
	sks.RwLock.Lock()
	defer sks.RwLock.Unlock()
	orderValue:=sks.Data[orderKey]
	hasPaid, price, num, detail := parseOrderValue(orderValue)
	if hasPaid {
		*reply= OrderPaid
		return nil
	}

	if value,existed:=sks.Data[balanceKey];existed{
		iValue,_:=strconv.Atoi(value)
		if iValue<args.Delta{
			*reply=BalanceInsufficient
			return nil
		}else{
			newValue:=strconv.Itoa(iValue-args.Delta)
			sks.Data[balanceKey]=newValue
		}
	}
	if value,existed:=sks.Data[rootBalanceKey];existed{
		iValue,_:=strconv.Atoi(value)
		newValue:=strconv.Itoa(iValue+args.Delta)
		sks.Data[rootBalanceKey]=newValue
	}
	newOrderValue := composeOrderValue(true, price, num, detail)
	sks.Data[orderKey]=newOrderValue
	return nil
}
