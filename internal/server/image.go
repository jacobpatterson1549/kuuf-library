package server

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"

	x_draw "golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

func parseImage(r *http.Request) (imageBase64 []byte, err error) {
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
	img, err := readImage(f, contentType)
	if err != nil {
		return nil, fmt.Errorf("reading image: %w", err)
	}
	img = scaleImage(img)
	b2, err := webP(img, title)
	if err != nil {
		return nil, fmt.Errorf("converting image to webp: %w", err)
	}
	imageBase64 = []byte(base64.StdEncoding.EncodeToString(b2))
	return
}

func readImage(r io.Reader, contentType string) (image.Image, error) {
	var (
		img image.Image
		err error
	)
	switch contentType {
	case "image/jpeg":
		img, err = jpeg.Decode(r)
	case "image/png":
		img, err = png.Decode(r)
	case "image/webp":
		img, err = webp.Decode(r)
	default:
		return nil, fmt.Errorf("unknown image type: %q", contentType)
	}
	if err != nil {
		return nil, fmt.Errorf("decoding image: %w", err)
	}
	return img, nil
}

// scaleImages scales the image up/down to fit in a square
func scaleImage(img image.Image) image.Image {
	srcR := img.Bounds()
	const maxW, maxH = 256, 256
	boundsR := image.Rect(0, 0, maxW, maxH)
	destR := scaleRect(srcR, boundsR)
	destImg := image.NewRGBA(destR)
	var s = x_draw.BiLinear
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
func webP(img image.Image, title string) ([]byte, error) {
	// TODO: stream b to cwebp command.  As of 2022, this is not possible.
	f, err := os.CreateTemp("", title)
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	n := f.Name()
	if err := png.Encode(f, img); err != nil { // HACKY
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
