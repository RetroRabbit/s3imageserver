package s3imageserver

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kr/s3"
	"github.com/pkg/errors"
)

var s3PreviewSourceOnce sync.Once

type S3PreviewConfig struct {
	AWSAccess string   `json:"aws_access"`
	AWSSecret string   `json:"aws_secret"`
	Command   []string `json:"command"`
}

type s3PreviewSource struct {
	S3PreviewConfig
	previewer ThumbnailRenderer
}

type ThumbnailRenderer interface {
	//Gets the filename, file as param, returns an image
	Render(string, io.Reader) (io.ReadCloser, error)
}

//A simple s3 image source, gets the image from s3 and presents as is
func NewS3PreviewSource() func(config S3PreviewConfig) *s3PreviewSource {
	s3PreviewSourceOnce.Do(func() {
		http.DefaultClient.Timeout = 15 * time.Second
	})

	return func(config S3PreviewConfig) *s3PreviewSource {
		return &s3PreviewSource{
			S3PreviewConfig: config,
			previewer:       &PreviewGenerator{config.Command},
		}
	}
}

func (s *s3PreviewSource) GetImage(path string) ([]byte, error) {
	parts := strings.Split(path, "/")
	reqURL := fmt.Sprintf("https://%v.s3.amazonaws.com/%v", parts[1], strings.Join(parts[2:], "/"))
	log.Println("aws request url ", reqURL)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "Could not create request")
	}
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("X-Amz-Acl", "public-read")
	s3.Sign(req, s3.Keys{
		AccessKey: s.AWSAccess,
		SecretKey: s.AWSSecret,
	})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to fetch")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("%v error while making request", resp.StatusCode)
	}

	image, err := s.previewer.Render(parts[len(parts)-1], resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to render thumbnail")
	}
	defer func() { _ = image.Close() }()

	data, err := ioutil.ReadAll(image)

	if err != nil {
		return nil, errors.Wrapf(err, "Error reading Render from %v", req.URL)
	}

	return data, nil
}
