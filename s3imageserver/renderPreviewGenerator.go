package s3imageserver

import (
	"bytes"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
)

type PreviewGenerator struct {
	Command []string
}

func (pg *PreviewGenerator) Render(filename string, file io.Reader) (io.ReadCloser, error) {
	tempPath := path.Join(os.TempDir(), filename)
	log.Println("Temp file at", tempPath)
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(tempFile, file)
	if err != nil {
		return nil, err
	}
	err = tempFile.Close()
	if err != nil {
		return nil, err
	}

	stdOut := &bytes.Buffer{}

	cmd := exec.Command(pg.Command[0], append(pg.Command[1:], tempPath)...)
	cmd.Stdout = stdOut

	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	resultingImg := stdOut.String()
	resultingImg = strings.TrimSpace(resultingImg)
	log.Println("thumbnail at", resultingImg)

	thumbnail, err := os.Open(resultingImg)
	if err != nil {
		return nil, err
	}

	return thumbnail, nil

}
