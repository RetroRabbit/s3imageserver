package s3imageserver

import (
	"errors"
	"log"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"strings"
	"net/url"

	"github.com/RetroRabbit/vips"
	"github.com/gosexy/to"
	"github.com/kr/s3"
)

type Image struct {
	Path            string
	FileName        string
	Bucket          string
	Crop            bool
	Debug           bool
	Interlaced      bool
	Height          int
	Width           int
	Image           []byte
	Quality	        int
	CacheTime       int
	CachePath       string
	ErrorImage      string
	ErrorResizeCrop bool
	OutputFormat    vips.ImageType
	Enlarge					bool
}

var allowedTypes = []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}
var allowedMap = map[vips.ImageType]string{vips.WEBP: ".webp", vips.JPEG: ".jpg", vips.PNG: ".png"}

func NewImage(r *http.Request, config HandlerConfig, fileName string) (image *Image, err error) {
	maxDimension := 3064
	height := int(to.Float64(r.URL.Query().Get("h")))
	if height == 0 {
		height = *config.DefaultHeight
	}
	width := int(to.Float64(r.URL.Query().Get("w")))
	if width == 0 {
		width = *config.DefaultWidth
	}
	if height > maxDimension {
		height = maxDimension
	}
	if width > maxDimension {
		width = maxDimension
	}
	enlarge := true
	if r.URL.Query().Get("c") != "" {
		enlarge = to.Bool(r.URL.Query().Get("e"))
	}
	crop := true
	if r.URL.Query().Get("c") != "" {
		crop = to.Bool(r.URL.Query().Get("c"))
	}
	interlaced := true
	if r.URL.Query().Get("i") != "" {
		interlaced = to.Bool(r.URL.Query().Get("i"))
	}
	quality := *config.DefaultQuality
	if (r.URL.Query().Get("p") != "") {
		profile := string(r.URL.Query().Get("p"))
		if (profile == "w" && *config.WifiQuality > 0) {
			quality = *config.WifiQuality
		}
	}
	if r.URL.Query().Get("q") != "" {
		quality = int(to.Float64(r.URL.Query().Get("q")))
	}
	image = &Image{
		Path:            config.AWS.FilePath,
		Bucket:          config.AWS.BucketName,
		Height:          height,
		Crop:            crop,
		Interlaced:			 interlaced,
		Width:           width,
		Quality:				 quality,
		CacheTime:       604800, // cache time in seconds, set 0 to infinite and -1 for disabled
		CachePath:       config.CachePath,
		ErrorImage:      "",
		ErrorResizeCrop: true,
		OutputFormat:    vips.WEBP,
		Enlarge:				 enlarge,
	}
	if config.CacheTime != nil {
		image.CacheTime = *config.CacheTime
	}
	image.isFormatSupported(config.OutputFormat)
	image.isFormatSupported(r.URL.Query().Get("f"))
	acceptedTypes := allowedTypes
	if config.Allowed != nil && len(config.Allowed) > 0 {
		acceptedTypes = config.Allowed
	}
	for _, allowed := range acceptedTypes {
		if len(filepath.Ext(fileName)) == 0 {
			image.FileName = filepath.FromSlash(fileName)
		} else if allowed == filepath.Ext(fileName) {
			image.FileName = filepath.FromSlash(fileName)
		}
	}
	if image.FileName == "" {
		log.Println("PRINT: FileName: " + fileName)
		err = errors.New("File name cannot be an empty string")
	}
	if image.Bucket == "" {
		err = errors.New("Bucket cannot be an empty string")
	}
	return image, err
}

func (i *Image) getImage(w ResponseWriter, r *http.Request, AWSAccess string, AWSSecret string, Facebook bool, FacebookLegacy bool) {
	var err error
	if i.CacheTime > -1 {
		err = i.getFromCache(r)
	} else {
		err = errors.New("Caching disabled")
	}
	if err != nil {
		if (Facebook) {
			err = i.getImageFromFacebook(r, FacebookLegacy);
		} else {
			err = i.getImageFromS3(AWSAccess, AWSSecret)
		}
		if err != nil {
			log.Println("PRINT: ", r.URL.String())
			log.Println("PRINT: ", err)
			err = i.getErrorImage()
			w.WriteHeader(404)
		} else {
			log.Println("PRINT: Error was empty ")
			i.resizeCrop()
			go i.writeCache(r)
		}
	}
	i.write(w)
}

func (i *Image) isFormatSupported(format string) {
	if format != "" {
		format = "." + format
		for v, k := range allowedMap {
			if k == format {
				i.OutputFormat = v
			}
		}
	}
}

func (i *Image) write(w ResponseWriter) {
	w.Header().Set("Content-Length", strconv.Itoa(len(i.Image)))
	w.Write(i.Image)
}

func (i *Image) getErrorImage() (err error) {
	if i.ErrorImage != "" {
		i.Image, err = ioutil.ReadFile(i.ErrorImage)
		if err != nil {
			return err
		}
		if i.ErrorResizeCrop {
			i.resizeCrop()
		}
		return nil
	}
	return errors.New("Error image not specified")
}

func (i *Image) getImageFromFacebook(r *http.Request, legacy bool) (err error) {
	fbUrl := fmt.Sprintf("https://scontent.xx.fbcdn.net/%v", i.FileName)
	if (legacy) {
		fbUrl = fmt.Sprintf("https://scontent.xx.fbcdn.net%v", r.URL.String())
	}
	req, reqErr := http.NewRequest("GET", fbUrl, nil)
	if reqErr != nil {
		log.Println("PRINT: ", r.URL.String())
		log.Println("PRINT: ", reqErr)
		err = reqErr
	} else {
		req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
		req.Header.Set("X-Amz-Acl", "public-read")
		resp, err := http.DefaultClient.Do(req)
		if (err == nil) {
				defer resp.Body.Close()
		}
		if err == nil && resp.StatusCode == http.StatusOK {
			i.Image, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Println("PRINT: ", r.URL.String())
				log.Println("PRINT: ", err)
			} else if i.Debug {
				log.Println("PRINT: Retrieved image from from facebook")
			}
			return nil
		} else if resp.StatusCode != http.StatusOK {
			if !legacy {
				query := strings.Replace(r.URL.String(), "/facebook", "", -1)
				url,_ := url.ParseRequestURI(query)
				r.URL = url
				return i.getImageFromFacebook(r, true)
			} else {
				if (err == nil) {
					return errors.New(fmt.Sprintf("%v error while making request", resp.StatusCode))
				} else {
					log.Println("PRINT: Error while making request")
				}
			}
		}
	}
	return err
}

func (i *Image) getImageFromS3(AWSAccess string, AWSSecret string) (err error) {
	reqURL := fmt.Sprintf("https://%v.s3.amazonaws.com/%v%v", i.Bucket, i.Path, i.FileName)
	req, reqErr := http.NewRequest("GET", reqURL, nil)
	if reqErr != nil {
		log.Println("PRINT: ", reqURL)
		log.Println("PRINT: ", reqErr)
		err = reqErr
	} else {
		req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
		req.Header.Set("X-Amz-Acl", "public-read")
		s3.Sign(req, s3.Keys{
			AccessKey: AWSAccess,
			SecretKey: AWSSecret,
		})
		resp, err := http.DefaultClient.Do(req)
		if (err == nil) {
				defer resp.Body.Close()
		}
		if err == nil && resp.StatusCode == http.StatusOK {
			i.Image, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Println("PRINT: ", reqURL)
				log.Println("PRINT: ", err)
				return err
			} else if i.Debug {
				log.Println("PRINT: Retrieved image from from S3")
			}
			return nil
		} else if resp.StatusCode != http.StatusOK {
			if (err == nil) {
				return errors.New(fmt.Sprintf("%v error while making request", resp.StatusCode))
			} else {
				log.Println("PRINT: %v Error while making request.", resp.StatusCode)
			}
		}
	}
	return err
}

func (i *Image) resizeCrop() {
	options := vips.Options{
		Width:        i.Width,
		Height:       i.Height,
		Crop:         i.Crop,
		Extend:       vips.EXTEND_WHITE,
		Interpolator: vips.BICUBIC,
		Interlaced: 	i.Interlaced,
		Gravity:      vips.CENTRE,
		Quality:      i.Quality,
		Format:       i.OutputFormat,
		Enlarge:			i.Enlarge,
	}
	buf, err := vips.Resize(i.Image, options)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	i.Image = buf
}
