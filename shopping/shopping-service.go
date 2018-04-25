package shopping

// Rush-shopping service based on own http library.
//
// We assume the followings:
// * The IDs of items are increasing from 1 continuously.
// * The ID of the (root) administrator user is 0.
// * The IDs of normal users are increasing from 1 continuously.
// * CartID is auto-increased from 1.
//
// The data format in KV-Store could be referred in
// shop_kvformat.md.

import(
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"distributed-system/http"
	"os"
	"rush-shopping/kv"
	"strconv"
	"strings"
	//"sync"
	"time"
)
//url of API
const (
	LOGIN                 = "/login"
	QUERY_ITEM            = "/items"
	CREATE_CART           = "/carts"
	Add_ITEM              = "/carts/"
	SUBMIT_OR_QUERY_ORDER = "/orders"
	PAY_ORDER             = "/pay"
	QUERY_ALL_ORDERS      = "/admin/orders"
)
// Keys of kvstore
const (
	TokenKeyPrefix      = "token:"
	OrderKeyPrefix      = "order:"
	ItemsStockKeyPrefix = "items_stock:"
	ItemsPriceKeyPrefix = "items_price:"
	BalanceKeyPrefix    = "balance:"

	CartIDMaxKey = "cartID"
	ItemsSizeKey = "items_size"
)

const RootUserID = 0
var RootUserToken = userID2Token(RootUserID)

// Trans status
const (
	OK       = 0
	OutOfStock  = 1
	OrderOutOfLimit = 2
	OrderPaid=3
	BalanceInsufficient =4
)
const (
	OrderPaidFlag   = "P" // have been paid
	OrderUnpaidFlag = "W" // wait to be paid
)


var (
	USER_AUTH_FAIL_MSG       = []byte("{\"code\":\"USER_AUTH_FAIL\",\"message\":\"用户名或密码错误\"}")
	MALFORMED_JSON_MSG       = []byte("{\"code\": \"MALFORMED_JSON\",\"message\": \"格式错误\"}")
	EMPTY_REQUEST_MSG        = []byte("{\"code\": \"EMPTY_REQUEST\",\"message\": \"请求体为空\"}")
	INVALID_ACCESS_TOKEN_MSG = []byte("{\"code\": \"INVALID_ACCESS_TOKEN\",\"message\": \"无效的令牌\"}")
	CART_NOT_FOUND_MSG       = []byte("{\"code\": \"CART_NOT_FOUND\", \"message\": \"篮子不存在\"}")
	CART_EMPTY               = []byte("{\"code\": \"CART_EMPTY\", \"message\": \"购物车为空\"}")
	NOT_AUTHORIZED_CART_MSG  = []byte("{\"code\": \"NOT_AUTHORIZED_TO_ACCESS_CART\",\"message\": \"无权限访问指定的篮子\"}")
	ITEM_OUT_OF_LIMIT_MSG    = []byte("{\"code\": \"ITEM_OUT_OF_LIMIT\",\"message\": \"篮子中物品数量超过了三个\"}")
	ITEM_NOT_FOUND_MSG       = []byte("{\"code\": \"ITEM_NOT_FOUND\",\"message\": \"物品不存在\"}")
	ITEM_OUT_OF_STOCK_MSG    = []byte("{\"code\": \"ITEM_OUT_OF_STOCK\", \"message\": \"物品库存不足\"}")
	ORDER_OUT_OF_LIMIT_MSG   = []byte("{\"code\": \"ORDER_OUT_OF_LIMIT\",\"message\": \"每个用户只能下一单\"}")

	ORDER_NOT_FOUND_MSG      = []byte("{\"code\": \"ORDER_NOT_FOUND\", \"message\": \"篮子不存在\"}")
	NOT_AUTHORIZED_ORDER_MSG = []byte("{\"code\": \"NOT_AUTHORIZED_TO_ACCESS_ORDER\",\"message\": \"无权限访问指定的订单\"}")
	ORDER_PAID_MSG           = []byte("{\"code\": \"ORDER_PAID\",\"message\": \"订单已支付\"}")
	BALANCE_INSUFFICIENT_MSG = []byte("{\"code\": \"BALANCE_INSUFFICIENT\",\"message\": \"余额不足\"}")
)

type ShopServer struct {
	server    *http.Server
	rootToken string

	ClientPool *clientspool 

	// resident memory
	ItemListCache  []Item // real item start from index 1
	//ItemLock       sync.Mutex
	ItemsJSONCache []byte
	UserMap        map[string]UserIDAndPass // map[name]password
	MaxItemID      int                      // The same with the number of types of items.
	MaxUserID      int                      // The same with the number of normal users.
}

const DefaultClientPoolMaxSize = 100

func InitService(network,appAddr,kvstoreAddr,userCsv,itemCsv string) *ShopServer{
	ss := new(ShopServer)
	ss.ClientPool = NewClientpools(network,kvstoreAddr,DefaultClientPoolMaxSize)
	ss.loadUsersAndItems(userCsv, itemCsv)

	ss.server=http.NewServer(appAddr)
	ss.server.AddHandlerFunc(LOGIN, ss.login)
	ss.server.AddHandlerFunc(QUERY_ITEM, ss.queryItem)
	ss.server.AddHandlerFunc(CREATE_CART, ss.createCart)
	ss.server.AddHandlerFunc(Add_ITEM, ss.addItem)
	ss.server.AddHandlerFunc(SUBMIT_OR_QUERY_ORDER, ss.orderProcess)
	ss.server.AddHandlerFunc(PAY_ORDER, ss.payOrder)

	log.Printf("Start shopping service on %s\n", appAddr)
	go func() {
		if err := ss.server.ListenAndServe(); err != nil {
			fmt.Println(err)
		}
	}()
	return ss
}


func (ss *ShopServer) Kill() {
	log.Println("Kill the http server")
	if err := ss.server.Close(); err != nil {
		log.Fatal("Http server close error:", err)
	}
}

//load user and items data to kvstore
func (ss *ShopServer) loadUsersAndItems(userCsv, itemCsv string) {
	log.Println("Load user and item data to kvstore")
	now := time.Now()
	defer func() {
		log.Printf("Finished data loading, cost %v ms\n", time.Since(now).Nanoseconds()/int64(time.Millisecond))
	}()
	ss.ClientPool.Put(CartIDMaxKey, "0")
	ss.ItemListCache = make([]Item, 1, 512)
	ss.ItemListCache[0] = Item{ID: 0}
	
	ss.UserMap = make(map[string]UserIDAndPass)
	// read users
	if file, err := os.Open(userCsv); err == nil {
		reader := csv.NewReader(file)
		for strs, err := reader.Read(); err == nil; strs, err = reader.Read() {
			userID, _ := strconv.Atoi(strs[0])
			ss.UserMap[strs[1]] = UserIDAndPass{userID, strs[2]}
			userToken := userID2Token(userID)
			ss.ClientPool.Put(BalanceKeyPrefix+userToken, strs[3])
			if userID > ss.MaxUserID {
				ss.MaxUserID = userID
			}
		}
		file.Close()
	} else {
		panic(err.Error())
	}
	ss.rootToken = userID2Token(ss.UserMap["root"].ID)
	// read items
	itemCnt := 0
	if file, err := os.Open(itemCsv); err == nil {
		reader := csv.NewReader(file)
		for strs, err := reader.Read(); err == nil; strs, err = reader.Read() {
			itemCnt++
			itemID, _ := strconv.Atoi(strs[0])
			price, _ := strconv.Atoi(strs[1])
			stock, _ := strconv.Atoi(strs[2])
			ss.ItemListCache = append(ss.ItemListCache, Item{ID: itemID, Price: price, Stock: stock})

			ss.ClientPool.Put(ItemsPriceKeyPrefix+strs[0], strs[1])
			ss.ClientPool.Put(ItemsStockKeyPrefix+strs[0], strs[2])

			if itemID > ss.MaxItemID {
				ss.MaxItemID = itemID
			}
		}
		ss.ItemsJSONCache, _ = json.Marshal(ss.ItemListCache[1:])
		ss.ClientPool.Put(ItemsSizeKey, strconv.Itoa(itemCnt))

		file.Close()
	} else {
		panic(err.Error())
	}
	//ss.coordClients.LoadItemList(itemCnt)
}

func (ss *ShopServer) login(resp *http.Response, req *http.Request){
	isEmpty, body := isBodyEmpty(resp, req)
	if isEmpty{
		return
	}
	var user LoginJson
	if err := json.Unmarshal(body, &user); err != nil {
		resp.WriteStatus(http.StatusBadRequest)
		resp.Write(MALFORMED_JSON_MSG)
		return
	}
	userIDAndPass, ok := ss.UserMap[user.Username]
	if !ok || userIDAndPass.Password != user.Password {
		resp.WriteStatus(http.StatusForbidden)
		resp.Write(USER_AUTH_FAIL_MSG)
		return
	}
	userID := userIDAndPass.ID
	token := userID2Token(userID)
	ss.ClientPool.Put(TokenKeyPrefix+token, "1")
	okMsg := []byte("{\"user_id\":" + strconv.Itoa(userID) + ",\"username\":\"" + user.Username + "\",\"access_token\":\"" + token + "\"}")
	resp.WriteStatus(http.StatusOK)
	resp.Write(okMsg)

}

func (ss *ShopServer) queryItem(resp *http.Response, req *http.Request){
	if exist, _ ,_:= ss.authorize(resp, req,  false); !exist {
		return
	}
	resp.WriteStatus(http.StatusOK)
	resp.Write(ss.ItemsJSONCache)
	return
}

func (ss *ShopServer) createCart(resp *http.Response, req *http.Request) {
	var token string
	exist, token,_ := ss.authorize(resp, req, false)
	if !exist {
		return
	}
	_, reply := ss.ClientPool.Incr(CartIDMaxKey, 1)
	cartIDStr := reply.Value

	cartKey := getCartKey(cartIDStr, token)
	_, reply = ss.ClientPool.Put(cartKey, "0")

	resp.WriteStatus(http.StatusOK)
	resp.Write([]byte("{\"cart_id\": \"" + cartIDStr + "\"}"))
	return
}

func (ss *ShopServer) addItem(resp *http.Response, req *http.Request) {
	var token string
	exist, token,body := ss.authorize(resp, req, false)
	if !exist {
		return
	}
	/*isEmpty, body := isBodyEmpty(resp, req)
	if isEmpty {
		return
	}*/
	var item ItemCount
	if err := json.Unmarshal(body, &item); err != nil {
		resp.WriteStatus(http.StatusBadRequest)
		resp.Write(MALFORMED_JSON_MSG)
		return
	}

	if item.ItemID < 1 || item.ItemID > ss.MaxItemID {
		resp.WriteStatus(http.StatusNotFound)
		resp.Write(ITEM_NOT_FOUND_MSG)
		return
	}

	cartIDStr := strings.Split(req.URL.Path, "/")[2]
	cartKey := getCartKey(cartIDStr, token)
	existed, cartValue := ss.checkCartExist(cartIDStr, cartKey, resp, req)
	if !existed {
		return
	}
	num, cartDetail := parseCartValue(cartValue)
	
	// Test whether #items in cart exceeds 3.
	if num+item.Count > 3 {
		resp.WriteStatus(http.StatusForbidden)
		resp.Write(ITEM_OUT_OF_LIMIT_MSG)
		return
	}
	num += item.Count
	// Set the new values of the cart.
	cartDetail[item.ItemID] += item.Count
	ss.ClientPool.Put(cartKey, composeCartValue(num, cartDetail))
	resp.WriteStatus(http.StatusNoContent)
	return
}

func (ss *ShopServer) orderProcess(resp *http.Response, req *http.Request) {
	if req.Method == "POST" {
		ss.submitOrder(resp, req)
		// fmt.Println("submitOrder")
	} else {
		ss.queryOneOrder(resp, req)
		// fmt.Println("queryOneOrder")
	}
}

func (ss *ShopServer) submitOrder(resp *http.Response, req *http.Request) {
	var token string
	exist, token,body := ss.authorize(resp, req,  false)
	if !exist {
		return
	}

	/*isEmpty, body := isBodyEmpty(resp, req)
	if isEmpty {
		return
	}*/

	var cartIDJson CartIDJson

	if err := json.Unmarshal(body, &cartIDJson); err != nil {
		resp.WriteStatus(http.StatusBadRequest)
		resp.Write(MALFORMED_JSON_MSG)
		return
	}
	cartIDStr := cartIDJson.IDStr
	cartKey := getCartKey(cartIDStr, token)
	existed, cartValue := ss.checkCartExist(cartIDStr, cartKey,resp, req)
	if !existed {
		return
	}
	
	// Test whether the cart is empty.
	num, _ := parseCartValue(cartValue)
	if num == 0 {
		resp.WriteStatus(http.StatusForbidden)
		resp.Write(CART_EMPTY)
		return
	}
	_,flag:=ss.ClientPool.SubmitOrder(cartIDStr,token,cartValue)
	switch flag{
	case OK:
		{
			resp.WriteStatus(http.StatusOK)
			resp.Write([]byte("{\"order_id\": \"" + token + "\"}"))
		}
	case OutOfStock:
		{
			resp.WriteStatus(http.StatusForbidden)
			resp.Write(ITEM_OUT_OF_STOCK_MSG)
		}
	case OrderOutOfLimit:
		{
			resp.WriteStatus(http.StatusForbidden)
			resp.Write(ORDER_OUT_OF_LIMIT_MSG)
		}
	}
}

func (ss *ShopServer) payOrder(resp *http.Response, req *http.Request){
	var token string

	exist, token,body := ss.authorize(resp, req,  false)
	if !exist {
		return
	}

	/*isEmpty, body := isBodyEmpty(resp, req)
	if isEmpty {
		return
	}*/
	var orderIDJson OrderIDJson
	if err := json.Unmarshal(body, &orderIDJson); err != nil {
		resp.WriteStatus(http.StatusBadRequest)
		resp.Write(MALFORMED_JSON_MSG)
		return
	}
	
	orderIDStr := orderIDJson.IDStr
	if orderIDStr != token {
		resp.WriteStatus(http.StatusUnauthorized)
		resp.Write(NOT_AUTHORIZED_ORDER_MSG)
		return
	}

	orderKey := OrderKeyPrefix + orderIDStr
	// Test whether the order exists, or it belongs other users.
	_, reply := ss.ClientPool.Get(orderKey)
	if !reply.Flag {
		resp.WriteStatus(http.StatusNotFound)
		resp.Write(ORDER_NOT_FOUND_MSG)
		return
	}
	// Test whether the order have been paid.
	hasPaid, price, _, _ := parseOrderValue(reply.Value)
	if hasPaid {
		resp.WriteStatus(http.StatusForbidden)
		resp.Write(ORDER_PAID_MSG)
		return
	}
	_,flag:=ss.ClientPool.PayOrder(orderIDStr,token,price)
	switch flag {
	case OK:
		{
			resp.WriteStatus(http.StatusOK)
			resp.Write([]byte("{\"order_id\": \"" + token + "\"}"))
		}
	case OrderPaid:
		{
			resp.WriteStatus(http.StatusForbidden)
			resp.Write(ORDER_PAID_MSG)
		}
	case BalanceInsufficient:
		{
			resp.WriteStatus(http.StatusForbidden)
			resp.Write(BALANCE_INSUFFICIENT_MSG)
		}
	}

	return
}

func (ss *ShopServer) queryOneOrder(resp *http.Response, req *http.Request) {
	var token string
	exist, token,_ := ss.authorize(resp, req,  false)
	if !exist {
		return
	}
	var reply kv.Reply
	if _, reply = ss.ClientPool.Get(OrderKeyPrefix + token); !reply.Flag {
		resp.WriteStatus(http.StatusOK)
		resp.Write([]byte("[]"))
		return
	}
	hasPaid, price, _, detail := parseOrderValue(reply.Value)
	var orders [1]Order
	order := &orders[0]
	itemNum := len(detail) // it cannot be zero.
	order.HasPaid = hasPaid
	order.IDStr = token
	order.Items = make([]ItemCount, itemNum)
	order.Total = price
	cnt := 0
	for itemID, itemCnt := range detail {
		if itemCnt != 0 {
			order.Items[cnt].ItemID = itemID
			order.Items[cnt].Count = itemCnt
			cnt++
		}
	}

	body, _ := json.Marshal(orders)
	resp.WriteStatus(http.StatusOK)
	resp.Write(body)
	return
}

const PARSE_BUFF_INIT_LEN = 128
func isBodyEmpty(resp *http.Response, req *http.Request)(bool,[]byte){
	var parseBuff [PARSE_BUFF_INIT_LEN]byte
	var ptr, totalReadN = 0, 0
	ret := make([]byte, 0, PARSE_BUFF_INIT_LEN/2)

	for readN, _ := req.Body.Read(parseBuff[ptr:]); readN != 0; readN, _ = req.Body.Read(parseBuff[ptr:]) {
		totalReadN += readN
		nextPtr := ptr + readN
		ret = append(ret, parseBuff[ptr:nextPtr]...)
		if nextPtr >= PARSE_BUFF_INIT_LEN {
			ptr = 0
		} else {
			ptr = nextPtr
		}
	}

	if totalReadN == 0 {
		resp.WriteStatus(http.StatusBadRequest)
		resp.Write(EMPTY_REQUEST_MSG)
		return true, nil
	}
	return false, ret
}
// return the flag that indicates whether is authroized or not
func  (ss *ShopServer) authorize(resp *http.Response, req *http.Request,isRoot bool)(bool,string,[]byte){
	valid := true
	//var authUserID int
	var authUserIDStr string
	isEmpty, body := isBodyEmpty(resp, req)
	if isEmpty{
		return false,"",nil
	}
	var token AccessTokenJson
	if err:=json.Unmarshal(body, &token); err != nil {
		valid=false
	}
	if token.Token==""{
		valid=false
	}else{
		authUserID,err := strconv.Atoi(token.Token)
		if err!=nil{
			valid = false
		}
		authUserIDStr = strconv.Itoa(authUserID)
		if isRoot && authUserIDStr != ss.rootToken || !isRoot && (authUserID < 1 || authUserID > ss.MaxUserID) {
			valid = false
		} else {
			if _, reply :=ss.ClientPool.Get(TokenKeyPrefix + authUserIDStr); !reply.Flag {
				valid = false
			}
		}
	}
	if !valid {
		resp.WriteStatus(http.StatusUnauthorized)
		resp.Write(INVALID_ACCESS_TOKEN_MSG)
		return false, "",nil
	}
	return true, authUserIDStr,body
}

func (ss *ShopServer) checkCartExist(cartIDStr, cartKey string, resp *http.Response, req *http.Request) ( bool, string) {
	vaild:= true 
	cartID, _ := strconv.Atoi(cartIDStr)
	_, reply := ss.ClientPool.Get(CartIDMaxKey)
	if reply.Flag{
		maxCartID, _ := strconv.Atoi(reply.Value)
		if cartID > maxCartID || cartID < 1 {
			vaild=false
		}
	}else{
		vaild=false
	}
	if !vaild{
		resp.WriteStatus(http.StatusNotFound)
		resp.Write(CART_NOT_FOUND_MSG)
		return false,""
	}
	_, reply = ss.ClientPool.Get(cartKey)
	if !reply.Flag {
		resp.WriteStatus(http.StatusUnauthorized)
		resp.Write(NOT_AUTHORIZED_CART_MSG)
		return false,""
	}
	return true,reply.Value
}
