// nagara はエンジニアが「ながら」で学べる、短い技術動画のプラットフォーム。
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/newbee1939/nagara/internal/filestore"
	"github.com/newbee1939/nagara/internal/web"
)

const maxUpload = 2 << 30 // 2 GB

func main() {
	addr := flag.String("addr", "localhost:8080", "待ち受けるアドレス")
	dataDir := flag.String("data", "data", "動画を保存するディレクトリ")
	flag.Parse()

	store, err := filestore.Open(*dataDir)
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           web.New(store, maxUpload),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("nagara: http://%s", *addr)
	log.Fatal(srv.ListenAndServe())
}
