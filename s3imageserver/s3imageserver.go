package s3imageserver

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"reflect"

	"database/sql"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/julienschmidt/httprouter"
	_ "github.com/mattn/go-sqlite3"
	"github.com/twinj/uuid"
)

type Config struct {
	Handlers        []HandlerConfig `json:"handlers"`
	HTTPPort        int             `json:"http_port"`
	HTTPSEnabled    bool            `json:"https_enabled"`
	HTTPSStrict     bool            `json:"https_strict"`
	HTTPSPort       int             `json:"https_port"`
	HTTPSCert       string          `json:"https_cert"`
	HTTPSKey        string          `json:"https_key"`
	Database        string          `json:"database"`
	CallbackEnabled bool            `json:"callback_enabled"`
}

type HandlerConfig struct {
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
	AWS    struct {
		AWSAccess  string `json:"aws_access"`
		AWSSecret  string `json:"aws_secret"`
		BucketName string `json:"bucket_name"`
		FilePath   string `json:"file_path"`
	} `json:"aws"`
	Facebook             bool     `json:"facebook"`
	FacebookLegacy       bool     `json:"facebook_lecagy"`
	FacebookGraph        bool     `json:"facebook_graph"`
	GoogleGraph          bool     `json:"google_graph"`
	ErrorImage           string   `json:"error_image"`
	Allowed              []string `json:"allowed_formats"`
	OutputFormat         string   `json:"output_format"`
	CachePath            string   `json:"cache_path"`
	CacheTime            *int     `json:"cache_time"`
	CacheEnabled         *bool    `json:"cache_enabled"`
	DefaultWidth         *int     `json:"default_width"`
	DefaultHeight        *int     `json:"default_height"`
	DefaultQuality       *int     `json:"default_quality"`
	DefaultDontCrop      bool     `json:"default_dont_crop"`
	WifiQuality          *int     `json:"wifi_quality"`
	VerificationRequired *bool    `json:"verification_required"`
}

type HandleVerification func(string) bool

func Run(verify HandleVerification) (done *sync.WaitGroup, callback chan CallEvent) {
	uuid.Init()
	envArg := flag.String("c", "config.json", "Configuration")
	flag.Parse()
	content, err := ioutil.ReadFile(*envArg)
	if err != nil {
		log.Println("Error:", err)
		os.Exit(1)
	}
	var conf Config
	err = json.Unmarshal(content, &conf)
	if err != nil {
		log.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Println("Initializing...")
	fmt.Println("HTTPS_Enabled:", conf.HTTPSEnabled)
	if conf.HTTPSEnabled {
		fmt.Println("Port:", conf.HTTPSPort)
		fmt.Println("Strict:", conf.HTTPSStrict)
	} else {
		fmt.Println("Port:", conf.HTTPPort)
	}
	fmt.Println("Database:", conf.Database)
	databaseInit(conf)
	var callbackChan chan CallEvent = nil
	if conf.CallbackEnabled {
		callbackChan = make(chan CallEvent)
	}
	fmt.Println("CallbackEnabled:", conf.CallbackEnabled, callbackChan == nil)

	r := httprouter.New()
	for _, handle := range conf.Handlers {
		handler := handle
		prefix := handler.Name
		if handler.Prefix != "" {
			prefix = handler.Prefix
		}
		r.GET("/"+prefix+"/*param", func(writer http.ResponseWriter, r *http.Request, ps httprouter.Params) {
			w := reflect.ValueOf(writer).Interface().(*ResponseWriter)
			param := strings.TrimPrefix(ps.ByName("param"), "/")
			cleanURL(r)
			i, err := NewImage(w, r, handler, param)
			i.ErrorImage = handler.ErrorImage
			if err == nil && (verify == nil || !*handler.VerificationRequired || verify(r.URL.Query().Get("t"))) {
				i.getImage(w, r, handler.AWS.AWSAccess, handler.AWS.AWSSecret, handler.Facebook, handler.FacebookLegacy, handler.FacebookGraph, handler.GoogleGraph)
			} else {
				if err != nil {
					log.Println(r.URL.String())
					log.Println(err.Error())
				}
				i.getErrorImage(w)
				w.WriteHeader(404)
				w.Header().Set("Content-Length", strconv.Itoa(len(i.Image)))
			}
			i.write(w)
		})
	}
	r.GET("/alive", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		w.WriteHeader(200)
	})
	if conf.Database != "" {
		r.GET("/backup.db", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
			if verify == nil || verify(r.URL.Query().Get("t")) {
				f, err := os.Open(conf.Database)
				if err != nil {
					w.WriteHeader(404)
					return
				}
				file, err := ioutil.ReadAll(f)
				if err != nil {
					log.Println(err)
					ferr := f.Close()
					if ferr != nil {
						log.Println(ferr)
					}
					w.WriteHeader(404)
					return
				}
				ferr := f.Close()
				if ferr != nil {
					log.Println(ferr)
				}
				w.Header().Set("Content-Length", strconv.Itoa(len(file)))
				w.Write(file)
			} else {
				w.WriteHeader(503)
			}
		})
	}

	wg := &sync.WaitGroup{}
	if conf.validateHTTPS() {
		config := tls.Config{
			MinVersion:               tls.VersionTLS10,
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
				tls.TLS_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
				tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA},
		}
		hot := http.Server{
			Addr:      ":" + strconv.Itoa(conf.HTTPSPort),
			Handler:   &HttpTimer{r, conf, callbackChan},
			TLSConfig: &config,
		}
		wg.Add(1)
		go func() {
			log.Fatal(hot.ListenAndServeTLS(conf.HTTPSCert, conf.HTTPSKey))
			wg.Done()
		}()
	} else {
		wg.Add(1)
		go func() {
			HTTPPort := ":80"
			if conf.HTTPPort != 0 {
				HTTPPort = ":" + strconv.Itoa(conf.HTTPPort)
			}
			log.Println("Starting on port ", HTTPPort)
			if conf.HTTPSStrict && conf.HTTPSEnabled {
				log.Fatal(http.ListenAndServe(HTTPPort, &HttpTimer{http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					http.Redirect(w, req, "https://"+req.Host+req.RequestURI, http.StatusMovedPermanently)
				}), conf, callbackChan}))
			} else {
				log.Fatal(http.ListenAndServe(HTTPPort, &HttpTimer{r, conf, callbackChan}))
			}
			wg.Done()
		}()
	}
	return wg, callbackChan
}

func databaseInit(conf Config) {
	if conf.Database != "" {
		conn, err := sql.Open("sqlite3", conf.Database)
		if err != nil {
			log.Println("SQL Open error -> ", err)
			return
		}
		_, err = conn.Exec("CREATE TABLE IF NOT EXISTS \"request_actions\" ( `id` TEXT NOT NULL UNIQUE, `requestId` TEXT NOT NULL, `action` TEXT, `result` TEXT, PRIMARY KEY(`id`) )")
		if err != nil {
			log.Println("SQL Create Table error -> ", err)
		}
		_, err = conn.Exec("CREATE TABLE IF NOT EXISTS \"requests\" ( `id` TEXT NOT NULL UNIQUE, `url` TEXT NOT NULL, `startTime` INTEGER DEFAULT 0, `endTime` INTEGER DEFAULT 0, `size` INTEGER DEFAULT 0, `type` INTEGER DEFAULT 0, `s3Size`	INTEGER DEFAULT 0, PRIMARY KEY(`id`) )")
		if err != nil {
			log.Println("SQL Create Table error -> ", err)
		}
		conn.Close()
	}
}

func cleanURL(r *http.Request) {
	query := strings.SplitN(r.URL.String(), "?", 2)
	queryString := query[0]
	if len(query) > 1 {
		queryString = queryString + "?" + strings.Replace(query[1], "?", "&", -1)
	}
	url, _ := url.ParseRequestURI(queryString)
	r.URL = url
}

func (c *Config) validateHTTPS() bool {
	if c.HTTPSEnabled && c.HTTPSKey != "" && c.HTTPSCert != "" && c.HTTPSPort != 0 && c.HTTPSPort != c.HTTPPort {
		return true
	}
	c.HTTPSEnabled = false
	return false
}
