package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
	"os"
)

var asciiChars = []rune(" ·:-=+*#%@█")

func pixelToASCII(c color.Color) rune {
	r, g, b, _ := c.RGBA()
	red := float64(r) / 257.0
	green := float64(g) / 257.0
	blue := float64(b) / 257.0
	brightness := 0.2126*red + 0.7152*green + 0.0722*blue
	scale := brightness / 255.0
	index := int(scale * float64(len(asciiChars)-1))
	if index < 0 {
		index = 0
	} else if index >= len(asciiChars) {
		index = len(asciiChars) - 1
	}
	return asciiChars[index]
}

func resizeImage(img image.Image, newWidth, newHeight int) image.Image {
	oldWidth := img.Bounds().Dx()
	oldHeight := img.Bounds().Dy()

	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	xScale := float64(oldWidth) / float64(newWidth)
	yScale := float64(oldHeight) / float64(newHeight)

	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := int(math.Floor(float64(x) * xScale))
			srcY := int(math.Floor(float64(y) * yScale))
			if srcX >= oldWidth {
				srcX = oldWidth - 1
			}
			if srcY >= oldHeight {
				srcY = oldHeight - 1
			}
			dst.Set(x, y, img.At(srcX, srcY))
		}
	}

	return dst
}

func rotate90(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			newX := h - (y - bounds.Min.Y) - 1
			newY := x - bounds.Min.X
			dst.Set(newX, newY, img.At(x, y))
		}
	}
	return dst
}

func rotate180(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			newX := w - (x - bounds.Min.X) - 1
			newY := h - (y - bounds.Min.Y) - 1
			dst.Set(newX, newY, img.At(x, y))
		}
	}
	return dst
}

func rotate270(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			newX := y - bounds.Min.Y
			newY := w - (x - bounds.Min.X) - 1
			dst.Set(newX, newY, img.At(x, y))
		}
	}
	return dst
}

func readExifOrientation(filename string) (int, error) {
	f, err := os.Open(filename)
	if err != nil {
		return 1, err
	}
	defer f.Close()

	var marker [2]byte
	if _, err := f.Read(marker[:]); err != nil {
		return 1, err
	}
	if marker[0] != 0xFF || marker[1] != 0xD8 {
		return 1, fmt.Errorf("not a JPEG file")
	}

	for {
		var segMarker [2]byte
		if _, err := f.Read(segMarker[:]); err != nil {
			break
		}
		if segMarker[0] != 0xFF {
			return 1, fmt.Errorf("invalid marker found")
		}

		if segMarker[1] == 0xE1 {
			var segLengthBytes [2]byte
			if _, err := f.Read(segLengthBytes[:]); err != nil {
				return 1, err
			}
			segLength := int(binary.BigEndian.Uint16(segLengthBytes[:])) - 2

			data := make([]byte, segLength)
			if _, err := io.ReadFull(f, data); err != nil {
				return 1, err
			}

			if len(data) < 6 || string(data[:6]) != "Exif\x00\x00" {
				continue
			}

			tiffData := data[6:]
			if len(tiffData) < 8 {
				return 1, fmt.Errorf("invalid TIFF data")
			}

			var order binary.ByteOrder
			if string(tiffData[:2]) == "II" {
				order = binary.LittleEndian
			} else if string(tiffData[:2]) == "MM" {
				order = binary.BigEndian
			} else {
				return 1, fmt.Errorf("invalid byte order")
			}

			if order.Uint16(tiffData[2:4]) != 42 {
				return 1, fmt.Errorf("invalid TIFF header")
			}

			ifdOffset := int(order.Uint32(tiffData[4:8]))
			if ifdOffset+2 > len(tiffData) {
				return 1, fmt.Errorf("invalid IFD offset")
			}

			numEntries := int(order.Uint16(tiffData[ifdOffset : ifdOffset+2]))
			for i := 0; i < numEntries; i++ {
				entryOffset := ifdOffset + 2 + i*12
				if entryOffset+12 > len(tiffData) {
					break
				}
				tag := order.Uint16(tiffData[entryOffset : entryOffset+2])
				if tag == 0x0112 {
					orient := order.Uint16(tiffData[entryOffset+8 : entryOffset+10])
					return int(orient), nil
				}
			}
		} else {
			var segLengthBytes [2]byte
			if _, err := f.Read(segLengthBytes[:]); err != nil {
				break
			}
			segLength := int(binary.BigEndian.Uint16(segLengthBytes[:])) - 2
			if _, err := f.Seek(int64(segLength), io.SeekCurrent); err != nil {
				break
			}
		}
	}
	return 1, nil
}

func main() {
	color := flag.Bool("color", false, "grey")
	flag.Parse()
	filename := flag.Args()[0]

	orientation, err := readExifOrientation(filename)
	if err != nil {
		log.Printf("Warning: could not read EXIF orientation: %v", err)
		orientation = 1
	}

	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("Failed to open image: %v", err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		log.Fatalf("Failed to decode image: %v", err)
	}

	switch orientation {
	case 3:
		img = rotate180(img)
	case 6:
		img = rotate90(img)
	case 8:
		img = rotate270(img)
	}

	newWidth := 80
	newHeight := 40
	resizedImg := resizeImage(img, newWidth, newHeight)

	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			c := resizedImg.At(x, y)
			asciiChar := pixelToASCII(c)
			if *color {
				r, g, b, _ := c.RGBA()
				red := int(r / 257)
				green := int(g / 257)
				blue := int(b / 257)

				fmt.Printf("\x1b[38;2;%d;%d;%dm%c\x1b[0m", red, green, blue, asciiChar)
			} else {
				fmt.Printf("%c", asciiChar)
			}
		}
		fmt.Println()
	}
}
