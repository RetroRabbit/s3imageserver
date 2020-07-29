package s3imageserver

import (
	"bytes"
	"net/http"
	"github.com/disintegration/imaging"
	"github.com/gosexy/to"
)

type FormatSettings struct {
	Enlarge       bool
	BlurAmount    float32
	Pixelation    int
	Quality       int
	Interlaced    bool
	Height        int
	Width         int
	Crop          bool
	FeatureCrop   bool
	OutputFormat  imaging.Format
	HeightMissing bool
	WidthMissing  bool
}

var allowedTypes = []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}

func GetFormatSettings(r *http.Request, config *FormatDefaults) *FormatSettings {
	maxDimension := 3064
	heightMissing := false
	widthMissing := false
	height := int(to.Float64(r.URL.Query().Get("h")))
	if height == 0 {
		height = *config.DefaultHeight
		heightMissing = true
	}
	width := int(to.Float64(r.URL.Query().Get("w")))
	if width == 0 {
		width = *config.DefaultWidth
		widthMissing = true
	}
	if height > maxDimension {
		height = maxDimension
	}
	if width > maxDimension {
		width = maxDimension
	}
	enlarge := true
	if r.URL.Query().Get("e") != "" {
		enlarge = to.Bool(r.URL.Query().Get("e"))
	}
	featureCrop := false
	crop := !config.DefaultDontCrop
	if r.URL.Query().Get("c") != "" {
		crop = to.Bool(r.URL.Query().Get("c"))
	}
	//should only use the default if cropping is set to true
	featureCrop = config.DefaultFeatureCrop != nil && *config.DefaultFeatureCrop && crop
	if r.URL.Query().Get("fc") != "" {
		featureCrop = to.Bool(r.URL.Query().Get("fc"))
	}
	interlaced := true
	if r.URL.Query().Get("i") != "" {
		interlaced = to.Bool(r.URL.Query().Get("i"))
	}
	var quality int
	if config.DefaultQuality != nil {
		quality = *config.DefaultQuality
	}
	if r.URL.Query().Get("p") != "" {
		profile := string(r.URL.Query().Get("p"))
		if profile == "w" && config.WifiQuality != nil && *config.WifiQuality > 0 {
			quality = *config.WifiQuality
		}
	}
	if r.URL.Query().Get("q") != "" {
		quality = int(to.Float64(r.URL.Query().Get("q")))
	}
	blurAmount := float32(0)
	if r.URL.Query().Get("b") != "" {
		blurAmount = float32(to.Float64(r.URL.Query().Get("b")))
	}
	pixelation := 0
	if r.URL.Query().Get("px") != "" {
		pixelation = int(to.Float64(r.URL.Query().Get("px")))
		if pixelation > 100 {
			pixelation = 100
		} else if pixelation < 0 {
			pixelation = 0
		}
	}
	f := r.URL.Query().Get("f")
	fmt, err := imaging.FormatFromExtension(f)
	if err != nil {
		fmt = imaging.JPEG
	}
	return &FormatSettings{
		Height:        height,
		Crop:          crop,
		FeatureCrop:   featureCrop,
		Interlaced:    interlaced,
		Width:         width,
		Quality:       quality,
		BlurAmount:    blurAmount,
		Pixelation:    pixelation,
		Enlarge:       enlarge,
		OutputFormat:  fmt,
		HeightMissing: heightMissing,
		WidthMissing:  widthMissing,
	}
}

func getFormatSupported(format string, def string) string {
	return format
}

func ResizeCrop(image []byte, settings *FormatSettings) ([]byte, error) {
	// TODO - avoid unnecessary copying and allocations
	// image should be io.Reader and should return io.Writer
	src, err := imaging.Decode(bytes.NewReader(image))
	if err != nil {
		return nil, err
	}
	out := imaging.Resize(src, settings.Width, settings.Height, imaging.Lanczos)

	outBuf := bytes.Buffer{}
	err  = imaging.Encode(&outBuf, out, settings.OutputFormat)

	return outBuf.Bytes(), err
}
