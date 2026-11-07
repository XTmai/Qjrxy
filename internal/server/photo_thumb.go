package server

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // 注册 PNG 解码器
	"os"
)

// errThumbnailUnsupported 表示该格式无法生成缩略图（如 webp，标准库不支持解码），
// 调用方应退回原图。
var errThumbnailUnsupported = errors.New("缩略图不支持该格式")

// thumbnailConcurrency 限制同时解码原图的数量：
// 解码一张 12MP 照片瞬时占用 ~48MB 内存，并发 12 张会把小内存 VPS 打爆。
var thumbnailConcurrency = make(chan struct{}, 2)

// photoThumbnail 生成宽 maxW 的 JPEG 缩略图（保持纵横比，只缩小不放大）。
//
// 手机拍摄的照片普遍带 EXIF 方向标记（竖拍时 orientation=6），浏览器渲染
// <img> 时会自动应用，但我们重编码后 EXIF 丢失，缩略图会侧躺——因此这里
// 显式读取方向并烘焙进像素。
func photoThumbnail(path string, maxW int) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return photoThumbnailFromBytes(data, maxW)
}

// photoThumbnailFromBytes 是 photoThumbnail 的纯函数入口，便于测试。
func photoThumbnailFromBytes(data []byte, maxW int) ([]byte, error) {
	thumbnailConcurrency <- struct{}{}
	defer func() { <-thumbnailConcurrency }()

	var img image.Image
	var err error
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8}):
		img, err = jpeg.Decode(bytes.NewReader(data))
	case bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47}):
		img, err = decodePNG(data)
	default:
		return nil, errThumbnailUnsupported
	}
	if err != nil {
		return nil, err
	}

	b := img.Bounds()
	if b.Dx() <= maxW {
		// 原图不比目标宽：不值得重编码，退回原图（浏览器对带 EXIF 的原图
		// 仍会自动摆正方向）。
		return nil, errThumbnailUnsupported
	}

	orientation := 1
	if bytes.HasPrefix(data, []byte{0xFF, 0xD8}) {
		orientation = jpegOrientation(data)
	}

	// 先降采样（大图 → 小图），再烘焙方向（小图转置开销可忽略）
	dstH := b.Dy() * maxW / b.Dx()
	small := resizeArea(img, maxW, dstH)
	small = applyOrientation(small, orientation)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, small, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodePNG(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// resizeArea 面积平均降采样：目标每个像素是源图中对应矩形的平均色。
// 对 10 倍以上的大幅缩小，面积平均比插值算法更抗锯齿（等价于盒滤波）。
func resizeArea(src image.Image, dstW, dstH int) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for dy := 0; dy < dstH; dy++ {
		sy0 := dy * sh / dstH
		sy1 := (dy + 1) * sh / dstH
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for dx := 0; dx < dstW; dx++ {
			sx0 := dx * sw / dstW
			sx1 := (dx + 1) * sw / dstW
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, bl, a, cnt uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					pr, pg, pb, pa := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
					r += uint64(pr)
					g += uint64(pg)
					bl += uint64(pb)
					a += uint64(pa)
					cnt++
				}
			}
			if cnt == 0 {
				continue
			}
			dst.SetRGBA(dx, dy, color.RGBA{
				R: uint8(r / cnt >> 8), G: uint8(g / cnt >> 8),
				B: uint8(bl / cnt >> 8), A: uint8(a / cnt >> 8),
			})
		}
	}
	return dst
}

// jpegOrientation 从 JPEG 的 EXIF 里读 Orientation 标记（1-8），无 EXIF 返回 1。
func jpegOrientation(data []byte) int {
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xFF {
			i++ // 填充字节
			continue
		}
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD9) || marker == 0xDA {
			return 1 // SOS 之后不再有 EXIF
		}
		segLen := int(data[i+2])<<8 | int(data[i+3])
		if segLen < 2 || i+2+segLen > len(data) {
			return 1
		}
		if marker == 0xE1 && i+10 <= len(data) && string(data[i+4:i+10]) == "Exif\x00\x00" {
			return parseTIFFOrientation(data[i+10 : i+2+segLen])
		}
		i += 2 + segLen
	}
	return 1
}

// parseTIFFOrientation 在 EXIF TIFF 头的 IFD0 里找 Orientation (0x0112)。
func parseTIFFOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	le := tiff[0] == 'I' && tiff[1] == 'I'
	u16 := func(b []byte) int {
		if le {
			return int(b[0]) | int(b[1])<<8
		}
		return int(b[0])<<8 | int(b[1])
	}
	if u16(tiff[2:4]) != 42 {
		return 1
	}
	ifd0 := u16(tiff[4:6]) // IFD 偏移是 32 位，但小文件里高 16 位为 0
	if len(tiff) >= 8 {
		ifd0 = int(uint32(tiff[4]) | uint32(tiff[5])<<8 | uint32(tiff[6])<<16 | uint32(tiff[7])<<24)
		if le {
			ifd0 = int(uint32(tiff[7]) | uint32(tiff[6])<<8 | uint32(tiff[5])<<16 | uint32(tiff[4])<<24)
		}
	}
	if ifd0 < 8 || ifd0+2 > len(tiff) {
		return 1
	}
	n := u16(tiff[ifd0 : ifd0+2])
	for k := 0; k < n; k++ {
		off := ifd0 + 2 + k*12
		if off+12 > len(tiff) {
			break
		}
		if u16(tiff[off:off+2]) == 0x0112 {
			v := u16(tiff[off+8 : off+10])
			if v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// applyOrientation 按 EXIF 方向翻转/旋转（在缩略图尺寸上操作，开销可忽略）。
func applyOrientation(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := w, h
	if o >= 5 { // 5-8 含 90° 旋转，宽高互换
		nw, nh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var nx, ny int
			switch o {
			case 2: // 水平翻转
				nx, ny = w-1-x, y
			case 3: // 旋转 180°
				nx, ny = w-1-x, h-1-y
			case 4: // 垂直翻转
				nx, ny = x, h-1-y
			case 5: // 转置
				nx, ny = y, x
			case 6: // 旋转 90° CW
				nx, ny = h-1-y, x
			case 7: // 反转置
				nx, ny = h-1-y, w-1-x
			case 8: // 旋转 90° CCW
				nx, ny = y, w-1-x
			}
			dst.Set(nx, ny, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
