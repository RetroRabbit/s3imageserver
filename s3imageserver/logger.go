package s3imageserver

import (
	"log"
	"net/http"
	"time"
  "github.com/twinj/uuid"

	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

type ResponseWriter struct {
	wraps   http.ResponseWriter
	counter int
  id      uuid.Uuid
}

func (rwr *ResponseWriter) Header() http.Header {
	return rwr.wraps.Header()
}

func (rwr *ResponseWriter) log(v ...interface{}) {
  log.Println(v)
}

func (rwr *ResponseWriter) Write(b []byte) (int, error) {
	rwr.counter += len(b)
	return rwr.wraps.Write(b)
}

func (rwr *ResponseWriter) WriteHeader(i int) {
	rwr.wraps.WriteHeader(i)
}

type HttpTimer struct {
	wraps http.Handler
  conf    Config
}

func (ht *HttpTimer) log(from string, t time.Duration, size int) {
  if ht.conf.Database != "" {
  	conn, err := sql.Open("sqlite3", ht.conf.Database)
  	if err != nil {
  		log.Println("SQL Open error -> ", err)
      return
  	}
  	_, err = conn.Exec("Insert into times (url,time) values ( ? , ? )", from, t)
  	if err != nil {
  		log.Println("SQL Insert error -> ", err)
  	}
  }
}

func (ht *HttpTimer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
  id := uuid.NewV4()
	rwr := &ResponseWriter{w, 0, id}
	ht.wraps.ServeHTTP(rwr, r)
	ht.log(r.URL.String(), time.Since(start), rwr.counter)
}
