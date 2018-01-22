package kvstore

import (
	"runtime"
	"strconv"
	"sync"
	"testing"
)

func checkMapCall(t *testing.T, ok bool, reply, expected MapReply) {
	if !ok {
		t.Fatalf("call error")
	}
	for k, v := range expected.Value {
		if v1, ok := reply.Value[k]; !ok || v1 != v {
			t.Fatalf("wrong reply <%v, %v>; expected <%v, %v>", k, v1, k, v)
			return
		}
	}
	for k, v := range reply.Value {
		if v1, ok := expected.Value[k]; !ok || v1 != v {
			t.Fatalf("wrong reply <%v, %v>; expected <%v, %v>", k, v, k, v1)
			return
		}
	}
}

func checkCall(t *testing.T, ok bool, reply, expected Reply) {
	if !ok {
		t.Fatalf("call error")
	} else if reply != expected {
		t.Fatalf("wrong reply %v; expected %v", reply, expected)
	}
}

func TestBasic(t *testing.T) {
	srvAddr := "localhost:9091"
	ts := StartTinyStore(srvAddr)
	defer ts.Kill()

	client := NewClient(srvAddr)

	ok, reply := client.Get("key1")
	checkCall(t, ok, reply, Reply{Flag: false, Value: ""})

	ok, reply = client.Put("key1", "1")
	checkCall(t, ok, reply, Reply{Flag: false, Value: "1"})

	ok, reply = client.Get("key1")
	checkCall(t, ok, reply, Reply{Flag: true, Value: "1"})

	ok = client.Del("key1")
	ok, reply = client.Get("key1")
	checkCall(t, ok, reply, Reply{Flag: false, Value: ""})

	ok, reply = client.SIsMember("set1", "key1")
	checkCall(t, ok, reply, Reply{Flag: false, Value: ""})

	ok, reply = client.SAdd("set1", "key1")
	checkCall(t, ok, reply, Reply{Flag: false, Value: ""})

	ok, reply = client.SIsMember("set1", "key1")
	checkCall(t, ok, reply, Reply{Flag: true, Value: ""})

	ok, reply = client.SIsMember("set1", "key2")
	checkCall(t, ok, reply, Reply{Flag: false, Value: ""})

	ok = client.SDel("set1")
	ok, reply = client.SIsMember("set1", "key2")
	checkCall(t, ok, reply, Reply{Flag: false, Value: ""})

	ok, reply = client.HSet("hash1", "key1", "1")
	checkCall(t, ok, reply, Reply{Flag: false, Value: "1"})

	ok, reply = client.HGet("hash1", "key1")
	checkCall(t, ok, reply, Reply{Flag: true, Value: "1"})

	ok, reply = client.HSet("hash1", "key1", "2")
	checkCall(t, ok, reply, Reply{Flag: true, Value: "2"})

	ok, reply = client.HIncr("hash1", "key1", 1)
	checkCall(t, ok, reply, Reply{Flag: true, Value: "3"})

	ok, reply = client.HIncr("hash1", "key2", 10)
	checkCall(t, ok, reply, Reply{Flag: false, Value: "10"})

	ok, mapReply := client.HGetAll("hash1")
	checkMapCall(t, ok, mapReply, MapReply{Flag: true, Value: map[string]string{"key1": "3", "key2": "10"}})

	ok = client.HDel("hash1", "key1")
	ok, reply = client.HGet("hash1", "key1")
	checkCall(t, ok, reply, Reply{Flag: false, Value: ""})

	ok = client.HDelAll("hash1")
	ok, mapReply = client.HGetAll("hash1")
	checkMapCall(t, ok, mapReply, MapReply{Flag: false, Value: nil})
}

func TestConcurrent(t *testing.T) {
	runtime.GOMAXPROCS(4)

	srvAddr := "localhost:9090"
	ts := StartTinyStore(srvAddr)
	defer ts.Kill()
	// StartTinyStore(srvAddr)

	client := NewClient(srvAddr)

	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, reply := client.Put("key"+strconv.Itoa(i), "1")
			checkCall(t, ok, reply, Reply{Flag: false, Value: "1"})
		}(i)
	}
	wg.Wait()

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, reply := client.Get("key" + strconv.Itoa(i))
			checkCall(t, ok, reply, Reply{Flag: true, Value: "1"})
		}(i)
	}
	wg.Wait()

	for i := 0; i < 5; i++ {
		for j := 0; j < 1000; j++ {
			wg.Add(2)
			go func(i, j int) {
				defer wg.Done()
				ok, reply := client.SAdd("set"+strconv.Itoa(i), "member"+strconv.Itoa(j))
				checkCall(t, ok, reply, Reply{Flag: false, Value: ""})
			}(i, j)
			go func(i, j int) {
				defer wg.Done()
				ok, reply := client.HSet("hash"+strconv.Itoa(i), "field"+strconv.Itoa(j), strconv.Itoa(j))
				checkCall(t, ok, reply, Reply{Flag: false, Value: strconv.Itoa(j)})
			}(i, j)
		}
	}
	wg.Wait()

	for i := 0; i < 5; i++ {
		for j := 0; j < 1000; j++ {
			wg.Add(2)
			go func(i, j int) {
				defer wg.Done()
				ok, reply := client.SIsMember("set"+strconv.Itoa(i), "member"+strconv.Itoa(j))
				checkCall(t, ok, reply, Reply{Flag: true, Value: ""})
			}(i, j)
			go func(i, j int) {
				defer wg.Done()
				ok, reply := client.HGet("hash"+strconv.Itoa(i), "field"+strconv.Itoa(j))
				checkCall(t, ok, reply, Reply{Flag: true, Value: strconv.Itoa(j)})
			}(i, j)
		}
	}
	wg.Wait()

	for i := 0; i < 5; i++ {
		for j := 1000; j < 2000; j++ {
			wg.Add(2)
			go func(i, j int) {
				defer wg.Done()
				ok, reply := client.SIsMember("set"+strconv.Itoa(i), "member"+strconv.Itoa(j))
				checkCall(t, ok, reply, Reply{Flag: false, Value: ""})
			}(i, j)
			go func(i, j int) {
				defer wg.Done()
				ok, reply := client.HGet("hash"+strconv.Itoa(i), "field"+strconv.Itoa(j))
				checkCall(t, ok, reply, Reply{Flag: false, Value: ""})
			}(i, j)
		}
	}
	wg.Wait()

	for i := 0; i < 5; i++ {
		for j := 0; j < 1000; j++ {
			wg.Add(1)
			go func(i, j int) {
				defer wg.Done()
				ok, reply := client.HIncr("hash"+strconv.Itoa(i), "field"+strconv.Itoa(j), j)
				checkCall(t, ok, reply, Reply{Flag: true, Value: strconv.Itoa(j * 2)})
			}(i, j)
		}
		for j := 1000; j < 2000; j++ {
			wg.Add(1)
			go func(i, j int) {
				defer wg.Done()
				ok, reply := client.HIncr("hash"+strconv.Itoa(i), "field"+strconv.Itoa(j), j)
				checkCall(t, ok, reply, Reply{Flag: false, Value: strconv.Itoa(j)})
			}(i, j)
		}
	}
	wg.Wait()

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, mapReply := client.HGetAll("hash" + strconv.Itoa(i))
			expectedMapValue := make(map[string]string)
			for j := 0; j < 1000; j++ {
				expectedMapValue["field"+strconv.Itoa(j)] = strconv.Itoa(j * 2)
			}
			for j := 1000; j < 2000; j++ {
				expectedMapValue["field"+strconv.Itoa(j)] = strconv.Itoa(j)
			}
			checkMapCall(t, ok, mapReply, MapReply{Flag: true, Value: expectedMapValue})
		}(i)
	}
	wg.Wait()
}
