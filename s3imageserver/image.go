package s3imageserver

import (
	"github.com/disintegration/imaging"
	"github.com/gosexy/to"
	"image"
	"io"
	"log"
	"net/http"
)

type FormatSettings struct {
	Enlarge       bool
	BlurAmount    float64
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
	q := r.URL.Query()
	height := int(to.Float64(q.Get("h")))
	width := int(to.Float64(q.Get("w")))
	if height > maxDimension {
		height = maxDimension
	}
	if width > maxDimension {
		width = maxDimension
	}
	if height == 0 && width == 0 {
		width, height = *config.DefaultWidth, *config.DefaultHeight
	}
	enlarge := true

	if q.Get("e") != "" {
		enlarge = to.Bool(q.Get("e"))
	}
	featureCrop := false
	crop := !config.DefaultDontCrop
	if q.Get("c") != "" {
		crop = to.Bool(q.Get("c"))
	}
	//should only use the default if cropping is set to true
	featureCrop = config.DefaultFeatureCrop != nil && *config.DefaultFeatureCrop && crop
	if q.Get("fc") != "" {
		featureCrop = to.Bool(q.Get("fc"))
	}
	interlaced := true
	if q.Get("i") != "" {
		interlaced = to.Bool(q.Get("i"))
	}
	var quality int
	if config.DefaultQuality != nil {
		quality = *config.DefaultQuality
	}
	if q.Get("p") != "" {
		profile := string(q.Get("p"))
		if profile == "w" && config.WifiQuality != nil && *config.WifiQuality > 0 {
			quality = *config.WifiQuality
		}
	}
	if q.Get("q") != "" {
		quality = int(to.Float64(q.Get("q")))
	}
	blurAmount := 0.0
	if q.Get("b") != "" {
		blurAmount = to.Float64(q.Get("b"))
	}
	pixelation := 0
	if q.Get("px") != "" {
		pixelation = int(to.Float64(q.Get("px")))
		if pixelation > 100 {
			pixelation = 100
		} else if pixelation < 0 {
			pixelation = 0
		}
	}
	f := q.Get("f")
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

func ResizeCrop(w io.Writer, r io.Reader, settings *FormatSettings) error {
	log.Printf("ResizeCrop %#v", settings)

	src, err := imaging.Decode(r)
	if err != nil {
		return err
	}
	log.Println("decoded image", src.Bounds().Max)
	var out image.Image
	if settings.Width <= 0 && settings.Height <= 0 {
		log.Println(settings.Width, settings.Height, "requested, keeping original dimentions")
		return imaging.Encode(w, src, settings.OutputFormat, imaging.JPEGQuality(settings.Quality))
	}

	if settings.Crop && settings.Width > 0 && settings.Height > 0 {
		out = imaging.Fill(src, settings.Width, settings.Height, imaging.Center, imaging.Lanczos)
	} else {
		out = imaging.Resize(src, settings.Width, settings.Height, imaging.Lanczos)

	}

	if settings.BlurAmount > 0 {
		out = imaging.Blur(out, settings.BlurAmount)
	}

	return imaging.Encode(w, out, settings.OutputFormat, imaging.JPEGQuality(settings.Quality))

}
