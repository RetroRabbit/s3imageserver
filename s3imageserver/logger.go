package s3imageserver

import (
	"log"
	"net/http"
	"time"

	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

type responsewriterwrapper struct {
	wraps   http.ResponseWriter
	counter int
}

func (rwr *responsewriterwrapper) Header() http.Header {
	return rwr.wraps.Header()
}

func (rwr *responsewriterwrapper) Write(b []byte) (int, error) {
	rwr.counter += len(b)
	return rwr.wraps.Write(b)
}

func (rwr *responsewriterwrapper) WriteHeader(i int) {
	rwr.wraps.WriteHeader(i)
}

type HttpTimer struct {
	wraps http.Handler
}

func (ht *HttpTimer) log(from string, t time.Duration, size int) {
	conn, err := sql.Open("sqlite3", "imageServer.db")
	if err != nil {
		log.Println("SQL Open error -> ", err)
    return
	}
	_, err = conn.Exec("Insert into times (url,time) values ( ? , ? )", from, t)
	if err != nil {
		log.Println("SQL Insert error -> ", err)
	}
}

func (ht *HttpTimer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rwr := &responsewriterwrapper{w, 0}
	ht.wraps.ServeHTTP(rwr, r)
	ht.log(r.URL.String(), time.Since(start), rwr.counter)
}
