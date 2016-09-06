package s3imageserver

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"

  "reflect"

	"strconv"
	"sync"
	"strings"
	"net/url"

	"github.com/julienschmidt/httprouter"
  "github.com/twinj/uuid"
)

type Config struct {
	Handlers     []HandlerConfig `json:"handlers"`
	HTTPPort     int             `json:"http_port"`
	HTTPSEnabled bool            `json:"https_enabled"`
	HTTPSStrict  bool            `json:"https_strict"`
	HTTPSPort    int             `json:"https_port"`
	HTTPSCert    string          `json:"https_cert"`
	HTTPSKey     string          `json:"https_key"`
	Database		 string					 `json:"database"`
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
	Facebook		 bool    `json:"facebook"`
	FacebookLegacy		 bool    `json:"facebook_lecagy"`
	ErrorImage   string   `json:"error_image"`
	Allowed      []string `json:"allowed_formats"`
	OutputFormat string   `json:"output_format"`
	CachePath    string   `json:"cache_path"`
	CacheTime    *int     `json:"cache_time"`
	DefaultWidth    	*int     `json:"default_width"`
	DefaultHeight    	*int     `json:"default_height"`
	DefaultQuality    *int     `json:"default_quality"`
	WifiQuality    *int     `json:"wifi_quality"`
	VerificationRequired    *bool     `json:"verification_required"`
}

type HandleVerification func(string) bool

func Run(verify HandleVerification) {
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
			i, err := NewImage(*w, r, handler, param)
			i.ErrorImage = handler.ErrorImage
			if err == nil && (verify == nil || !*handler.VerificationRequired || verify(r.URL.Query().Get("t"))) {
				i.getImage(*w, r, handler.AWS.AWSAccess, handler.AWS.AWSSecret, handler.Facebook, handler.FacebookLegacy)
			} else {
				if err != nil {
					log.Println(r.URL.String())
					log.Println(err.Error())
				}
				i.getErrorImage(*w)
				w.WriteHeader(404)
				w.Header().Set("Content-Length", strconv.Itoa(len(i.Image)))
			}
			i.write(*w)
		})
	}
	r.GET("/stat", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		w.WriteHeader(200)
	})

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
			Handler:   r,
			TLSConfig: &config,
		}
		wg.Add(1)
		go func() {
			log.Fatal(hot.ListenAndServeTLS(conf.HTTPSCert, conf.HTTPSKey))
			wg.Done()
		}()
	}
	wg.Add(1)
	go func() {
		HTTPPort := ":80"
		if conf.HTTPPort != 0 {
			HTTPPort = ":" + strconv.Itoa(conf.HTTPPort)
		}
		if conf.HTTPSStrict && conf.HTTPSEnabled {
			http.ListenAndServe(HTTPPort, &HttpTimer{http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				http.Redirect(w, req, "https://"+req.Host+req.RequestURI, http.StatusMovedPermanently)
			}), conf})
		} else {
			http.ListenAndServe(HTTPPort, &HttpTimer{r, conf})
		}
		wg.Done()
	}()
	wg.Wait()
}

func cleanURL(r *http.Request) {
	query := strings.SplitN(r.URL.String(), "?", 2)
	queryString := query[0]
	if (len(query) > 1) {
		queryString = queryString + "?" + strings.Replace(query[1], "?", "&", -1)
	}
	url,_ := url.ParseRequestURI(queryString)
	r.URL = url
}

func (c *Config) validateHTTPS() bool {
	if c.HTTPSEnabled && c.HTTPSKey != "" && c.HTTPSCert != "" && c.HTTPSPort != 0 && c.HTTPSPort != c.HTTPPort {
		return true
	}
	c.HTTPSEnabled = false
	return false
}
