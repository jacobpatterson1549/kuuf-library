package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	maxImageWidth  = 256
	maxImageHeight = 256
)

func faviconBase64() string {
	r := strings.NewReader(faviconSVG)
	var sb strings.Builder
	enc := base64.NewEncoder(base64.StdEncoding, &sb)
	r.WriteTo(enc)
	enc.Close()
	return sb.String()
}

func parseImage(ctx context.Context, r *http.Request) (imageBase64 []byte, err error) {
	f, fh, err := r.FormFile("image")
	if err != nil {
		if err == http.ErrMissingFile {
			return nil, nil
		}
		return nil, err
	}
	if maxSize := int64(10_000_000); fh.Size > maxSize { // 10mb
		return nil, fmt.Errorf("file to large (%v), max size the server will process is %v bytes", fh.Size, maxSize)
	}
	title := fh.Filename
	contentType := fh.Header.Get("Content-Type")
	return convertImage(ctx, f, title, contentType)
}

// imageNeedsUpdating checks to see if the image needs to be updated with the following criteria:
// - it is not empty, AND:
// - it is not a valid base64 string
// - it does not have a valid webp header
// - it does not have a max width/height or the other dimension is too large
func imageNeedsUpdating(imageBase64 string) bool {
	if len(imageBase64) == 0 {
		return false
	}
	sr := strings.NewReader(imageBase64)
	dec := base64.NewDecoder(base64.StdEncoding, sr)
	cfg, err := webp.DecodeConfig(dec)
	if err != nil {
		return true
	}
	switch {
	case cfg.Width == maxImageWidth && cfg.Height <= maxImageHeight,
		cfg.Height == maxImageHeight && cfg.Width <= maxImageWidth:
		return false
	}
	return true
}

func updateImage(ctx context.Context, imageBase64 string, id string) ([]byte, error) {
	sr := strings.NewReader(imageBase64)
	r := base64.NewDecoder(base64.StdEncoding, sr)
	title, contentType := id+"_converted", "image/webp"
	return convertImage(ctx, r, title, contentType)
}

func convertImage(ctx context.Context, r io.Reader, title, contentType string) ([]byte, error) {
	img, err := readImage(r, contentType)
	if err != nil {
		return nil, fmt.Errorf("reading image: %w", err)
	}
	img = scaleImage(img)
	b2, err := webP(ctx, img, title)
	if err != nil {
		return nil, fmt.Errorf("converting image to webp: %w", err)
	}
	imageBase64 := base64.StdEncoding.EncodeToString(b2)
	return []byte(imageBase64), nil
}

func readImage(r io.Reader, contentType string) (image.Image, error) {
	switch contentType {
	case "image/jpeg":
		return jpeg.Decode(r)
	case "image/png":
		return png.Decode(r)
	case "image/webp":
		return webp.Decode(r)
	}
	return nil, fmt.Errorf("unknown image type: %q", contentType)
}

// scaleImages scales the image up/down to fit in a square
func scaleImage(img image.Image) image.Image {
	srcR := img.Bounds()
	boundsR := image.Rect(0, 0, maxImageWidth, maxImageHeight)
	destR := scaleRect(srcR, boundsR)
	destImg := image.NewRGBA(destR)
	var s = draw.CatmullRom
	s.Scale(destImg, destR, img, srcR, draw.Over, nil)
	return destImg
}

func scaleRect(srcR, boundsR image.Rectangle) image.Rectangle {
	srcW, srcH := srcR.Dx(), srcR.Dy()
	boundsW, boundsH := boundsR.Dx(), boundsR.Dy()
	scaleW := float64(srcW) / float64(boundsW)
	scaleH := float64(srcH) / float64(boundsH)
	scale := scaleW
	if scaleW < scaleH {
		scale = scaleH
	}
	destW := int(float64(srcW) / scale)
	destH := int(float64(srcH) / scale)
	destR := image.Rect(0, 0, destW, destH)
	return destR
}

// webP should be used in the kuuf-library server to encode uploaded jpg/png images
func webP(ctx context.Context, img image.Image, title string) ([]byte, error) {
	// It would be nice if the image bytes could be streamed to the cwebp command.
	// As of 2022, this is not possible, a file must be provided.
	f, err := os.CreateTemp(".", title)
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	n := f.Name()
	if err2 := png.Encode(f, img); err2 != nil {
		return nil, fmt.Errorf("writing image to temporary file: %w", err)
	}
	defer os.Remove(n)
	cmd := exec.CommandContext(ctx, "cwebp", n, "-lossless", "-o", "-")
	b2, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running cwebp: %w", err)
	}
	return b2, nil
}
