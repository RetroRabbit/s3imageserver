package s3imageserver

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kr/s3"
	"github.com/pkg/errors"
)

type S3Config struct {
	AWSAccess string `json:"aws_access"`
	AWSSecret string `json:"aws_secret"`
}

type s3source struct {
	S3Config
}

var s3SourceOnce sync.Once

//A simple s3 image source, gets the image from s3 and presents as is
func NewS3Source(config S3Config) *s3source {
	s3SourceOnce.Do(func() {
		http.DefaultClient.Timeout = 15 * time.Second
	})

	return &s3source{
		S3Config: config,
	}
}

func (s *s3source) GetImage(path string) (io.ReadCloser, error) {
	parts := strings.Split(path, "/")
	reqURL := fmt.Sprintf("https://%v.s3.amazonaws.com/%v", parts[1], strings.Join(parts[2:], "/"))
	log.Println("aws request url ", reqURL)
	req, reqErr := http.NewRequest("GET", reqURL, nil)
	if reqErr != nil {
		return nil, errors.Wrap(reqErr, "Could not create request")
	}
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("X-Amz-Acl", "public-read")
	s3.Sign(req, s3.Keys{
		AccessKey: s.S3Config.AWSAccess,
		SecretKey: s.S3Config.AWSSecret,
	})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(reqErr, "Failed to fetch")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("%v error while making request", resp.StatusCode)
	}

	return resp.Body, nil
}
