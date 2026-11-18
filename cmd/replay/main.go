package main

import (
	"log"
	"order_processor/internal/adapters/miaoshou/web"
	"order_processor/internal/store"
)

func main() {
	db, e := store.Open("data/orders.db")
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	n, e := store.ReplayArchives(db, "data", web.Decoder{})
	if e != nil {
		log.Fatal(e)
	}
	log.Printf("已解析 %d 份原始响应", n)
}
