package s3imageserver

import (
	"bytes"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/pkg/errors"
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
	stdErr := &bytes.Buffer{}

	command := make([]string, len(pg.Command))

	// copy the slice as we are running a concurrent threads and Command is shared
	copy(command, pg.Command)

	cmd := exec.Command(command[0], append(command[1:], tempPath)...)
	cmd.Stdout = stdOut
	cmd.Stderr = stdErr

	err = cmd.Run()
	if err != nil {
		return nil, errors.Wrap(err, string(stdErr.Bytes()))
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
