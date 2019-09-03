package s3imageserver

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"

	"net/url"
	"strconv"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

type Config struct {
	SourceConfigs   map[string]json.RawMessage `json:"sources"`
	Routes          []HandlerConfig            `json:"routes"`
	HTTPPort        int                        `json:"http_port"`
	HTTPSEnabled    bool                       `json:"https_enabled"`
	HTTPSStrict     bool                       `json:"https_strict"`
	HTTPSPort       int                        `json:"https_port"`
	HTTPSCert       string                     `json:"https_cert"`
	HTTPSKey        string                     `json:"https_key"`
	Database        string                     `json:"database"`
	CallbackEnabled bool                       `json:"callback_enabled"`
	Defaults        *FormatDefaults            `json:"defaults"`
}

type HandlerConfig struct {
	Route                string          `json:"route"`
	Source               string          `json:"source"`
	ErrorImage           string          `json:"error_image"`
	Allowed              []string        `json:"allowed_formats"`
	VerificationRequired *bool           `json:"verification_required"`
	Defaults             *FormatDefaults `json:"defaults"`
	Rewrite              *RegexRewrite   `json:"rewrite"`
}

type FormatDefaults struct {
	DefaultWidth       *int   `json:"default_width"`
	DefaultHeight      *int   `json:"default_height"`
	DefaultQuality     *int   `json:"default_quality"`
	DefaultDontCrop    bool   `json:"default_dont_crop"`
	DefaultFeatureCrop *bool  `json:"default_feature_crop"`
	WifiQuality        *int   `json:"wifi_quality"`
	DefaultImageFormat string `json:"default_format"`
}

type RegexRewrite struct {
	Match   string `json:"match"`
	Replace string `json:"replace"`
}

type HandleVerification func(string) bool

var Sources = &SourceMap{}

func init() {
	Sources.AddSource("s3", NewS3Source)
	Sources.AddSource("s3Thumb", NewS3PreviewSource())
}

func Run(verify HandleVerification) (done *sync.WaitGroup) {
	done = &sync.WaitGroup{}
	envArg := flag.String("c", "config.json", "Configuration")
	flag.Parse()
	content, err := ioutil.ReadFile(*envArg)
	if err != nil {
		log.Println("Error:", err)
		return
	}
	var conf Config
	err = json.Unmarshal(content, &conf)
	if err != nil {
		log.Println("Error:", err)
		return
	}
	fmt.Println("Initializing...")
	fmt.Println("HTTPS_Enabled:", conf.HTTPSEnabled)
	if conf.HTTPSEnabled {
		fmt.Println("Port:", conf.HTTPSPort)
		fmt.Println("Strict:", conf.HTTPSStrict)
	} else {
		fmt.Println("Port:", conf.HTTPPort)
	}

	r := http.NewServeMux()
	for _, handler := range conf.Routes {
		log.Println("Adding handler", handler.Route)

		if handler.Defaults == nil {
			handler.Defaults = conf.Defaults
		}
		imgSource, err := Sources.GetSource(handler.Source, conf.SourceConfigs[handler.Source])
		if err != nil {
			log.Println("Cannot start handler:", handler.Route, "with source", handler.Source, "due to", err)
			continue
		}

		r.HandleFunc(handler.Route, Handle(imgSource, handler, verify))
	}
	r.HandleFunc("/alive", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

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
		done.Add(1)
		go func() {
			defer done.Done()
			log.Println(hot.ListenAndServeTLS(conf.HTTPSCert, conf.HTTPSKey))
		}()
	}

	HTTPPort := ":80"
	if conf.HTTPPort != 0 {
		HTTPPort = ":" + strconv.Itoa(conf.HTTPPort)
	}

	done.Add(1)
	go func() {
		defer done.Done()
		log.Println("Starting on port ", HTTPPort)
		if conf.HTTPSStrict && conf.HTTPSEnabled {
			log.Println(http.ListenAndServe(HTTPPort, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				http.Redirect(w, req, "https://"+req.Host+req.RequestURI, http.StatusMovedPermanently)
			})))
			return
		} else {
			log.Println(http.ListenAndServe(HTTPPort, r))
			return
		}
	}()

	return done
}

func Handle(source ImageSource, config HandlerConfig, verify HandleVerification) func(w http.ResponseWriter, req *http.Request) {
	var match *regexp.Regexp

	if config.Rewrite != nil {
		match = regexp.MustCompile(config.Rewrite.Match)
	} else {
		log.Println("rewrite is nil for route", config.Route)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		log.Println(config.Route, "Handeling", r)
		//TODO:: This is dodgy AF. it replaces ? with &, impling we get malformed query params
		cleanURL(r)
		if match != nil {
			r.URL.Path = match.ReplaceAllString(r.URL.Path, config.Rewrite.Replace)
		}

		//Get formatting settings
		formatting := GetFormatSettings(r, config.Defaults)

		//GET image from source
		img, err := source.GetImage(r.URL.Path)
		if err != nil {
			log.Printf("GetImage failed for %v with error %+v", r.URL.String(), err)
			//Mssing img
			img, err := ErrorImage(config.ErrorImage, formatting)
			if err != nil {
				log.Printf("Error getting error img %+v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, err = w.Write(img)
			if err != nil {
				log.Printf("Error writing result %+v", err)
			}
			return
		}

		log.Println("Image with size", len(img), err)

		//Resize and/or crop + Present in encoding
		resultImg, err := ResizeCrop(img, formatting)
		if err != nil {
			log.Printf("ResizeCrop failed for %v with error %+v", r.URL.String(), err)
			//Mssing img
			img, err := ErrorImage(config.ErrorImage, formatting)
			if err != nil {
				log.Printf("Error getting error img %+v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, err = w.Write(img)
			if err != nil {
				log.Printf("Error writing result %+v", err)
			}
			return
		}

		w.Header().Set("Content-Length", strconv.Itoa(len(resultImg)))
		_, err = w.Write(resultImg)
		if err != nil {
			log.Printf("Error writing result %+v", err)
		}
	}
}

func ErrorImage(url string, formatting *FormatSettings) ([]byte, error) {
	if url != "" {
		Image, err := ioutil.ReadFile(url)
		if err != nil {
			return nil, err
		}
		return ResizeCrop(Image, formatting)
	}
	return nil, nil
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
