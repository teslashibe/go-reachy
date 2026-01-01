package inference

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
)

// EncodeImageBase64 encodes an image to base64 JPEG format.
func EncodeImageBase64(img image.Image) (string, error) {
	var buf bytes.Buffer

	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// EncodeImageBytesBase64 encodes raw JPEG bytes to base64.
func EncodeImageBytesBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// DecodeBase64Image decodes a base64 string to an image.
func DecodeBase64Image(b64 string) (image.Image, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}



