package upload

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"govoting/internal/security"
)

const (
	MaxImageBytes = 3 << 20
	maxPixels     = 32_000_000
)

var ErrInvalidImage = errors.New("file harus berupa gambar JPG, PNG, atau WebP yang valid")

func Save(data []byte, uploadRoot, subdir string, maxDimension int, preserveAlpha bool) (string, error) {
	if len(data) == 0 || len(data) > MaxImageBytes {
		return "", fmt.Errorf("%w: ukuran maksimal 3 MB", ErrInvalidImage)
	}
	contentType := http.DetectContentType(data)
	switch contentType {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return "", ErrInvalidImage
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		return "", ErrInvalidImage
	}
	src, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", ErrInvalidImage
	}
	if maxDimension > 0 && (cfg.Width > maxDimension || cfg.Height > maxDimension) {
		src = resize(src, maxDimension)
	}

	usePNG := preserveAlpha && (format == "png" || format == "webp")
	var encoded bytes.Buffer
	var ext string
	if usePNG {
		ext = ".png"
		if err := png.Encode(&encoded, src); err != nil {
			return "", err
		}
	} else {
		ext = ".jpg"
		dst := image.NewRGBA(src.Bounds())
		draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		draw.Draw(dst, src.Bounds(), src, src.Bounds().Min, draw.Over)
		if err := jpeg.Encode(&encoded, dst, &jpeg.Options{Quality: 88}); err != nil {
			return "", err
		}
	}

	name, err := security.RandomHex(16)
	if err != nil {
		return "", err
	}
	name += ext

	dir := filepath.Join(uploadRoot, subdir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, bytes.NewReader(encoded.Bytes())); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Chmod(0o640); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	final := filepath.Join(dir, name)
	if err := os.Rename(tmpName, final); err != nil {
		return "", err
	}
	return "/uploads/" + subdir + "/" + name, nil
}

func resize(src image.Image, maxDimension int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := float64(maxDimension) / float64(w)
	if float64(maxDimension)/float64(h) < scale {
		scale = float64(maxDimension) / float64(h)
	}
	newW := int(float64(w)*scale + 0.5)
	newH := int(float64(h)*scale + 0.5)
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

func Remove(uploadRoot, webPath string) {
	clean := filepath.Clean(strings.TrimPrefix(webPath, "/uploads/"))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return
	}
	_ = os.Remove(filepath.Join(uploadRoot, clean))
}
