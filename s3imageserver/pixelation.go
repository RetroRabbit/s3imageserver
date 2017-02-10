package s3imageserver

import (
  "image"
  "bytes"
  _ "image/png"
  "image/jpeg"
)

func (i *Image) pixelate(w *ResponseWriter) () {
  img, _, err := image.Decode(bytes.NewReader(i.Image))
  if err == nil {
    bounds := img.Bounds()
    blockSize :=int(float64(bounds.Dx()) * (float64(i.Pixelation) / 100.0))
    numBlocksX := bounds.Dx() / blockSize
    if bounds.Dx()%blockSize > 0 {
        numBlocksX++
    }
    numBlocksY := bounds.Dy() / blockSize
    if bounds.Dy()%blockSize > 0 {
        numBlocksY++
    }
    dst := image.NewRGBA(bounds)
    pixGetter := newPixelGetter(img)
      pixSetter := newPixelSetter(dst)

    for by := 0; by < numBlocksY; by++ {
            for bx := 0; bx < numBlocksX; bx++ {
                // calculate the block bounds
                bb := image.Rect(bx*blockSize, by*blockSize, (bx+1)*blockSize, (by+1)*blockSize)
                bbSrc := bb.Add(bounds.Min).Intersect(bounds)

                // calculate average color of the block
                var r, g, b, a float32
                var cnt float32
                for y := bbSrc.Min.Y; y < bbSrc.Max.Y; y++ {
                    for x := bbSrc.Min.X; x < bbSrc.Max.X; x++ {
                        px := pixGetter.getPixel(x, y)
                        r += px.R
                        g += px.G
                        b += px.B
                        a += px.A
                        cnt++
                    }
                }
                if cnt > 0 {
                    r /= cnt
                    g /= cnt
                    b /= cnt
                    a /= cnt
                }

                // set the calculated color for all pixels in the block
                for y := bbSrc.Min.Y; y < bbSrc.Max.Y; y++ {
                    for x := bbSrc.Min.X; x < bbSrc.Max.X; x++ {
                        pixSetter.setPixel(x, y, pixel{r, g, b, a})
                    }
                }
            }
        }

    buf := new(bytes.Buffer)
    err := jpeg.Encode(buf, dst, nil)
    if (err == nil) {
      i.Image = buf.Bytes()
    } else {
      w.log("PIXELATION: ", err)
    }
  }
}
