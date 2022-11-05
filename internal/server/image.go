package server

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
)

func parseImage(r *http.Request) (imageBase64 []byte, err error) {
	f, fh, err := r.FormFile("image")
	if err != nil {
		if err == http.ErrMissingFile {
			return nil, nil
		}
		return nil, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading image: %w", err)
	}
	title := fh.Filename
	b2, err := webP(b, title)
	if err != nil {
		return nil, fmt.Errorf("converting image to webp: %w", err)
	}
	imageBase64 = []byte(base64.StdEncoding.EncodeToString(b2))
	return
}

// webP should be used in the kuuf-library server to encode uploaded jpg/png images
func webP(b []byte, title string) ([]byte, error) {
	// TODO: stream b to cwebp command.  As of 2022, this is not possible.
	f, err := os.CreateTemp("", title)
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	n := f.Name()
	if err := os.WriteFile(n, b, 0664); err != nil { // HACKY
		return nil, fmt.Errorf("writing image to temporary file: %w", err)
	}
	defer os.Remove(n)
	cmd := exec.Command("cwebp", n, "-o", "-")
	b2, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running cwebp: %w", err)
	}
	return b2, nil
}
