package s3imageserver

import (
	"log"
	"net/http"
	"time"
  "fmt"
  "github.com/twinj/uuid"

	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

type ResponseWriter struct {
	wraps   http.ResponseWriter
	counter int
  id      uuid.Uuid
  conf    Config
}

type RequestType int

const (
	UNKNOWN RequestType = iota
	CACHED
	GENERATE
)

func (rwr *ResponseWriter) Header() http.Header {
	return rwr.wraps.Header()
}

func (rwr *ResponseWriter) log(v ...interface{}) {
  rwr.loga("", v)
}

func (rwr *ResponseWriter) loga(action string, v ...interface{}) {
  log.Println(action, " -> ", fmt.Sprint(v...))
  if rwr.conf.Database != "" {
  	conn, err := sql.Open("sqlite3", rwr.conf.Database)
  	if err != nil {
  		log.Println("SQL Open error -> ", err)
      return
  	}
		_, err = conn.Exec("INSERT INTO request_actions (id, requestId, action, result) VALUES ( ? , ? , ? , ? )", uuid.NewV4().String(), rwr.id, action, fmt.Sprint(v...))
  	if err != nil {
  		log.Println("SQL Insert error -> ", err)
  	}
    conn.Close()
  }
}

func (rwr *ResponseWriter) Write(b []byte) (int, error) {
	rwr.counter += len(b)
	return rwr.wraps.Write(b)
}

func (rwr *ResponseWriter) WriteHeader(i int) {
	rwr.wraps.WriteHeader(i)
}

func (rwr *ResponseWriter) setS3Size(size int) {
  if rwr.conf.Database != "" {
  	conn, err := sql.Open("sqlite3", rwr.conf.Database)
  	if err != nil {
  		log.Println("SQL Open error -> ", err)
      return
  	}
    query := fmt.Sprintf("UPDATE requests set s3Size = %v where id like \"%v\"", size, rwr.id)
    _, err = conn.Exec(query)
  	if err != nil {
  		log.Println("SQL Insert error -> ", err)
  	}
    conn.Close()
  }
}

func (rwr *ResponseWriter) updateType(rt RequestType) {
  if rwr.conf.Database != "" {
  	conn, err := sql.Open("sqlite3", rwr.conf.Database)
  	if err != nil {
  		log.Println("SQL Open error -> ", err)
      return
  	}
    query := fmt.Sprintf("UPDATE requests set type = %v where id like \"%v\"", rt, rwr.id)
    _, err = conn.Exec(query)
  	if err != nil {
  		log.Println("SQL Insert error -> ", err)
  	}
    conn.Close()
  }
}

type HttpTimer struct {
	wraps http.Handler
  conf    Config
}

func (ht *HttpTimer) recordRequest(id uuid.Uuid, url string, from time.Time) {
  if ht.conf.Database != "" {
  	conn, err := sql.Open("sqlite3", ht.conf.Database)
  	if err != nil {
  		log.Println("SQL Open error -> ", err)
      return
  	}
    _, err = conn.Exec("INSERT INTO requests (id, url, startTime) VALUES ( ? , ? , ? )", id, url, from.Unix())
  	if err != nil {
  		log.Println("SQL Insert error -> ", err)
  	}
    conn.Close()
  }
}

func (ht *HttpTimer) completeRequest(id uuid.Uuid, to time.Time, size int) {
  if ht.conf.Database != "" {
  	conn, err := sql.Open("sqlite3", ht.conf.Database)
  	if err != nil {
  		log.Println("SQL Open error -> ", err)
      return
  	}
    query := fmt.Sprintf("UPDATE requests set endTime = %v , size = %v where id like \"%v\"", to.Unix(), size, id)
    _, err = conn.Exec(query)
  	if err != nil {
  		log.Println("SQL Insert error -> ", err)
  	}
    conn.Close()
  }
}

func (ht *HttpTimer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
  id := uuid.NewV4()
  start := time.Now()
	if (r.URL.String() != "/stat") {
	  ht.recordRequest(id, r.URL.String(), start.UTC())
	}
	rwr := &ResponseWriter{w, 0, id, ht.conf}
	ht.wraps.ServeHTTP(rwr, r)
	if (r.URL.String() != "/stat") {
		ht.completeRequest(id, time.Now().UTC(), rwr.counter)
	}
}
