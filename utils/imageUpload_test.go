package utils

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func encodedBrandingImage(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buffer bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buffer, img)
	case "jpeg":
		err = jpeg.Encode(&buffer, img, nil)
	default:
		t.Fatalf("unsupported test format %q", format)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestValidateBrandingImageReadsRealFormatAndDimensions(t *testing.T) {
	tests := []struct {
		format string
		mime   string
	}{
		{format: "png", mime: "image/png"},
		{format: "jpeg", mime: "image/jpeg"},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			content := encodedBrandingImage(t, tt.format, 3, 2)
			got, err := ValidateBrandingImage(bytes.NewReader(content), int64(len(content)+1))
			if err != nil {
				t.Fatal(err)
			}
			if got.Format != tt.format || got.MIME != tt.mime || got.Width != 3 || got.Height != 2 || got.Bytes != int64(len(content)) {
				t.Fatalf("unexpected validation result: %#v", got)
			}
			if !bytes.Equal(got.Content, content) {
				t.Fatal("validated content changed")
			}
		})
	}
}

func TestValidateBrandingImageRejectsNonImageContent(t *testing.T) {
	if _, err := ValidateBrandingImage(strings.NewReader("not an image"), 100); err == nil {
		t.Fatal("expected invalid image error")
	}
}

func TestValidateBrandingImageRejectsOversizedFile(t *testing.T) {
	content := encodedBrandingImage(t, "png", 1, 1)
	if _, err := ValidateBrandingImage(bytes.NewReader(content), int64(len(content)-1)); err == nil {
		t.Fatal("expected file size error")
	}
}

func TestValidateBrandingImageRejectsOversizedDimensions(t *testing.T) {
	content := encodedBrandingImage(t, "png", 5, 2)
	if _, err := validateBrandingImage(bytes.NewReader(content), int64(len(content)+1), 4); err == nil {
		t.Fatal("expected dimension error")
	}
}
