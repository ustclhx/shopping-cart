package main

import(
	"./shopping"
	//"fmt"
	"log"
	//"net"
	"os"
	"flag"
	"runtime/pprof"
)

func main(){
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	sks:= shopping.NewShoppingKVStoreService("tcp",":8000")
	sks.Serve()
	shopping.InitService("tcp",":10000",":8000","./data/users.csv","./data/items.csv")
	block := make(chan bool)
	<-block
}
